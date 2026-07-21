// surfacecheck is internal maintainer/CI tooling: it scans the complete
// compiler-visible root ax surface for all supported target profiles,
// compares it against the reviewed live baseline, and cross-validates the
// permanent public-surface audit. Successful modes write one minified JSON
// document to stdout; every failure writes exactly one minified ax.Error
// envelope to stderr.
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
const maxArtifactBytes = 1 << 20

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

// baselineFeature is one current approved compiler-visible API feature.
type baselineFeature struct {
	ID        string `json:"id"`
	Signature string `json:"signature"`
}

// baselineDoc is the live operational surface projection.
type baselineDoc struct {
	SchemaVersion int               `json:"schema_version"`
	Features      []baselineFeature `json:"features"`
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
	Status              string `json:"status"`
	FeaturesChecked     int    `json:"features_checked"`
	AuditRecordsChecked int    `json:"audit_records_checked"`
	ProfilesChecked     int    `json:"profiles_checked"`
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

// parseBaseline decodes and validates a live baseline document.
func parseBaseline(data []byte) (*baselineDoc, error) {
	var doc baselineDoc
	if err := decodeStrict(data, &doc); err != nil {
		return nil, err
	}
	if err := validateBaseline(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

// validateBaseline enforces the baseline schema contract.
func validateBaseline(doc *baselineDoc) error {
	if doc.SchemaVersion != 1 {
		return fmt.Errorf("schema_version must be 1, got %d", doc.SchemaVersion)
	}
	if doc.Features == nil {
		return errors.New("features is required")
	}
	for i, f := range doc.Features {
		if f.ID == "" || f.Signature == "" {
			return fmt.Errorf("features[%d]: id and signature are required", i)
		}
		if i > 0 && doc.Features[i-1].ID >= f.ID {
			return fmt.Errorf("features must be sorted bytewise by unique id near %q", f.ID)
		}
	}
	return nil
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

// baselineFromInventory projects the live inventory into a baseline document.
func baselineFromInventory(inv []Feature) *baselineDoc {
	features := make([]baselineFeature, len(inv))
	for i, f := range inv {
		features[i] = baselineFeature{ID: f.ID, Signature: f.Signature}
	}
	return &baselineDoc{SchemaVersion: 1, Features: features}
}

// seedAudit projects the live inventory into an audit-shaped seed with empty
// decision fields; the seed is intentionally invalid until reviewed.
func seedAudit(inv []Feature) *auditDoc {
	records := make([]auditRecord, len(inv))
	for i, f := range inv {
		records[i] = auditRecord{
			ID:                 f.ID,
			Kind:               f.Kind,
			Owner:              f.Owner,
			Name:               f.Name,
			Signature:          f.Signature,
			DownstreamEvidence: []string{},
		}
	}
	return &auditDoc{SchemaVersion: 1, AuditedAt: "", Records: records}
}

// runConfig carries the invocation directory and target profiles for one run.
type runConfig struct {
	Dir      string
	Profiles []Profile
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

	dir := cfg.Dir
	if dir == "" {
		dir = "."
	}
	profiles := cfg.Profiles
	if len(profiles) == 0 {
		profiles = DefaultProfiles()
	}

	var base *baselineDoc
	var audit *auditDoc
	if !*list && !*seed {
		var code int
		base, audit, code = loadArtifacts(ctx, stderr, dir, *baselineFlag, *auditFlag)
		if code != 0 {
			return code
		}
	}

	scans, err := scanAll(ctx, dir, profiles)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return emitFailure(ctx, stderr, codePermission, "permission denied executing the required tooling",
				"check execute and read permissions on the Go toolchain and module", contract.ExitAuth, err)
		}
		return emitFailure(ctx, stderr, codeInternal, "unexpected internal failure scanning the public surface",
			"", contract.ExitInternal, err)
	}
	inv, drift := reconcile(scans, profiles)
	if len(drift) == 0 {
		switch {
		case *list:
			return writeDocument(ctx, stdout, stderr, baselineFromInventory(inv))
		case *seed:
			return writeDocument(ctx, stdout, stderr, seedAudit(inv))
		}
		drift = diffBaseline(inv, base)
		drift = append(drift, crossValidateAudit(base, audit)...)
		gaps, gapErr := deprecationGaps(dir, audit, scans)
		if gapErr != nil {
			return emitFailure(ctx, stderr, codeInternal, "unexpected internal failure checking deprecation notices",
				"", contract.ExitInternal, gapErr)
		}
		drift = append(drift, gaps...)
	}
	if len(drift) > 0 {
		sortDrift(drift)
		suggestions := make([]string, len(drift))
		for i, d := range drift {
			suggestions[i] = d.String()
		}
		return emitDrift(ctx, stderr, suggestions)
	}
	return writeDocument(ctx, stdout, stderr, &gateResult{
		Status:              "pass",
		FeaturesChecked:     len(inv),
		AuditRecordsChecked: len(audit.Records),
		ProfilesChecked:     len(profiles),
	})
}

// loadArtifacts reads, parses, and validates the baseline and audit
// documents, emitting the failure envelope and returning a non-zero exit code
// on any problem.
func loadArtifacts(ctx context.Context, stderr io.Writer,
	dir, baselinePath, auditPath string) (*baselineDoc, *auditDoc, int) {
	if baselinePath == "" {
		baselinePath = defaultBaselinePath
	}
	if auditPath == "" {
		auditPath = defaultAuditPath
	}
	base, code := loadArtifact[baselineDoc](ctx, stderr, dir, baselinePath, "baseline", parseBaseline)
	if code != 0 {
		return nil, nil, code
	}
	audit, code := loadArtifact[auditDoc](ctx, stderr, dir, auditPath, "audit", parseAudit)
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
