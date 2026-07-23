package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Feature kinds from the audit schema contract.
const (
	kindConst           = "const"
	kindVar             = "var"
	kindFunc            = "func"
	kindType            = "type"
	kindField           = "field"
	kindInterfaceMethod = "interface-method"
	kindMethod          = "method"
)

// Access review metadata; never part of a feature's identity.
const (
	accessDirect   = "direct"
	accessPromoted = "promoted"
	accessAlias    = "alias"
)

// Drift kinds from the data model.
const (
	driftAdded              = "added"
	driftMissing            = "missing"
	driftSignatureChanged   = "signature-changed"
	driftProfileDivergent   = "profile-divergent"
	driftAuditMissing       = "audit-missing"
	driftAuditStateInvalid  = "audit-state-invalid"
	driftDeprecationMissing = "deprecation-missing"
)

// Audit lifecycle states.
const (
	lifecycleLive       = "live"
	lifecycleDeprecated = "deprecated"
	lifecycleRemovable  = "removable"
	lifecycleRemoved    = "removed"
)

// Supported target operating systems and architectures.
const (
	goosLinux   = "linux"
	goosDarwin  = "darwin"
	goosWindows = "windows"
	goarchAMD64 = "amd64"
	goarchARM64 = "arm64"
)

// Recurring drift detail strings.
const (
	detailActiveAuditRecord = "active audit record"
	detailAbsent            = "absent"
	detailDeprecationNotice = "Deprecated: paragraph on the source declaration"
)

// Profile is one supported compiler selection used to verify that the root
// API is platform-invariant. The host process stays host-built; profile
// values are passed only to child go list commands.
type Profile struct {
	GOOS   string
	GOARCH string
}

// String returns the canonical "goos/goarch" spelling.
func (p Profile) String() string { return p.GOOS + "/" + p.GOARCH }

// DefaultProfiles returns the six supported target profiles: the Cartesian
// product of linux/darwin/windows and amd64/arm64. A fresh slice is returned
// on every call so callers cannot mutate shared state.
func DefaultProfiles() []Profile {
	return []Profile{
		{GOOS: goosLinux, GOARCH: goarchAMD64},
		{GOOS: goosLinux, GOARCH: goarchARM64},
		{GOOS: goosDarwin, GOARCH: goarchAMD64},
		{GOOS: goosDarwin, GOARCH: goarchARM64},
		{GOOS: goosWindows, GOARCH: goarchAMD64},
		{GOOS: goosWindows, GOARCH: goarchARM64},
	}
}

// Feature is one compiler-visible package declaration or selector exposed by
// the scanned root package. ID is the canonical public-selector identity;
// Access is review metadata (direct, promoted, or alias) and never part of
// the identity.
type Feature struct {
	ID        string
	Kind      string
	Owner     string
	Name      string
	Signature string
	Access    string
	sourceID  string
}

// DriftItem is one deterministic validation difference between source,
// profiles, baseline, and audit.
type DriftItem struct {
	ID       string
	Drift    string
	Expected string
	Actual   string
}

// String renders the canonical suggestion form "<drift> <id>".
func (d DriftItem) String() string { return d.Drift + " " + d.ID }

// sortDrift orders items by (id, drift, expected, actual) per the data model.
func sortDrift(d []DriftItem) {
	sort.Slice(d, func(i, j int) bool {
		if d[i].ID != d[j].ID {
			return d[i].ID < d[j].ID
		}
		if d[i].Drift != d[j].Drift {
			return d[i].Drift < d[j].Drift
		}
		if d[i].Expected != d[j].Expected {
			return d[i].Expected < d[j].Expected
		}
		return d[i].Actual < d[j].Actual
	})
}

// maxGoListBytes bounds the decoded go list stream so a pathological dep
// graph cannot exhaust memory.
const maxGoListBytes = 64 << 20

// limitedWriter fails closed once max bytes have been written.
type limitedWriter struct {
	w   io.Writer
	n   int64
	max int64
	err error
}

// Write implements io.Writer with a hard byte ceiling.
func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n+int64(len(p)) > l.max {
		l.err = fmt.Errorf("output exceeds %d bytes", l.max)
		return 0, l.err
	}
	l.n += int64(len(p))
	return l.w.Write(p)
}

// profileScan is the inventory observed for one target profile.
type profileScan struct {
	profile  Profile
	features []Feature
	files    []string
}

// scanAll scans dir once per profile, sequentially, and returns each
// profile's canonical inventory.
func scanAll(ctx context.Context, dir string, profiles []Profile) ([]profileScan, error) {
	scans := make([]profileScan, 0, len(profiles))
	for _, p := range profiles {
		scan, err := scanProfile(ctx, dir, p)
		if err != nil {
			return nil, fmt.Errorf("scanning profile %s: %w", p, err)
		}
		scans = append(scans, scan)
	}
	return scans, nil
}

// listedPackage is the subset of go list -json fields the importer needs.
type listedPackage struct {
	ImportPath string   `json:"ImportPath"`
	Name       string   `json:"Name"`
	Export     string   `json:"Export"`
	DepOnly    bool     `json:"DepOnly"`
	GoFiles    []string `json:"GoFiles"`
	CgoFiles   []string `json:"CgoFiles"`
}

// scanProfile runs go list -deps -export -json for one target profile, loads
// the root package from compiler export data, and inventories its complete
// compiler-visible surface.
func scanProfile(ctx context.Context, dir string, p Profile) (profileScan, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-export", "-json", ".")
	cmd.Dir = dir
	cmd.Env = profileEnv(p)
	var stdout, stderr bytes.Buffer
	limited := &limitedWriter{w: &stdout, max: maxGoListBytes}
	cmd.Stdout = limited
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if limited.err != nil {
			return profileScan{}, limited.err
		}
		return profileScan{}, fmt.Errorf("go list: %w", err)
	}
	if limited.err != nil {
		return profileScan{}, limited.err
	}

	exports := map[string]string{}
	var rootPath string
	var rootFiles []string
	dec := json.NewDecoder(&stdout)
	for {
		var lp listedPackage
		err := dec.Decode(&lp)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return profileScan{}, fmt.Errorf("decoding go list output: %w", err)
		}
		if lp.Export != "" {
			exports[lp.ImportPath] = lp.Export
		}
		if !lp.DepOnly {
			rootPath = lp.ImportPath
			rootFiles = append(rootFiles, lp.GoFiles...)
			rootFiles = append(rootFiles, lp.CgoFiles...)
		}
	}
	if rootPath == "" {
		return profileScan{}, errors.New("root package not found in go list output")
	}
	sort.Strings(rootFiles)

	lookup := func(path string) (io.ReadCloser, error) {
		export, ok := exports[path]
		if !ok {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(export)
	}
	fset := token.NewFileSet()
	pkg, err := importer.ForCompiler(fset, "gc", lookup).Import(rootPath)
	if err != nil {
		return profileScan{}, fmt.Errorf("importing root package: %w", err)
	}
	return profileScan{profile: p, features: collectFeatures(pkg), files: rootFiles}, nil
}

// profileEnv returns the process environment with GOOS/GOARCH pinned to the
// target profile so the child go list compiles for that selection.
func profileEnv(p Profile) []string {
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "GOOS=") || strings.HasPrefix(e, "GOARCH=") {
			continue
		}
		env = append(env, e)
	}
	return append(env, "GOOS="+p.GOOS, "GOARCH="+p.GOARCH)
}

// reconcile verifies profile invariance: every profile must yield the same
// canonical ID/signature set. On divergence it fails closed with a nil
// inventory and one drift item per divergent feature.
func reconcile(scans []profileScan, profiles []Profile) ([]Feature, []DriftItem) {
	if len(scans) == 0 {
		return nil, nil
	}
	if drift := profileDrift(scans, profiles); len(drift) > 0 {
		return nil, drift
	}
	return scans[0].features, nil
}

// profileDrift returns one drift item per feature whose presence or signature
// differs across the scanned profiles.
func profileDrift(scans []profileScan, profiles []Profile) []DriftItem {
	byProfile := signatureMaps(scans)
	var drift []DriftItem
	for _, id := range unionIDs(scans) {
		if item, divergent := divergenceItem(id, byProfile, profiles); divergent {
			drift = append(drift, item)
		}
	}
	sortDrift(drift)
	return drift
}

// unionIDs returns the sorted union of feature IDs across all scans.
func unionIDs(scans []profileScan) []string {
	union := map[string]bool{}
	for _, s := range scans {
		for _, f := range s.features {
			union[f.ID] = true
		}
	}
	ids := make([]string, 0, len(union))
	for id := range union {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// signatureMaps projects each scan into an ID-to-signature map.
func signatureMaps(scans []profileScan) []map[string]string {
	byProfile := make([]map[string]string, len(scans))
	for i, s := range scans {
		byProfile[i] = map[string]string{}
		for _, f := range s.features {
			byProfile[i][f.ID] = f.Signature
		}
	}
	return byProfile
}

// divergenceItem reports whether one feature is invariant across profiles and,
// when it is not, the profile-divergent drift item for it.
func divergenceItem(id string, byProfile []map[string]string, profiles []Profile) (DriftItem, bool) {
	first, present := byProfile[0][id]
	allSame := present
	for i := 1; i < len(byProfile) && allSame; i++ {
		sig, ok := byProfile[i][id]
		allSame = ok && sig == first
	}
	if allSame {
		return DriftItem{}, false
	}
	return DriftItem{
		ID:       id,
		Drift:    driftProfileDivergent,
		Expected: "present in all profiles",
		Actual:   divergenceActual(id, byProfile, profiles, first),
	}, true
}

// divergenceActual describes the first profile, in declaration order, whose
// observation of id breaks invariance.
func divergenceActual(id string, byProfile []map[string]string, profiles []Profile, first string) string {
	for i, p := range profiles {
		sig, present := byProfile[i][id]
		if !present {
			return "absent in " + p.String()
		}
		if i > 0 && sig != first {
			return "signature differs in " + p.String()
		}
	}
	return ""
}

// diffBaseline compares the live inventory against the approved baseline:
// source-only IDs are added, baseline-only IDs are missing, and equal IDs
// with different signatures are signature-changed.
func diffBaseline(live []Feature, base *baselineDoc) []DriftItem {
	liveMap := map[string]string{}
	for _, f := range live {
		liveMap[f.ID] = f.Signature
	}
	baseMap := map[string]string{}
	for _, f := range base.Features {
		baseMap[f.ID] = f.Signature
	}
	var drift []DriftItem
	for _, f := range live {
		sig, ok := baseMap[f.ID]
		if !ok {
			drift = append(drift, DriftItem{ID: f.ID, Drift: driftAdded, Expected: "", Actual: f.Signature})
			continue
		}
		if sig != f.Signature {
			drift = append(drift, DriftItem{ID: f.ID, Drift: driftSignatureChanged, Expected: sig, Actual: f.Signature})
		}
	}
	for _, f := range base.Features {
		if _, ok := liveMap[f.ID]; !ok {
			drift = append(drift, DriftItem{ID: f.ID, Drift: driftMissing, Expected: f.Signature, Actual: ""})
		}
	}
	sortDrift(drift)
	return drift
}

// crossValidateAudit enforces the audit/baseline completeness invariants:
// every baseline ID needs an active audit record, active records must appear
// in the live baseline, and removed records must not.
func crossValidateAudit(base *baselineDoc, audit *auditDoc) []DriftItem {
	active := map[string]bool{}
	removed := map[string]bool{}
	for _, r := range audit.Records {
		if r.Lifecycle == lifecycleRemoved {
			removed[r.ID] = true
			continue
		}
		active[r.ID] = true
	}
	inBaseline := map[string]bool{}
	var drift []DriftItem
	for _, f := range base.Features {
		inBaseline[f.ID] = true
		if removed[f.ID] {
			drift = append(drift, DriftItem{
				ID:       f.ID,
				Drift:    driftAuditStateInvalid,
				Expected: "absent from live baseline",
				Actual:   "removed audit record",
			})
			continue
		}
		if !active[f.ID] {
			drift = append(drift, DriftItem{
				ID:       f.ID,
				Drift:    driftAuditMissing,
				Expected: detailActiveAuditRecord,
				Actual:   detailAbsent,
			})
		}
	}
	for id := range active {
		if !inBaseline[id] {
			drift = append(drift, DriftItem{
				ID:       id,
				Drift:    driftAuditStateInvalid,
				Expected: "absent from live baseline or transitioned to removed",
				Actual:   detailActiveAuditRecord,
			})
		}
	}
	sortDrift(drift)
	return drift
}

// collector accumulates canonical features for one loaded root package.
type collector struct {
	pkg     *types.Package
	qf      types.Qualifier
	feats   map[string]Feature
	visited map[*types.Named]bool
	declIDs map[types.Object]string
}

// collectFeatures inventories the complete compiler-visible surface of pkg:
// exported declarations, direct and promoted members, complete interface
// method sets, alias-attributed members, and reachable hidden concrete types.
func collectFeatures(pkg *types.Package) []Feature {
	c := &collector{
		pkg:     pkg,
		qf:      qualifier(pkg),
		feats:   map[string]Feature{},
		visited: map[*types.Named]bool{},
		declIDs: declarationIDs(pkg),
	}
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if obj == nil || !obj.Exported() {
			continue
		}
		c.object(obj)
	}
	feats := make([]Feature, 0, len(c.feats))
	for _, f := range c.feats {
		feats = append(feats, f)
	}
	sort.Slice(feats, func(i, j int) bool { return feats[i].ID < feats[j].ID })
	return feats
}

// declarationIDs indexes member objects by the declaration that owns their
// documentation. Promoted and alias selectors reuse these same objects, so
// the index lets lifecycle validation follow a public selector back to its
// single source declaration without approximating Go's selection rules.
func declarationIDs(pkg *types.Package) map[types.Object]string {
	ids := map[types.Object]string{}
	for _, name := range pkg.Scope().Names() {
		obj := pkg.Scope().Lookup(name)
		typeName, ok := obj.(*types.TypeName)
		if !ok || typeName.IsAlias() {
			continue
		}
		named, ok := typeName.Type().(*types.Named)
		if !ok {
			continue
		}
		owner := typeName.Name()
		for i := range named.NumMethods() {
			method := named.Method(i)
			ids[method] = declaredMethodID(owner, method)
		}
		switch underlying := named.Underlying().(type) {
		case *types.Struct:
			for i := range underlying.NumFields() {
				field := underlying.Field(i)
				ids[field] = kindField + ":" + owner + "." + field.Name()
			}
		case *types.Interface:
			for i := range underlying.NumExplicitMethods() {
				method := underlying.ExplicitMethod(i)
				ids[method] = kindInterfaceMethod + ":" + owner + "." + method.Name()
			}
		}
	}
	return ids
}

// declaredMethodID returns the canonical ID of method's receiver declaration.
func declaredMethodID(owner string, method *types.Func) string {
	id := kindMethod + ":"
	if sig, ok := method.Type().(*types.Signature); ok && sig.Recv() != nil {
		if _, pointer := sig.Recv().Type().(*types.Pointer); pointer {
			id += "*"
		}
	}
	return id + owner + "." + method.Name()
}

// selectorSourceID returns the source declaration ID only when it differs
// from the public selector ID. An empty value means the selector is declared
// directly under its public identity.
func (c *collector) selectorSourceID(obj types.Object, selectorID string) string {
	if declarationID := c.declIDs[obj]; declarationID != "" && declarationID != selectorID {
		return declarationID
	}
	return ""
}

// qualifier prints root package types unqualified and every other package by
// import path so distinct packages with the same declared name cannot collapse
// to the same canonical signature.
func qualifier(root *types.Package) types.Qualifier {
	return func(p *types.Package) string {
		if p == root {
			return ""
		}
		return p.Path()
	}
}

// add records f unless an identical ID was already recorded; iteration order
// is deterministic, so the first writer wins deterministically.
func (c *collector) add(f Feature) {
	if _, ok := c.feats[f.ID]; !ok {
		c.feats[f.ID] = f
	}
}

// object inventories one exported package-scope object and walks the types it
// exposes.
func (c *collector) object(obj types.Object) {
	switch obj := obj.(type) {
	case *types.Const:
		c.add(Feature{ID: kindConst + ":" + obj.Name(), Kind: kindConst, Name: obj.Name(),
			Signature: types.TypeString(obj.Type(), c.qf), Access: accessDirect})
	case *types.Var:
		c.add(Feature{ID: kindVar + ":" + obj.Name(), Kind: kindVar, Name: obj.Name(),
			Signature: types.TypeString(obj.Type(), c.qf), Access: accessDirect})
		c.expose(obj.Type())
	case *types.Func:
		sig, _ := obj.Type().(*types.Signature)
		c.add(Feature{ID: kindFunc + ":" + obj.Name(), Kind: kindFunc, Name: obj.Name(),
			Signature: canonicalSignature(sig, c.qf), Access: accessDirect})
		c.expose(sig)
	case *types.TypeName:
		c.typeName(obj)
	}
}

// typeName inventories an exported type declaration or alias and its members.
func (c *collector) typeName(obj *types.TypeName) {
	if obj.IsAlias() {
		rhs := types.Unalias(obj.Type())
		c.add(Feature{ID: kindType + ":" + obj.Name(), Kind: kindType, Name: obj.Name(),
			Signature: "= " + types.TypeString(rhs, c.qf), Access: accessAlias})
		if named, ok := rhs.(*types.Named); ok {
			// The alias claims the target's members under the public alias
			// selector; they are not double-counted under a hidden name.
			c.visited[named] = true
			c.members(named, obj.Name(), accessAlias)
		}
		return
	}
	named, ok := obj.Type().(*types.Named)
	if !ok {
		return
	}
	c.add(Feature{ID: kindType + ":" + obj.Name(), Kind: kindType, Name: obj.Name(),
		Signature: types.TypeString(named.Underlying(), c.qf), Access: accessDirect})
	c.members(named, obj.Name(), "")
	c.expose(named.Underlying())
}

// members inventories the exported members of named under owner. access is
// empty for defined root types (each member computes direct or promoted) or
// "alias" when members are reached through a root alias selector.
func (c *collector) members(named *types.Named, owner, access string) {
	if _, ok := named.Underlying().(*types.Interface); ok {
		c.ifaceMethods(named, owner, access)
		return
	}
	if _, ok := named.Underlying().(*types.Struct); ok {
		c.fields(named, owner, access)
	}
	c.methods(named, owner, access)
}

// memberAccess attributes how a method is reached: through an alias, by
// promotion, or directly.
func memberAccess(access string, sel *types.Selection) string {
	if access != "" {
		return access
	}
	if len(sel.Index()) > 1 {
		return accessPromoted
	}
	return accessDirect
}

// methods inventories the value and pointer-only method sets of a concrete
// named type under owner.
func (c *collector) methods(named *types.Named, owner, access string) {
	valueSet := map[string]bool{}
	msV := types.NewMethodSet(named)
	for i := range msV.Len() {
		sel := msV.At(i)
		m, isFunc := sel.Obj().(*types.Func)
		if !isFunc || !m.Exported() {
			continue
		}
		valueSet[m.Id()] = true
		c.addMethod(owner, m, false, memberAccess(access, sel))
	}
	msP := types.NewMethodSet(types.NewPointer(named))
	for i := range msP.Len() {
		sel := msP.At(i)
		m, isFunc := sel.Obj().(*types.Func)
		if !isFunc || !m.Exported() || valueSet[m.Id()] {
			continue
		}
		c.addMethod(owner, m, true, memberAccess(access, sel))
	}
}

// addMethod records one method feature and walks its signature for exposed
// hidden types.
func (c *collector) addMethod(owner string, m *types.Func, pointerOnly bool, access string) {
	id := kindMethod + ":"
	if pointerOnly {
		id += "*"
	}
	id += owner + "." + m.Name()
	sig, _ := m.Type().(*types.Signature)
	c.add(Feature{ID: id, Kind: kindMethod, Owner: owner, Name: m.Name(),
		Signature: canonicalSignature(sig, c.qf), Access: access, sourceID: c.selectorSourceID(m, id)})
	c.expose(sig)
}

// ifaceMethods inventories the complete method set of an interface type,
// including methods promoted from embedded interfaces. Method-set indices are
// flattened for interfaces, so direct versus promoted is decided against the
// interface's explicitly declared methods.
func (c *collector) ifaceMethods(named *types.Named, owner, access string) {
	iface, ok := named.Underlying().(*types.Interface)
	if !ok {
		return
	}
	explicit := map[string]bool{}
	for i := range iface.NumExplicitMethods() {
		explicit[iface.ExplicitMethod(i).Id()] = true
	}
	ms := types.NewMethodSet(named)
	for i := range ms.Len() {
		sel := ms.At(i)
		m, isFunc := sel.Obj().(*types.Func)
		if !isFunc || !m.Exported() {
			continue
		}
		sig, _ := m.Type().(*types.Signature)
		c.add(Feature{
			ID:        kindInterfaceMethod + ":" + owner + "." + m.Name(),
			Kind:      kindInterfaceMethod,
			Owner:     owner,
			Name:      m.Name(),
			Signature: canonicalSignature(sig, c.qf),
			Access:    ifaceMethodAccess(access, explicit, m),
			sourceID:  c.selectorSourceID(m, kindInterfaceMethod+":"+owner+"."+m.Name()),
		})
		c.expose(sig)
	}
}

// ifaceMethodAccess attributes an interface method: through an alias, by
// promotion from an embedded interface, or declared directly.
func ifaceMethodAccess(access string, explicit map[string]bool, m *types.Func) string {
	if access != "" {
		return access
	}
	if explicit[m.Id()] {
		return accessDirect
	}
	return accessPromoted
}

// fields inventories the direct and promoted exported fields of a struct
// type following go/types promotion rules: shallowest depth wins and
// same-depth name collisions are ambiguous and excluded.
func (c *collector) fields(named *types.Named, owner, access string) {
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return
	}
	emitted := map[string]bool{}
	seen := map[*types.Struct]bool{st: true}
	frontier := []*types.Struct{st}
	for depth := 0; len(frontier) > 0; depth++ {
		candidates, counts, next := fieldFrame(frontier, emitted, seen)
		for name, f := range candidates {
			if counts[name] > 1 {
				continue
			}
			emitted[name] = true
			c.addField(owner, name, f, fieldAccess(access, depth))
		}
		frontier = next
	}
}

// fieldFrame gathers the exported field candidates visible from one promotion
// depth together with their counts and the next embedded frontier.
func fieldFrame(frontier []*types.Struct, emitted map[string]bool,
	seen map[*types.Struct]bool) (map[string]*types.Var, map[string]int, []*types.Struct) {
	candidates := map[string]*types.Var{}
	counts := map[string]int{}
	var next []*types.Struct
	for _, s := range frontier {
		for i := range s.NumFields() {
			f := s.Field(i)
			if f.Anonymous() {
				if inner := underlyingStruct(f.Type()); inner != nil && !seen[inner] {
					next = append(next, inner)
				}
			}
			if !f.Exported() || emitted[f.Name()] {
				continue
			}
			counts[f.Name()]++
			if counts[f.Name()] == 1 {
				candidates[f.Name()] = f
			}
		}
	}
	// Mark newly reached structs only after the whole depth is gathered. The
	// same struct reached through two paths at this depth must appear twice in
	// the next frame so its fields are counted as ambiguous; structs reached
	// at a shallower depth still remain suppressed to terminate cycles.
	for _, inner := range next {
		seen[inner] = true
	}
	return candidates, counts, next
}

// fieldAccess attributes a field: through an alias, directly, or promoted
// from an embedded struct.
func fieldAccess(access string, depth int) string {
	if access != "" {
		return access
	}
	if depth == 0 {
		return accessDirect
	}
	return accessPromoted
}

// addField records one field feature and walks its type for exposed hidden
// types.
func (c *collector) addField(owner, name string, f *types.Var, access string) {
	id := kindField + ":" + owner + "." + name
	c.add(Feature{
		ID:        id,
		Kind:      kindField,
		Owner:     owner,
		Name:      name,
		Signature: types.TypeString(f.Type(), c.qf),
		Access:    access,
		sourceID:  c.selectorSourceID(f, id),
	})
	c.expose(f.Type())
}

// underlyingStruct dereferences pointers, aliases, and named types to reach a
// struct literal, or returns nil.
func underlyingStruct(t types.Type) *types.Struct {
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	t = types.Unalias(t)
	if n, ok := t.(*types.Named); ok {
		t = n.Underlying()
	}
	if s, ok := t.(*types.Struct); ok {
		return s
	}
	return nil
}

// expose walks t for reachable hidden concrete types: unexported named types
// declared in the root package whose members become selectable through an
// exported declaration. Types from other packages and already-visited types
// are not traversed.
func (c *collector) expose(t types.Type) {
	switch t := t.(type) {
	case *types.Pointer:
		c.expose(t.Elem())
	case *types.Slice:
		c.expose(t.Elem())
	case *types.Array:
		c.expose(t.Elem())
	case *types.Map:
		c.expose(t.Key())
		c.expose(t.Elem())
	case *types.Chan:
		c.expose(t.Elem())
	case *types.Signature:
		c.expose(t.Params())
		c.expose(t.Results())
	case *types.Tuple:
		for i := range t.Len() {
			c.expose(t.At(i).Type())
		}
	case *types.Alias:
		c.expose(types.Unalias(t))
	case *types.Struct:
		for i := range t.NumFields() {
			if f := t.Field(i); f.Exported() {
				c.expose(f.Type())
			}
		}
	case *types.Interface:
		for i := range t.NumMethods() {
			c.expose(t.Method(i).Type())
		}
	case *types.Named:
		obj := t.Obj()
		if obj == nil || obj.Pkg() != c.pkg || obj.Exported() || c.visited[t] {
			return
		}
		c.visited[t] = true
		c.members(t, obj.Name(), "")
	}
}

// canonicalSignature renders sig without parameter or result names so renames
// that do not affect compatibility stay invisible to the gate. Type
// parameters are re-minted fresh because NewSignatureType binds them and the
// originals are already bound to sig.
func canonicalSignature(sig *types.Signature, qf types.Qualifier) string {
	var tparams []*types.TypeParam
	if tp := sig.TypeParams(); tp != nil {
		for i := range tp.Len() {
			old := tp.At(i)
			name := types.NewTypeName(old.Obj().Pos(), nil, old.Obj().Name(), nil)
			tparams = append(tparams, types.NewTypeParam(name, old.Constraint()))
		}
	}
	stripped := types.NewSignatureType(nil, nil, tparams,
		stripNames(sig.Params()), stripNames(sig.Results()), sig.Variadic())
	return types.TypeString(stripped, qf)
}

// stripNames returns a copy of t with every variable name cleared.
func stripNames(t *types.Tuple) *types.Tuple {
	vars := make([]*types.Var, t.Len())
	for i := range vars {
		v := t.At(i)
		vars[i] = types.NewVar(v.Pos(), v.Pkg(), "", v.Type())
	}
	return types.NewTuple(vars...)
}

// deprecationGaps reports one drift item for every audit row in the
// deprecated or removable state whose source declaration lacks a valid
// Go-recognized Deprecated: paragraph.
func deprecationGaps(dir string, audit *auditDoc, scans []profileScan) ([]DriftItem, error) {
	profiles, err := deprecationProfiles(dir, scans)
	if err != nil {
		return nil, err
	}
	var drift []DriftItem
	for _, r := range audit.Records {
		if r.Lifecycle != lifecycleDeprecated && r.Lifecycle != lifecycleRemovable {
			continue
		}
		if deprecationMissing(r.ID, profiles) {
			drift = append(drift, DriftItem{
				ID:       r.ID,
				Drift:    driftDeprecationMissing,
				Expected: detailDeprecationNotice,
				Actual:   detailAbsent,
			})
		}
	}
	sortDrift(drift)
	return drift, nil
}

// deprecationProfile binds one profile's public selectors to their source
// declarations and the notices found in that profile's selected files.
type deprecationProfile struct {
	declarations map[string]string
	deprecated   map[string]bool
}

// deprecationProfiles builds the source and notice indexes for every scanned
// profile.
func deprecationProfiles(dir string, scans []profileScan) ([]deprecationProfile, error) {
	profiles := make([]deprecationProfile, len(scans))
	for i, scan := range scans {
		deprecated, err := deprecatedDecls(dir, scan.files)
		if err != nil {
			return nil, fmt.Errorf("profile %s: %w", scan.profile, err)
		}
		profiles[i] = deprecationProfile{
			declarations: declarationSources(scan.features),
			deprecated:   deprecated,
		}
	}
	return profiles, nil
}

// declarationSources maps each public selector to the declaration that owns
// its documentation in one profile.
func declarationSources(features []Feature) map[string]string {
	declarations := make(map[string]string, len(features))
	for _, feature := range features {
		declarationID := feature.sourceID
		if declarationID == "" {
			declarationID = feature.ID
		}
		declarations[feature.ID] = declarationID
	}
	return declarations
}

// deprecationMissing reports whether any selected profile lacks a notice on
// the declaration that provides id in that profile.
func deprecationMissing(id string, profiles []deprecationProfile) bool {
	for _, profile := range profiles {
		declarationID := id
		if sourceID := profile.declarations[id]; sourceID != "" {
			declarationID = sourceID
		}
		if !profile.deprecated[declarationID] {
			return true
		}
	}
	return false
}

// deprecatedDecls parses the root source files selected by one profile's go
// list result and returns the canonical IDs whose declarations carry a valid
// deprecation paragraph.
func deprecatedDecls(dir string, files []string) (map[string]bool, error) {
	deprecated := map[string]bool{}
	fset := token.NewFileSet()
	for _, name := range files {
		file, parseErr := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing %s: %w", name, parseErr)
		}
		collectDeprecatedDecls(file, deprecated)
	}
	return deprecated, nil
}

// collectDeprecatedDecls records the canonical IDs of declarations in file
// that carry a valid Deprecated: paragraph.
func collectDeprecatedDecls(file *ast.File, deprecated map[string]bool) {
	mark := func(id string, doc *ast.CommentGroup) {
		if hasDeprecatedParagraph(doc) {
			deprecated[id] = true
		}
	}
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			collectDeprecatedFunc(decl, mark)
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				collectDeprecatedSpec(decl, spec, mark)
			}
		}
	}
}

// collectDeprecatedFunc records one function or method declaration.
func collectDeprecatedFunc(decl *ast.FuncDecl, mark func(string, *ast.CommentGroup)) {
	if decl.Recv == nil {
		mark(kindFunc+":"+decl.Name.Name, decl.Doc)
		return
	}
	owner, pointer := recvBaseName(decl.Recv)
	if owner == "" {
		return
	}
	id := kindMethod + ":"
	if pointer {
		id += "*"
	}
	mark(id+owner+"."+decl.Name.Name, decl.Doc)
}

// collectDeprecatedSpec handles const/var names, type declarations, struct
// fields, and interface methods inside one general declaration spec.
func collectDeprecatedSpec(decl *ast.GenDecl, spec ast.Spec, mark func(string, *ast.CommentGroup)) {
	switch spec := spec.(type) {
	case *ast.ValueSpec:
		kind := kindVar + ":"
		if decl.Tok == token.CONST {
			kind = kindConst + ":"
		}
		for _, name := range spec.Names {
			mark(kind+name.Name, specDoc(spec.Doc, decl.Doc))
		}
	case *ast.TypeSpec:
		mark(kindType+":"+spec.Name.Name, specDoc(spec.Doc, decl.Doc))
		collectDeprecatedMembers(spec, mark)
	}
}

// collectDeprecatedMembers handles struct fields and interface methods inside
// one type declaration spec.
func collectDeprecatedMembers(spec *ast.TypeSpec, mark func(string, *ast.CommentGroup)) {
	switch typ := spec.Type.(type) {
	case *ast.StructType:
		for _, field := range typ.Fields.List {
			for _, name := range field.Names {
				mark(kindField+":"+spec.Name.Name+"."+name.Name, field.Doc)
			}
		}
	case *ast.InterfaceType:
		for _, method := range typ.Methods.List {
			for _, name := range method.Names {
				mark(kindInterfaceMethod+":"+spec.Name.Name+"."+name.Name, method.Doc)
			}
		}
	}
}

// specDoc prefers the spec's own doc comment and falls back to the enclosing
// declaration's doc for single-spec declarations.
func specDoc(spec, decl *ast.CommentGroup) *ast.CommentGroup {
	if spec != nil {
		return spec
	}
	return decl
}

// recvBaseName extracts the base type name and pointer-ness from a method
// receiver, including generic receiver index forms.
func recvBaseName(recv *ast.FieldList) (string, bool) {
	if recv == nil || len(recv.List) == 0 {
		return "", false
	}
	typ := recv.List[0].Type
	pointer := false
	if star, ok := typ.(*ast.StarExpr); ok {
		pointer = true
		typ = star.X
	}
	switch expr := typ.(type) {
	case *ast.IndexExpr:
		typ = expr.X
	case *ast.IndexListExpr:
		typ = expr.X
	}
	if ident, ok := typ.(*ast.Ident); ok {
		return ident.Name, pointer
	}
	return "", false
}

// hasDeprecatedParagraph reports whether doc contains a paragraph beginning
// with "Deprecated:", the Go-recognized deprecation convention.
func hasDeprecatedParagraph(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	for _, paragraph := range strings.Split(doc.Text(), "\n\n") {
		if strings.HasPrefix(strings.TrimSpace(paragraph), "Deprecated:") {
			return true
		}
	}
	return false
}
