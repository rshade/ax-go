package unreadfield

import (
	"reflect"
	"slices"
	"testing"
)

func findingFields(res Result) []string {
	fields := make([]string, len(res.Findings))
	for i, f := range res.Findings {
		fields[i] = f.Key.Type + "." + f.Key.Field
	}
	return fields
}

func TestAnalyzeFixtures(t *testing.T) {
	units, err := loadUnits(t.Context(), "testdata/fixtures", nil, goList)
	if err != nil {
		t.Fatalf("loadUnits: %v", err)
	}
	byKey := make(map[string]unit, len(units))
	for _, u := range units {
		byKey[u.key] = u
	}

	tests := []struct {
		pkg  string
		want []string
	}{
		// Spec User Story 1 acceptance shapes.
		{pkg: "keyed", want: []string{"row.unread"}},
		{pkg: "readloop"},
		{pkg: "addrof"},
		{pkg: "selfassign", want: []string{"state.count"}},
		{pkg: "compound"},
		{pkg: "positional", want: []string{"pair.left", "pair.right"}},
		{pkg: "promoted"},
		// Research R4 store forms.
		{pkg: "storeforms", want: []string{"ranged.key"}},
		// Research R5 whole-value uses.
		{pkg: "escapeeq"},
		{pkg: "escapereflect"},
		{pkg: "escapeiface"},
		{pkg: "escapenested"},
		{pkg: "escapecontainer", want: []string{"row.skip"}},
		{pkg: "samepkgcall", want: []string{"job.retries"}},
		{pkg: "dynamiccall"},
		{pkg: "assignrhs", want: []string{"settings.port"}},
		// Research R6 scope rules.
		{pkg: "exportedtype", want: []string{"Options.retries"}},
		{pkg: "unexportedtype", want: []string{"result.Status"}},
		{pkg: "blank"},
		{pkg: "generated"},
		{pkg: "alias", want: []string{"impl.cache"}},
		{pkg: "generic", want: []string{"box.label"}},
		{pkg: "anontable", want: []string{
			"row@anontable.go:18:7.hidden",
			"struct{...}@anontable.go:4:13.expectError",
		}},
	}
	if len(tests) != len(units) {
		t.Errorf("table covers %d packages, fixture module has %d", len(tests), len(units))
	}
	for _, tc := range tests {
		t.Run(tc.pkg, func(t *testing.T) {
			u, ok := byKey["fixtures/"+tc.pkg]
			if !ok {
				t.Fatalf("fixture package %s did not load", tc.pkg)
			}
			res := Analyze(u.fset, u.files, u.pkg, u.info)
			if got := findingFields(res); !slices.Equal(got, tc.want) {
				t.Errorf("findings = %q, want %q", got, tc.want)
			}
			assigned := make(map[FieldKey]bool, len(res.Assigned))
			for _, k := range res.Assigned {
				assigned[k] = true
			}
			for _, f := range res.Findings {
				if !assigned[f.Key] {
					t.Errorf("finding %v is not in Assigned", f.Key)
				}
				if len(f.Assigned) == 0 {
					t.Errorf("finding %v has no assignment positions", f.Key)
				}
			}
			if !slices.IsSortedFunc(res.Findings, compareFindings) {
				t.Error("Findings are not sorted by key")
			}
			if again := Analyze(u.fset, u.files, u.pkg, u.info); !reflect.DeepEqual(again, res) {
				t.Error("a second Analyze call returned a different result")
			}
		})
	}
}

func TestAnalyzeAssignmentPositions(t *testing.T) {
	units, err := loadUnits(t.Context(), "testdata/fixtures", nil, goList)
	if err != nil {
		t.Fatalf("loadUnits: %v", err)
	}
	for _, u := range units {
		if u.key != "fixtures/positional" {
			continue
		}
		res := Analyze(u.fset, u.files, u.pkg, u.info)
		left := res.Findings[0]
		if left.Declared.Line != 4 || len(left.Assigned) != 1 || left.Assigned[0].Line != 9 {
			t.Errorf("pair.left declared %v assigned %v, want line 4 and line 9", left.Declared, left.Assigned)
		}
		return
	}
	t.Fatal("fixtures/positional did not load")
}
