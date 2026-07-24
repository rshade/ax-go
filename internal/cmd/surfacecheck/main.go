// surfacecheck is internal maintainer/CI tooling: it scans the complete
// compiler-visible surface of every public package, under each supported
// build-tag configuration and target profile, compares the result against the
// reviewed live baseline, and cross-validates the permanent public-surface
// audit. Successful modes write one minified JSON document to stdout; every
// failure writes exactly one minified ax.Error envelope to stderr.
//
// Build constraints create a blind spot the other gates cannot see. go-apidiff
// diffs one configuration; `go vet ./...` and `go test ./...` compile one
// configuration; doccover parses source while ignoring build constraints
// entirely. Nothing else in the repository would notice that ax_no_grpc had
// quietly started removing a second identifier, or that a tag combination had
// stopped compiling on windows/arm64. surfacecheck scans the 4 supported tag
// combinations × 6 GOOS/GOARCH profiles = 24 loads of the seven public
// packages.
//
// The load COUNT does not scale with the package count: a load is one
// (configuration, profile) combination, and scanCombination loads every
// requested package within it. Adding a seventh public package therefore leaves
// this at 24.
//
// The gate is NOT a one-way ratchet (unlike doccover's baseline.txt): symbols
// legitimately come and go, so additions and removals both surface as drift
// and are resolved by a reviewed regeneration:
//
//	go run ./internal/cmd/surfacecheck -update
//	git diff internal/cmd/surfacecheck/baseline.json   # review every line
//
// Policy as constants: the tag combinations, profile list, and public package
// list are hardcoded in inventory.go, so a matrix change is a reviewable Go
// commit auditable through git blame — matching covercheck and benchcheck.
//
// Run from the module root:
//
//	go run ./internal/cmd/surfacecheck
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/rshade/ax-go/contract"
)

// maxArtifactBytes is the read bound for baseline and audit artifacts; an
// oversized artifact is invalid repository input, never an OOM.
const maxArtifactBytes = 8 << 20

// baselineSchemaVersion is the baseline schema version. Version 2 records
// every public package across the build-tag configuration axis; version 1
// recorded a flat root-only feature list. A mismatch is bad input, not drift.
const baselineSchemaVersion = 2

// Default artifact locations, resolved relative to the invocation directory
// (module root for all documented invocations).
const (
	defaultBaselinePath = "internal/cmd/surfacecheck/baseline.json"
	defaultAuditPath    = "specs/015-internalize-helpers/public-surface-audit.json"
)

// Stable error codes for the failure contract.
const (
	codeDrift      = "surface_drift"
	codeArtifact   = "invalid_surface_artifact"
	codePermission = "surface_permission"
	codeInternal   = "surface_internal"
)

// allSentinel is the canonical encoding for "present in every configuration"
// or "present on every profile". An exhaustive explicit list is normalised to
// this on write so a regenerated baseline is byte-identical.
const allSentinel = "all"

// presenceSet is a JSON value that is either the string "all" or an explicit
// sorted list. It keeps a universal feature's baseline entry compact and, more
// importantly, canonical: an exhaustive list and "all" must not be two
// spellings of the same fact, or the baseline would not be byte-deterministic.
//
// The mixed receivers below are required by encoding/json, not an oversight:
// MarshalJSON must take a value receiver because presenceSet is stored by
// value inside baselineFeature, while UnmarshalJSON must take a pointer
// receiver to write through to the caller.
//
//nolint:recvcheck // encoding/json requires value-receiver Marshal and pointer-receiver Unmarshal
type presenceSet struct {
	// All is true when the feature is present in every member of the universe.
	All bool
	// Values is the explicit sorted membership list; empty when All is true.
	Values []string
}

// MarshalJSON renders "all" or the explicit sorted list.
func (s presenceSet) MarshalJSON() ([]byte, error) {
	if s.All {
		return json.Marshal(allSentinel)
	}
	values := s.Values
	if values == nil {
		values = []string{}
	}
	return json.Marshal(values)
}

// UnmarshalJSON accepts either the "all" sentinel or an explicit list, and
// rejects any other scalar so a typo like "ALL" is bad input rather than a
// silently empty set.
func (s *presenceSet) UnmarshalJSON(data []byte) error {
	var sentinel string
	if err := json.Unmarshal(data, &sentinel); err == nil {
		if sentinel != allSentinel {
			return fmt.Errorf("presence must be %q or a list, got %q", allSentinel, sentinel)
		}
		s.All = true
		s.Values = nil
		return nil
	}
	var values []string
	if err := json.Unmarshal(data, &values); err != nil {
		return fmt.Errorf("presence must be %q or a list of strings: %w", allSentinel, err)
	}
	s.All = false
	s.Values = values
	return nil
}

// contains reports whether member is in the set, resolving the "all" sentinel.
func (s presenceSet) contains(member string) bool {
	return s.All || slices.Contains(s.Values, member)
}

// validate rejects a set whose explicit membership is empty, unsorted,
// duplicated, or outside universe.
func (s presenceSet) validate(universe []string) error {
	if s.All {
		if len(s.Values) != 0 {
			return errors.New(`the "all" sentinel carries no explicit values`)
		}
		return nil
	}
	if len(s.Values) == 0 {
		return errors.New("presence must list at least one member")
	}
	if len(s.Values) == len(universe) {
		return fmt.Errorf("an exhaustive presence list must be written as %q", allSentinel)
	}
	for i, v := range s.Values {
		if !slices.Contains(universe, v) {
			return fmt.Errorf("presence member %q is not a known value", v)
		}
		if i > 0 && s.Values[i-1] >= v {
			return fmt.Errorf("presence must be sorted bytewise and duplicate-free near %q", v)
		}
	}
	return nil
}

// newPresenceSet builds a canonical presence set from members, collapsing to
// the "all" sentinel when members covers the whole universe.
func newPresenceSet(members map[string]bool, universe []string) presenceSet {
	if len(members) == len(universe) {
		return presenceSet{All: true}
	}
	values := make([]string, 0, len(members))
	for m := range members {
		values = append(values, m)
	}
	slices.Sort(values)
	return presenceSet{Values: values}
}

// baselineFeature is one current approved compiler-visible API feature,
// together with the configurations and profiles it is present in.
type baselineFeature struct {
	ID             string      `json:"id"`
	Signature      string      `json:"signature"`
	Configurations presenceSet `json:"configurations"`
	Profiles       presenceSet `json:"profiles"`
}

// combinations expands the recorded presence into the exact set of
// combinations the feature is expected in.
//
// Presence is stored as the product of a configuration set and a profile set
// because build constraints and platform constraints are independent
// mechanisms, so real presence patterns factorise. buildBaseline verifies that
// factorisation is exact before writing, so expansion here is lossless.
func (f baselineFeature) combinations(universe []Combination) map[Combination]bool {
	expected := map[Combination]bool{}
	for _, combo := range universe {
		if f.Configurations.contains(combo.Configuration) && f.Profiles.contains(combo.Profile) {
			expected[combo] = true
		}
	}
	return expected
}

// packageBaseline is one public package's recorded surface.
type packageBaseline struct {
	Path     string            `json:"path"`
	Features []baselineFeature `json:"features"`
}

// baselineDoc is the live operational surface projection.
type baselineDoc struct {
	SchemaVersion int               `json:"schema_version"`
	Packages      []packageBaseline `json:"packages"`
}

// rootFeatures returns the root package's recorded features; the audit joins
// on the bare root spellings only.
func (d *baselineDoc) rootFeatures(root string) []baselineFeature {
	for _, pkg := range d.Packages {
		if pkg.Path == root {
			return pkg.Features
		}
	}
	return nil
}

// auditRecord is the permanent decision for one API feature.
type auditRecord struct {
	ID                    string   `json:"id"`
	Kind                  string   `json:"kind"`
	Owner                 string   `json:"owner"`
	Name                  string   `json:"name"`
	Signature             string   `json:"signature"`
	Classification        string   `json:"classification"`
	Rationale             string   `json:"rationale"`
	Disposition           string   `json:"disposition"`
	InternalTarget        string   `json:"internal_target"`
	Replacement           string   `json:"replacement"`
	CompatibilityStrategy string   `json:"compatibility_strategy"`
	Lifecycle             string   `json:"lifecycle"`
	FirstPublished        string   `json:"first_published"`
	DeprecatedIn          string   `json:"deprecated_in"`
	RemovedIn             string   `json:"removed_in"`
	DownstreamCheckedAt   string   `json:"downstream_checked_at"`
	DownstreamEvidence    []string `json:"downstream_evidence"`
}

// auditDoc is the permanent historical decision artifact.
type auditDoc struct {
	SchemaVersion int           `json:"schema_version"`
	AuditedAt     string        `json:"audited_at"`
	Records       []auditRecord `json:"records"`
}

// gateResult is the success document emitted on stdout for a passing check.
type gateResult struct {
	Status                string `json:"status"`
	FeaturesChecked       int    `json:"features_checked"`
	AuditRecordsChecked   int    `json:"audit_records_checked"`
	PackagesChecked       int    `json:"packages_checked"`
	ConfigurationsChecked int    `json:"configurations_checked"`
	ProfilesChecked       int    `json:"profiles_checked"`
}

// updateResult is the success document emitted on stdout after -update
// rewrites the committed baseline.
type updateResult struct {
	Status          string `json:"status"`
	Baseline        string `json:"baseline"`
	FeaturesWritten int    `json:"features_written"`
}

// decodeStrict unmarshals one JSON document with unknown-field and
// trailing-value rejection under a hard size cap.
func decodeStrict(data []byte, v any) error {
	if len(data) > maxArtifactBytes {
		return fmt.Errorf("artifact exceeds %d bytes", maxArtifactBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("decoding artifact: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("trailing data after the JSON document")
	}
	return nil
}

// parseBaseline decodes and validates a live baseline document against the
// run's policy.
func parseBaseline(data []byte, schema baselineSchema) (*baselineDoc, error) {
	var doc baselineDoc
	if err := decodeStrict(data, &doc); err != nil {
		return nil, err
	}
	if err := validateBaseline(&doc, schema); err != nil {
		return nil, err
	}
	return &doc, nil
}

// validateBaseline enforces the baseline schema contract: the recorded
// packages are exactly the reviewed public package list in canonical order,
// and every feature is sorted, uniquely identified, and carries a canonical
// presence encoding.
func validateBaseline(doc *baselineDoc, schema baselineSchema) error {
	if doc.SchemaVersion != baselineSchemaVersion {
		return fmt.Errorf("schema_version must be %d, got %d", baselineSchemaVersion, doc.SchemaVersion)
	}
	if doc.Packages == nil {
		return errors.New("packages is required")
	}
	if len(doc.Packages) != len(schema.packages) {
		return fmt.Errorf("packages must record the %d public packages, got %d",
			len(schema.packages), len(doc.Packages))
	}
	for i, pkg := range doc.Packages {
		if pkg.Path != schema.packages[i] {
			return fmt.Errorf("packages[%d]: expected %q, got %q", i, schema.packages[i], pkg.Path)
		}
		if pkg.Features == nil {
			return fmt.Errorf("packages[%d] (%s): features is required", i, pkg.Path)
		}
		if err := validatePackageFeatures(pkg, schema.configs, schema.profiles); err != nil {
			return fmt.Errorf("packages[%d] (%s): %w", i, pkg.Path, err)
		}
	}
	return nil
}

// validatePackageFeatures enforces the per-feature schema rules for one
// recorded package.
func validatePackageFeatures(pkg packageBaseline, configNames, profileNames []string) error {
	for j, f := range pkg.Features {
		if f.ID == "" || f.Signature == "" {
			return fmt.Errorf("features[%d]: id and signature are required", j)
		}
		if j > 0 && pkg.Features[j-1].ID >= f.ID {
			return fmt.Errorf("features must be sorted bytewise by unique id near %q", f.ID)
		}
		if err := f.Configurations.validate(configNames); err != nil {
			return fmt.Errorf("features[%d] (%s): configurations: %w", j, f.ID, err)
		}
		if err := f.Profiles.validate(profileNames); err != nil {
			return fmt.Errorf("features[%d] (%s): profiles: %w", j, f.ID, err)
		}
	}
	return nil
}

// configurationNames returns the configuration names in canonical order.
func configurationNames(configs []Configuration) []string {
	names := make([]string, len(configs))
	for i, c := range configs {
		names[i] = c.Name
	}
	return names
}

// profileNames returns the profile names in canonical order.
func profileNames(profiles []Profile) []string {
	names := make([]string, len(profiles))
	for i, p := range profiles {
		names[i] = p.String()
	}
	return names
}

// parseAudit decodes and validates a permanent audit document.
func parseAudit(data []byte) (*auditDoc, error) {
	var doc auditDoc
	if err := decodeStrict(data, &doc); err != nil {
		return nil, err
	}
	if err := validateAudit(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

var (
	// semverTag matches release tags such as v0.4.0.
	semverTag = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	// internalTarget matches cohesive internal/<role> relocation targets. Each
	// slash-separated segment must be a well-formed lowercase package name, so
	// empty segments, trailing slashes, and dangling separators are rejected
	// rather than accepted as import paths that cannot exist.
	internalTarget = regexp.MustCompile(`^internal(/[a-z0-9]+([_-][a-z0-9]+)*)+$`)
)

// fullDate is the RFC 3339 full-date layout required for artifact dates.
const fullDate = "2006-01-02"

// validateAudit enforces the audit schema contract including classification,
// disposition, and lifecycle cross-field rules.
func validateAudit(doc *auditDoc) error {
	if doc.SchemaVersion != 1 {
		return fmt.Errorf("schema_version must be 1, got %d", doc.SchemaVersion)
	}
	if _, err := time.Parse(fullDate, doc.AuditedAt); err != nil {
		return fmt.Errorf("audited_at must be an RFC 3339 full-date: %w", err)
	}
	if doc.Records == nil {
		return errors.New("records is required")
	}
	for i, r := range doc.Records {
		if err := validateRecord(r); err != nil {
			return fmt.Errorf("records[%d] (%s): %w", i, r.ID, err)
		}
		if i > 0 && doc.Records[i-1].ID >= r.ID {
			return fmt.Errorf("records must be sorted bytewise by unique id near %q", r.ID)
		}
	}
	return nil
}

// validateRecord enforces the per-record cross-field rules.
func validateRecord(r auditRecord) error {
	if err := validateRecordIdentity(r); err != nil {
		return err
	}
	if err := validateRecordDates(r); err != nil {
		return err
	}
	if err := validateRecordEvidence(r); err != nil {
		return err
	}
	if err := validateClassification(r); err != nil {
		return err
	}
	return validateLifecycle(r)
}

// validateRecordIdentity enforces identity, kind, and rationale rules. The
// canonical ID redundantly encodes kind, owner, and name, so the two
// spellings are cross-checked: an audit whose halves disagree cannot be
// meaningfully reconciled against the live surface, which joins on ID alone.
func validateRecordIdentity(r auditRecord) error {
	if r.ID == "" || r.Name == "" || r.Signature == "" {
		return errors.New("id, name, and signature are required")
	}
	switch r.Kind {
	case kindConst, kindVar, kindFunc, kindType, kindField, kindInterfaceMethod, kindMethod:
	default:
		return fmt.Errorf("kind %q is not a valid feature kind", r.Kind)
	}
	if err := validateOwnerScope(r); err != nil {
		return err
	}
	if !slices.Contains(expectedFeatureIDs(r.Kind, r.Owner, r.Name), r.ID) {
		return fmt.Errorf("id %q does not match kind %q, owner %q, and name %q",
			r.ID, r.Kind, r.Owner, r.Name)
	}
	if r.Rationale == "" || strings.ContainsAny(r.Rationale, "\n\r") {
		return errors.New("rationale must be a single non-empty line")
	}
	return nil
}

// validateOwnerScope enforces the owner-scope rule in both directions: member
// kinds name the root selector they hang off, and package-scope kinds do not.
func validateOwnerScope(r auditRecord) error {
	if memberKind(r.Kind) {
		if r.Owner == "" {
			return fmt.Errorf("member kind %q requires a non-empty owner", r.Kind)
		}
		return nil
	}
	if r.Owner != "" {
		return fmt.Errorf("package-scope kind %q requires an empty owner, got %q", r.Kind, r.Owner)
	}
	return nil
}

// memberKind reports whether kind names a member selected from a root type
// rather than a package-scope declaration.
func memberKind(kind string) bool {
	switch kind {
	case kindField, kindInterfaceMethod, kindMethod:
		return true
	default:
		return false
	}
}

// expectedFeatureIDs returns the canonical feature IDs that kind, owner, and
// name may legitimately produce. Pointer-receiver-only methods carry a "*"
// marker that is not recoverable from owner alone, so kindMethod admits both
// the value and pointer spellings.
func expectedFeatureIDs(kind, owner, name string) []string {
	if owner == "" {
		return []string{kind + ":" + name}
	}
	ids := []string{kind + ":" + owner + "." + name}
	if kind == kindMethod {
		ids = append(ids, kind+":*"+owner+"."+name)
	}
	return ids
}

// validateRecordDates enforces tag and full-date formats on the release and
// evidence fields.
func validateRecordDates(r auditRecord) error {
	tags := []struct{ name, value string }{
		{"first_published", r.FirstPublished},
		{"deprecated_in", r.DeprecatedIn},
		{"removed_in", r.RemovedIn},
	}
	for _, tag := range tags {
		if tag.value != "" && !semverTag.MatchString(tag.value) {
			return fmt.Errorf("%s %q is not a vX.Y.Z tag", tag.name, tag.value)
		}
	}
	if r.DownstreamCheckedAt != "" {
		if _, err := time.Parse(fullDate, r.DownstreamCheckedAt); err != nil {
			return fmt.Errorf("downstream_checked_at must be an RFC 3339 full-date: %w", err)
		}
	}
	return nil
}

// validateRecordEvidence enforces sortedness, uniqueness, and non-emptiness
// of downstream evidence entries.
func validateRecordEvidence(r auditRecord) error {
	for j, e := range r.DownstreamEvidence {
		if e == "" {
			return errors.New("downstream_evidence entries must be non-empty")
		}
		if j > 0 && r.DownstreamEvidence[j-1] >= e {
			return errors.New("downstream_evidence must be sorted bytewise and duplicate-free")
		}
	}
	return nil
}

// validateLifecycle enforces the lifecycle state machine's field
// requirements.
func validateLifecycle(r auditRecord) error {
	switch r.Lifecycle {
	case lifecycleLive, lifecycleDeprecated:
		if r.DeprecatedIn != "" || r.RemovedIn != "" {
			return errors.New("live and deprecated require empty deprecated_in and removed_in")
		}
	case lifecycleRemovable:
		if r.DeprecatedIn == "" {
			return errors.New("removable requires a verified deprecated_in")
		}
		if r.RemovedIn != "" {
			return errors.New("removable requires an empty removed_in")
		}
	case lifecycleRemoved:
		if r.DeprecatedIn == "" || r.RemovedIn == "" {
			return errors.New("removed requires deprecated_in and removed_in")
		}
	default:
		return fmt.Errorf("lifecycle %q is not valid", r.Lifecycle)
	}
	return nil
}

// validateClassification enforces the classification/disposition pairing
// rules from the audit schema contract.
func validateClassification(r auditRecord) error {
	switch r.Classification {
	case "supported":
		return validateSupported(r)
	case "implementation-leak":
		return validateLeak(r)
	default:
		return fmt.Errorf("classification %q is not valid", r.Classification)
	}
}

// validateSupported enforces that supported rows stay public and live with no
// migration fields populated.
func validateSupported(r auditRecord) error {
	if r.Disposition != "keep-public" {
		return errors.New("supported pairs only with keep-public")
	}
	if r.Lifecycle != lifecycleLive {
		return errors.New("supported pairs only with lifecycle live")
	}
	if r.InternalTarget != "" || r.Replacement != "" || r.DownstreamCheckedAt != "" {
		return errors.New("supported rows leave internal_target, replacement, and downstream_checked_at empty")
	}
	return nil
}

// validateLeak enforces the implementation-leak evidence and migration
// requirements.
func validateLeak(r auditRecord) error {
	if err := validateLeakDisposition(r); err != nil {
		return err
	}
	if r.Replacement == "" {
		return errors.New("implementation-leak requires a replacement or an explicit no-replacement reason")
	}
	if r.CompatibilityStrategy == "" {
		return errors.New("implementation-leak requires a compatibility strategy")
	}
	if r.DownstreamCheckedAt == "" {
		return errors.New("implementation-leak requires a downstream evidence search date")
	}
	if r.DownstreamEvidence == nil {
		return errors.New("implementation-leak requires a downstream evidence array")
	}
	return nil
}

// validateLeakDisposition enforces the disposition/internal-target pairing.
func validateLeakDisposition(r auditRecord) error {
	switch r.Disposition {
	case "relocate-with-forwarder":
		if !validRelocationTarget(r.InternalTarget) {
			return fmt.Errorf("relocate-with-forwarder requires a cohesive internal/<role> target, got %q",
				r.InternalTarget)
		}
	case "deprecate-in-place":
		if r.InternalTarget != "" && !validRelocationTarget(r.InternalTarget) {
			return fmt.Errorf("internal_target %q is not a cohesive internal/<role> target", r.InternalTarget)
		}
	default:
		return errors.New("implementation-leak pairs only with relocate-with-forwarder or deprecate-in-place")
	}
	return nil
}

// validRelocationTarget reports whether target names a cohesive internal
// package; the grab-bag internal/helpers is forbidden.
func validRelocationTarget(target string) bool {
	if !internalTarget.MatchString(target) {
		return false
	}
	return target != "internal/helpers" && !strings.HasPrefix(target, "internal/helpers/")
}

// baselineFromInventory projects the live inventory into a baseline document,
// verifying that every feature's presence factorises exactly into a
// configuration set and a profile set.
//
// The product encoding cannot represent a presence pattern that does not
// factorise (present on linux under one configuration and on windows under
// another, say). Rather than record such a pattern lossily — which would let
// the gate later accept a surface nobody reviewed — this fails closed and
// reports the feature, leaving the decision to a maintainer.
func baselineFromInventory(inv []Observation, cfg runConfig,
	universe []Combination) (*baselineDoc, []DriftItem) {
	byPackage := map[string][]baselineFeature{}
	configNames := configurationNames(cfg.Configurations)
	profileNames := profileNames(cfg.Profiles)
	var drift []DriftItem

	for _, o := range inv {
		presentConfigs := map[string]bool{}
		presentProfiles := map[string]bool{}
		for combo := range o.Present {
			presentConfigs[combo.Configuration] = true
			presentProfiles[combo.Profile] = true
		}
		feature := baselineFeature{
			ID:             o.Feature.ID,
			Signature:      o.Feature.Signature,
			Configurations: newPresenceSet(presentConfigs, configNames),
			Profiles:       newPresenceSet(presentProfiles, profileNames),
		}
		key := featureKey{Package: o.Feature.Package, ID: o.Feature.ID}
		if !presenceFactorises(o.Present, feature, universe) {
			drift = append(drift, DriftItem{
				ID:       qualifiedID(key, cfg.root()),
				Drift:    driftPresenceUnfactored,
				Expected: "presence expressible as configurations × profiles",
				Actual:   "presence varies by configuration and profile together",
			})
			continue
		}
		byPackage[o.Feature.Package] = append(byPackage[o.Feature.Package], feature)
	}
	if len(drift) > 0 {
		sortDrift(drift)
		return nil, drift
	}

	packages := make([]packageBaseline, 0, len(cfg.Packages))
	for _, path := range cfg.Packages {
		features := byPackage[path]
		if features == nil {
			features = []baselineFeature{}
		}
		packages = append(packages, packageBaseline{Path: path, Features: features})
	}
	return &baselineDoc{SchemaVersion: baselineSchemaVersion, Packages: packages}, nil
}

// presenceFactorises reports whether the product encoding reproduces the
// observed presence exactly.
func presenceFactorises(observed map[Combination]bool,
	feature baselineFeature, universe []Combination) bool {
	expected := feature.combinations(universe)
	if len(expected) != len(observed) {
		return false
	}
	for combo := range expected {
		if !observed[combo] {
			return false
		}
	}
	return true
}

// seedAudit projects the root package's live inventory into an audit-shaped
// seed with empty decision fields; the seed is intentionally invalid until
// reviewed.
func seedAudit(inv []Observation, root string) *auditDoc {
	records := make([]auditRecord, 0, len(inv))
	for _, o := range inv {
		if o.Feature.Package != root {
			continue
		}
		records = append(records, auditRecord{
			ID:                 o.Feature.ID,
			Kind:               o.Feature.Kind,
			Owner:              o.Feature.Owner,
			Name:               o.Feature.Name,
			Signature:          o.Feature.Signature,
			DownstreamEvidence: []string{},
		})
	}
	return &auditDoc{SchemaVersion: 1, AuditedAt: "", Records: records}
}

// runConfig carries the invocation directory, build configurations, target
// profiles, and public packages for one run.
type runConfig struct {
	Dir            string
	Configurations []Configuration
	Profiles       []Profile
	Packages       []string
}

// resolve fills each unset field with its documented default.
func (c runConfig) resolve() runConfig {
	if c.Dir == "" {
		c.Dir = "."
	}
	if len(c.Configurations) == 0 {
		c.Configurations = DefaultConfigurations()
	}
	if len(c.Profiles) == 0 {
		c.Profiles = DefaultProfiles()
	}
	if len(c.Packages) == 0 {
		c.Packages = PublicPackages()
	}
	return c
}

// root is the package the audit and its deprecation notices are scoped to:
// the first entry in canonical package order.
func (c runConfig) root() string { return c.Packages[0] }

// schema projects the run's policy into the shape a baseline must match.
func (c runConfig) schema() baselineSchema {
	return baselineSchema{
		packages: c.Packages,
		configs:  configurationNames(c.Configurations),
		profiles: profileNames(c.Profiles),
	}
}

// baselineSchema is the policy a baseline document is validated against: the
// exact packages it must record, and the configuration and profile names its
// presence sets may name.
type baselineSchema struct {
	packages []string
	configs  []string
	profiles []string
}

// main exits with the deterministic contract code.
func main() {
	os.Exit(run(context.Background(), runConfig{Dir: "."}, os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the CLI contract: default check mode plus -list, -audit-seed,
// -baseline, and -audit. Success writes one minified JSON document to stdout;
// failure writes exactly one minified ax.Error envelope to stderr.
func run(ctx context.Context, cfg runConfig, args []string, stdout, stderr io.Writer) int {
	flagSet := flag.NewFlagSet("surfacecheck", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)
	list := flagSet.Bool("list", false, "print the generated live-baseline candidate")
	seed := flagSet.Bool("audit-seed", false, "print an audit-shaped seed with empty decision fields")
	update := flagSet.Bool("update", false, "regenerate the baseline file from the current tree")
	baselineFlag := flagSet.String("baseline", "", "check against an alternate baseline")
	auditFlag := flagSet.String("audit", "", "check against an alternate audit")
	if err := flagSet.Parse(args); err != nil {
		return emitFailure(ctx, stderr, codeArtifact, "invalid surfacecheck flags",
			"fix the command line: "+err.Error(), contract.ExitValidation, err)
	}
	if flagSet.NArg() != 0 {
		err := fmt.Errorf("unexpected positional arguments: %s", strings.Join(flagSet.Args(), " "))
		return emitFailure(ctx, stderr, codeArtifact, "invalid surfacecheck arguments",
			"remove the positional arguments", contract.ExitValidation, err)
	}

	cfg = cfg.resolve()
	universe := DefaultCombinations(cfg.Configurations, cfg.Profiles)
	generate := *list || *seed || *update

	var base *baselineDoc
	var audit *auditDoc
	if !generate {
		var code int
		base, audit, code = loadArtifacts(ctx, stderr, cfg, *baselineFlag, *auditFlag)
		if code != 0 {
			return code
		}
	}

	scans, err := scanAll(ctx, cfg.Dir, cfg.Configurations, cfg.Profiles, cfg.Packages)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return emitFailure(ctx, stderr, codePermission, "permission denied executing the required tooling",
				"check execute and read permissions on the Go toolchain and module", contract.ExitAuth, err)
		}
		return emitFailure(ctx, stderr, codeInternal, "unexpected internal failure scanning the public surface",
			"", contract.ExitInternal, err)
	}
	inv, drift := reconcile(scans, cfg.root())
	if len(drift) > 0 {
		return emitDrift(ctx, stderr, suggestionsOf(drift))
	}
	if *seed {
		return writeDocument(ctx, stdout, stderr, seedAudit(inv, cfg.root()))
	}
	generated, drift := baselineFromInventory(inv, cfg, universe)
	if len(drift) > 0 {
		return emitDrift(ctx, stderr, suggestionsOf(drift))
	}
	switch {
	case *list:
		return writeDocument(ctx, stdout, stderr, generated)
	case *update:
		return updateBaseline(ctx, stdout, stderr, cfg.Dir, *baselineFlag, generated)
	}

	drift, gapErr := checkAgainstArtifacts(cfg, inv, base, audit, scans, universe)
	if gapErr != nil {
		return emitFailure(ctx, stderr, codeInternal,
			"unexpected internal failure checking deprecation notices",
			"", contract.ExitInternal, gapErr)
	}
	if len(drift) > 0 {
		return emitDrift(ctx, stderr, suggestionsOf(drift))
	}
	return writeDocument(ctx, stdout, stderr, &gateResult{
		Status:                "pass",
		FeaturesChecked:       len(inv),
		AuditRecordsChecked:   len(audit.Records),
		PackagesChecked:       len(cfg.Packages),
		ConfigurationsChecked: len(cfg.Configurations),
		ProfilesChecked:       len(cfg.Profiles),
	})
}

// checkAgainstArtifacts runs the three reviewed-artifact checks — baseline
// diff, audit cross-validation, and deprecation notices — and returns their
// combined drift in canonical order.
func checkAgainstArtifacts(cfg runConfig, inv []Observation, base *baselineDoc,
	audit *auditDoc, scans []combinationScan, universe []Combination) ([]DriftItem, error) {
	drift := diffBaseline(inv, base, universe, cfg.root())
	drift = append(drift, crossValidateAudit(base, audit, cfg.root())...)
	gaps, err := deprecationGaps(cfg.Dir, audit, scans, cfg.root())
	if err != nil {
		return nil, err
	}
	drift = append(drift, gaps...)
	return drift, nil
}

// suggestionsOf renders drift into the sorted suggestion strings the error
// envelope carries.
func suggestionsOf(drift []DriftItem) []string {
	sortDrift(drift)
	suggestions := make([]string, len(drift))
	for i, d := range drift {
		suggestions[i] = d.String()
	}
	return suggestions
}

// updateBaseline rewrites the committed baseline from the current tree.
//
// The file is written indented and newline-terminated, unlike the minified
// documents the stdout contract requires: it is a reviewed artifact whose
// whole purpose is a readable line-by-line diff.
func updateBaseline(ctx context.Context, stdout, stderr io.Writer,
	dir, baselinePath string, generated *baselineDoc) int {
	if baselinePath == "" {
		baselinePath = defaultBaselinePath
	}
	if !filepath.IsAbs(baselinePath) {
		baselinePath = filepath.Join(dir, baselinePath)
	}
	payload, err := json.MarshalIndent(generated, "", "  ")
	if err != nil {
		return emitFailure(ctx, stderr, codeInternal, "unexpected internal failure encoding the baseline",
			"", contract.ExitInternal, err)
	}
	if writeErr := os.WriteFile(baselinePath, append(payload, '\n'), 0o600); writeErr != nil {
		if errors.Is(writeErr, fs.ErrPermission) {
			return emitFailure(ctx, stderr, codePermission, "permission denied writing the baseline artifact",
				"check write permissions on the baseline artifact", contract.ExitAuth, writeErr)
		}
		return emitFailure(ctx, stderr, codeArtifact, "the baseline artifact could not be written",
			"check that the baseline path is writable", contract.ExitValidation, writeErr)
	}
	return writeDocument(ctx, stdout, stderr, &updateResult{
		Status:          "updated",
		Baseline:        baselinePath,
		FeaturesWritten: featureCount(generated),
	})
}

// featureCount totals the features recorded across every package.
func featureCount(doc *baselineDoc) int {
	total := 0
	for _, pkg := range doc.Packages {
		total += len(pkg.Features)
	}
	return total
}

// loadArtifacts reads, parses, and validates the baseline and audit
// documents, emitting the failure envelope and returning a non-zero exit code
// on any problem.
func loadArtifacts(ctx context.Context, stderr io.Writer,
	cfg runConfig, baselinePath, auditPath string) (*baselineDoc, *auditDoc, int) {
	if baselinePath == "" {
		baselinePath = defaultBaselinePath
	}
	if auditPath == "" {
		auditPath = defaultAuditPath
	}
	schema := cfg.schema()
	parse := func(data []byte) (*baselineDoc, error) { return parseBaseline(data, schema) }
	base, code := loadArtifact[baselineDoc](ctx, stderr, cfg.Dir, baselinePath, "baseline", parse)
	if code != 0 {
		return nil, nil, code
	}
	audit, code := loadArtifact[auditDoc](ctx, stderr, cfg.Dir, auditPath, "audit", parseAudit)
	if code != 0 {
		return nil, nil, code
	}
	return base, audit, 0
}

// loadArtifact reads and strictly parses one artifact, classifying missing or
// malformed content as invalid repository input and permission denials as
// authentication/permission failures.
func loadArtifact[T any](ctx context.Context, stderr io.Writer,
	dir, path, label string, parse func([]byte) (*T, error)) (*T, int) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	data, err := readArtifact(path)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return nil, emitFailure(ctx, stderr, codePermission, "permission denied reading the "+label+" artifact",
				"check read permissions on the "+label+" artifact", contract.ExitAuth, err)
		}
		return nil, emitFailure(ctx, stderr, codeArtifact, "the "+label+" artifact is missing or unreadable",
			"restore the reviewed "+label+" artifact or pass its path explicitly", contract.ExitValidation, err)
	}
	doc, err := parse(data)
	if err != nil {
		return nil, emitFailure(ctx, stderr, codeArtifact, "the "+label+" artifact is malformed or schema-invalid",
			err.Error(), contract.ExitValidation, err)
	}
	return doc, 0
}

// readArtifact reads path under the 1 MiB bound.
func readArtifact(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxArtifactBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxArtifactBytes {
		return nil, fmt.Errorf("artifact exceeds %d bytes", maxArtifactBytes)
	}
	return data, nil
}

// writeDocument marshals v as minified JSON plus one newline on stdout.
func writeDocument(ctx context.Context, stdout, stderr io.Writer, v any) int {
	payload, err := json.Marshal(v)
	if err != nil {
		return emitFailure(ctx, stderr, codeInternal, "unexpected internal failure encoding the result",
			"", contract.ExitInternal, err)
	}
	if _, writeErr := stdout.Write(append(payload, '\n')); writeErr != nil {
		return emitFailure(ctx, stderr, codeInternal, "unexpected internal failure writing the result",
			"", contract.ExitInternal, writeErr)
	}
	return contract.ExitSuccess
}

// emitDrift writes the surface_drift envelope with sorted suggestions.
func emitDrift(ctx context.Context, stderr io.Writer, suggestions []string) int {
	env := contract.NewError(ctx, codeDrift, "public surface differs from the reviewed baseline",
		contract.WithErrorTool("surfacecheck"),
		contract.WithErrorVersion(toolVersion()),
		contract.WithErrorExitCode(contract.ExitValidation),
		contract.WithActionableFix("review the sorted drift and update source, audit, and baseline intentionally"),
		contract.WithSuggestions(suggestions...),
		contract.WithRetryable(false))
	if err := contract.WriteError(stderr, env); err != nil {
		return contract.ExitInternal
	}
	return contract.ExitValidation
}

// emitFailure writes one deterministic error envelope and returns the exit
// code. The underlying cause is attached for debugging but never serialized,
// so no host paths leak into output.
func emitFailure(ctx context.Context, stderr io.Writer, code, message, suggestion string, exit int, cause error) int {
	opts := []contract.ErrorOption{
		contract.WithErrorTool("surfacecheck"),
		contract.WithErrorVersion(toolVersion()),
		contract.WithErrorExitCode(exit),
		contract.WithRetryable(false),
		contract.WithErrorCause(cause),
	}
	if suggestion != "" {
		opts = append(opts, contract.WithSuggestions(suggestion))
	}
	env := contract.NewError(ctx, code, message, opts...)
	if err := contract.WriteError(stderr, env); err != nil {
		return contract.ExitInternal
	}
	return exit
}

// toolVersion reports the module version embedded at build time, falling back
// to "dev" for go run and test binaries so output stays deterministic.
func toolVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
