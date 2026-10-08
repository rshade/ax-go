package unreadfield

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"time"
)

// Run loads every package under dir (./..., tests included) once per
// configuration, analyzes each unit, and intersects the results: a field is
// reported only when it is a finding in every configuration in which a
// literal assigns it (research R3). Positions are slash-separated and
// relative to dir.
//
// Each configuration is bounded by perConfigTimeout. A configuration that
// cannot be listed, parsed, or type-checked fails the whole run with
// *AnalysisError, which wraps context.DeadlineExceeded on a timeout (including
// one that fires while go list runs), context.Canceled on cancellation, and
// fs.ErrPermission on a permission failure.
func Run(ctx context.Context, dir string, configs []Configuration, perConfigTimeout time.Duration) (Report, error) {
	return runWith(ctx, dir, configs, perConfigTimeout, goList)
}

func runWith(
	ctx context.Context, dir string, configs []Configuration, timeout time.Duration, list lister,
) (Report, error) {
	root, err := filepath.Abs(dir)
	if err != nil {
		return Report{}, fmt.Errorf("resolving %s: %w", dir, err)
	}
	acc := accumulator{
		assignedIn: make(map[FieldKey]int),
		findingIn:  make(map[FieldKey]int),
		findings:   make(map[FieldKey]*Finding),
		packages:   make(map[string]bool),
	}
	for _, cfg := range configs {
		results, cfgErr := analyzeConfiguration(ctx, root, cfg, timeout, list)
		if cfgErr != nil {
			return Report{}, &AnalysisError{Configuration: cfg.Name, Err: cfgErr}
		}
		for key, res := range results {
			acc.add(key, res)
		}
	}
	return acc.report(len(configs)), nil
}

// analyzeConfiguration returns each unit's relativized result, keyed by unit.
func analyzeConfiguration(
	ctx context.Context, root string, cfg Configuration, timeout time.Duration, list lister,
) (map[string]Result, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	units, err := loadUnits(ctx, root, cfg.Tags, list)
	if err != nil {
		return nil, err
	}
	results := make(map[string]Result, len(units))
	for _, u := range units {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("analyzing %s: %w", u.key, ctxErr)
		}
		results[u.key] = relativize(Analyze(u.fset, u.files, u.pkg, u.info), root)
	}
	return results, nil
}

func relativize(res Result, root string) Result {
	for i := range res.Findings {
		f := &res.Findings[i]
		f.Declared.File = relativePath(root, f.Declared.File)
		for j := range f.Assigned {
			f.Assigned[j].File = relativePath(root, f.Assigned[j].File)
		}
	}
	return res
}

func relativePath(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// accumulator intersects per-configuration results.
type accumulator struct {
	assignedIn map[FieldKey]int
	findingIn  map[FieldKey]int
	findings   map[FieldKey]*Finding
	packages   map[string]bool
}

func (acc *accumulator) add(unitKey string, res Result) {
	acc.packages[unitKey] = true
	for _, key := range res.Assigned {
		acc.assignedIn[key]++
	}
	for _, f := range res.Findings {
		acc.findingIn[f.Key]++
		prior, ok := acc.findings[f.Key]
		if !ok {
			acc.findings[f.Key] = &Finding{Key: f.Key, Declared: f.Declared, Assigned: slices.Clone(f.Assigned)}
			continue
		}
		prior.Assigned = append(prior.Assigned, f.Assigned...)
		slices.SortFunc(prior.Assigned, comparePositions)
		prior.Assigned = slices.Compact(prior.Assigned)
	}
}

func (acc *accumulator) report(configurations int) Report {
	report := Report{Configurations: configurations, Packages: len(acc.packages), Fields: len(acc.assignedIn)}
	for key, f := range acc.findings {
		if acc.findingIn[key] == acc.assignedIn[key] {
			report.Findings = append(report.Findings, *f)
		}
	}
	slices.SortFunc(report.Findings, compareFindings)
	return report
}
