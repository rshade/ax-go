package schema

import (
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestIsCapability(t *testing.T) {
	cases := []struct {
		class string
		want  bool
	}{
		{"read-only", true},
		{"create", true},
		{"mutate", true},
		{"delete", true},
		{"external-network", true},
		{"admin", true},
		{"", false},
		{"Read-Only", false},
		{" mutate", false},
		{"write", false},
	}
	for _, tc := range cases {
		t.Run(tc.class, func(t *testing.T) {
			if got := IsCapability(tc.class); got != tc.want {
				t.Fatalf("IsCapability(%q) = %v, want %v", tc.class, got, tc.want)
			}
		})
	}
}

func TestLookupFlag(t *testing.T) {
	root := &cobra.Command{Use: "app"}
	root.PersistentFlags().String("region", "us", "region")
	child := &cobra.Command{Use: "deploy"}
	child.Flags().String("output", "json", "output")
	root.AddCommand(child)

	cases := []struct {
		name string
		cmd  *cobra.Command
		flag string
		want *Violation
	}{
		{name: "local flag", cmd: child, flag: "output"},
		{name: "persistent flag on owner", cmd: root, flag: "region"},
		{name: "nil command", cmd: nil, flag: "output", want: &Violation{Field: "cmd", Reason: ReasonNilCommand}},
		{name: "missing flag", cmd: child, flag: "nope", want: &Violation{Field: "flag", Reason: ReasonFlagNotFound}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			flag, violation := lookupFlag(tc.cmd, tc.flag)
			if !reflect.DeepEqual(violation, tc.want) {
				t.Fatalf("lookupFlag() violation = %+v, want %+v", violation, tc.want)
			}
			if tc.want == nil && (flag == nil || flag.Name != tc.flag) {
				t.Fatalf("lookupFlag() flag = %v, want %q", flag, tc.flag)
			}
		})
	}
}

// formatValue is a custom pflag.Value whose Type() is not a pflag built-in, so
// enum membership compares it as an exact string.
type formatValue struct{ value string }

func (v *formatValue) String() string     { return v.value }
func (v *formatValue) Set(s string) error { v.value = s; return nil }
func (*formatValue) Type() string         { return "format" }

// customSliceValue is a custom pflag.Value that also implements
// pflag.SliceValue; wrapping it would hide the slice behaviour.
type customSliceValue struct{ values []string }

func (v *customSliceValue) String() string                { return strings.Join(v.values, ",") }
func (v *customSliceValue) Set(s string) error            { v.values = append(v.values, s); return nil }
func (*customSliceValue) Type() string                    { return "csvList" }
func (v *customSliceValue) Append(s string) error         { v.values = append(v.values, s); return nil }
func (v *customSliceValue) Replace(values []string) error { v.values = values; return nil }
func (v *customSliceValue) GetSlice() []string            { return v.values }

func enumFixture() (*cobra.Command, *cobra.Command) {
	root := &cobra.Command{Use: "app"}
	root.PersistentFlags().String("region", "us", "region")
	child := &cobra.Command{Use: "deploy"}
	child.Flags().String("output", "json", "output")
	child.Flags().String("mode", "", "mode")
	child.Flags().Int("n", 3, "n")
	child.Flags().Uint8("level", 1, "level")
	child.Flags().Int8("small", 0, "small")
	child.Flags().Var(&formatValue{value: "json"}, "format", "format")
	child.Flags().Var(&customSliceValue{}, "csv", "csv")
	child.Flags().Bool("force", false, "force")
	child.Flags().Count("verbose", "v")
	child.Flags().Float64("ratio", 0.5, "ratio")
	child.Flags().Duration("timeout", 0, "timeout")
	child.Flags().StringSlice("tags", nil, "tags")
	root.AddCommand(child)
	return root, child
}

func TestAddFlagEnum(t *testing.T) {
	cases := []struct {
		name    string
		target  string
		flag    string
		values  []string
		example string
		want    *Violation
	}{
		{name: "string", flag: "output", values: []string{"json", "table"}},
		{name: "int", flag: "n", values: []string{"1", "3", "5"}},
		{name: "uint8", flag: "level", values: []string{"1", "2"}},
		{name: "custom type", flag: "format", values: []string{"json", "yaml"}},
		{name: "persistent on owner", target: "root", flag: "region", values: []string{"us", "eu"}},
		{name: "empty default exempt", flag: "mode", values: []string{"fast", "slow"}},
		{
			name:   "nil command",
			target: "nil",
			flag:   "output",
			values: []string{"json"},
			want:   &Violation{Field: "cmd", Reason: ReasonNilCommand},
		},
		{
			name:   "missing flag",
			flag:   "nope",
			values: []string{"x"},
			want:   &Violation{Field: "flag", Reason: ReasonFlagNotFound},
		},
		{
			name:   "bool",
			flag:   "force",
			values: []string{"true"},
			want:   &Violation{Field: "flag", Reason: ReasonUnsupportedType},
		},
		{
			name:   "count",
			flag:   "verbose",
			values: []string{"1"},
			want:   &Violation{Field: "flag", Reason: ReasonUnsupportedType},
		},
		{
			name:   "float64",
			flag:   "ratio",
			values: []string{"0.5"},
			want:   &Violation{Field: "flag", Reason: ReasonUnsupportedType},
		},
		{
			name:   "duration",
			flag:   "timeout",
			values: []string{"0s"},
			want:   &Violation{Field: "flag", Reason: ReasonUnsupportedType},
		},
		{
			name:   "stringSlice",
			flag:   "tags",
			values: []string{"a"},
			want:   &Violation{Field: "flag", Reason: ReasonUnsupportedType},
		},
		{
			name:   "custom slice",
			flag:   "csv",
			values: []string{"a"},
			want:   &Violation{Field: "flag", Reason: ReasonUnsupportedType},
		},
		{name: "empty values", flag: "output", values: nil, want: &Violation{Field: "values", Reason: ReasonRequired}},
		{
			name:   "non-int on int",
			flag:   "n",
			values: []string{"3", "x"},
			want:   &Violation{Field: "values", Reason: ReasonInvalidValue},
		},
		{
			name:   "int8 overflow",
			flag:   "small",
			values: []string{"0", "300"},
			want:   &Violation{Field: "values", Reason: ReasonInvalidValue},
		},
		{
			name:   "canonical duplicates",
			flag:   "n",
			values: []string{"3", "03"},
			want:   &Violation{Field: "values", Reason: ReasonDuplicate},
		},
		{
			name:   "default outside set",
			flag:   "output",
			values: []string{"table", "yaml"},
			want:   &Violation{Field: "default", Reason: ReasonNotInEnum},
		},
		{
			name:    "example outside set",
			flag:    "output",
			values:  []string{"json", "table"},
			example: "yaml",
			want:    &Violation{Field: "example", Reason: ReasonNotInEnum},
		},
		{name: "example inside set", flag: "n", values: []string{"3", "5"}, example: "05"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, child := enumFixture()
			cmd := child
			switch tc.target {
			case "root":
				cmd = root
			case "nil":
				cmd = nil
			}
			var before pflag.Value
			var flag *pflag.Flag
			if cmd != nil {
				flag, _ = lookupFlag(cmd, tc.flag)
			}
			if flag != nil {
				if tc.example != "" {
					flag.Annotations = map[string][]string{exampleAnnotationKey: {tc.example}}
				}
				before = flag.Value
			}

			violation := AddFlagEnum(cmd, tc.flag, tc.values)
			if !reflect.DeepEqual(violation, tc.want) {
				t.Fatalf("AddFlagEnum() violation = %+v, want %+v", violation, tc.want)
			}
			if tc.want != nil {
				if flag != nil && flag.Value != before {
					t.Fatalf("failed AddFlagEnum mutated flag.Value")
				}
				return
			}
			if got := FlagEnum(flag); !reflect.DeepEqual(got, tc.values) {
				t.Fatalf("FlagEnum() = %v, want %v", got, tc.values)
			}
			if flag.Value.Type() != before.Type() {
				t.Fatalf("Type() = %q, want %q", flag.Value.Type(), before.Type())
			}
		})
	}
}

func TestAddFlagEnumRedeclarationReplaces(t *testing.T) {
	_, child := enumFixture()
	if v := AddFlagEnum(child, "output", []string{"json", "table"}); v != nil {
		t.Fatalf("first AddFlagEnum: %+v", v)
	}
	if v := AddFlagEnum(child, "output", []string{"yaml", "json"}); v != nil {
		t.Fatalf("second AddFlagEnum: %+v", v)
	}
	flag := child.Flags().Lookup("output")
	wrapper, ok := flag.Value.(*enumValue)
	if !ok {
		t.Fatalf("flag.Value = %T, want *enumValue", flag.Value)
	}
	if _, nested := wrapper.inner.(*enumValue); nested {
		t.Fatal("re-declaration wrapped the value twice")
	}
	if got := FlagEnum(flag); !reflect.DeepEqual(got, []string{"yaml", "json"}) {
		t.Fatalf("FlagEnum() = %v, want [yaml json]", got)
	}
	if v := AddFlagEnum(child, "output", []string{"table"}); v == nil || v.Reason != ReasonNotInEnum {
		t.Fatalf("invalid re-declaration violation = %+v, want default not_in_enum", v)
	}
	if got := FlagEnum(flag); !reflect.DeepEqual(got, []string{"yaml", "json"}) {
		t.Fatalf("failed re-declaration changed FlagEnum() to %v", got)
	}
}

func TestAddFlagExample(t *testing.T) {
	newCmd := func(t *testing.T) *cobra.Command {
		t.Helper()
		cmd := &cobra.Command{Use: "app"}
		cmd.Flags().String("service", "", "service")
		cmd.Flags().Int("n", 1, "n")
		cmd.Flags().Duration("timeout", 0, "timeout")
		cmd.Flags().StringSlice("tags", nil, "tags")
		cmd.Flags().IntSlice("ports", nil, "ports")
		cmd.Flags().Float64("ratio", 0, "ratio")
		cmd.Flags().Float64Slice("weights", nil, "weights")
		cmd.Flags().Var(&formatValue{}, "format", "format")
		cmd.Flags().String("output", "json", "output")
		if err := AddFlagEnum(cmd, "output", []string{"json", "table"}); err != nil {
			t.Fatalf("AddFlagEnum: %v", err)
		}
		return cmd
	}
	cases := []struct {
		name    string
		nilCmd  bool
		flag    string
		example string
		want    *Violation
	}{
		{name: "float", flag: "ratio", example: "1.5"},
		{name: "float64Slice", flag: "weights", example: "0.5,2"},
		{
			name:    "float NaN not representable in MCP",
			flag:    "ratio",
			example: "NaN",
			want:    &Violation{Field: "example", Reason: ReasonInvalidValue},
		},
		{
			name:    "float +Inf not representable in MCP",
			flag:    "ratio",
			example: "+Inf",
			want:    &Violation{Field: "example", Reason: ReasonInvalidValue},
		},
		{
			name:    "float64Slice -Inf element not representable in MCP",
			flag:    "weights",
			example: "1,-Inf",
			want:    &Violation{Field: "example", Reason: ReasonInvalidValue},
		},
		{name: "string", flag: "service", example: "svc-a"},
		{name: "int", flag: "n", example: "5"},
		{name: "duration", flag: "timeout", example: "45s"},
		{name: "stringSlice", flag: "tags", example: "a,b"},
		{name: "intSlice", flag: "ports", example: "1,2"},
		{name: "custom unchecked", flag: "format", example: "anything at all"},
		{name: "enum member", flag: "output", example: "table"},
		{
			name:    "nil command",
			nilCmd:  true,
			flag:    "service",
			example: "x",
			want:    &Violation{Field: "cmd", Reason: ReasonNilCommand},
		},
		{name: "missing flag", flag: "nope", example: "x", want: &Violation{Field: "flag", Reason: ReasonFlagNotFound}},
		{name: "empty", flag: "service", example: "", want: &Violation{Field: "example", Reason: ReasonRequired}},
		{name: "non-int", flag: "n", example: "three", want: &Violation{Field: "example", Reason: ReasonInvalidValue}},
		{
			name:    "bad duration",
			flag:    "timeout",
			example: "abc",
			want:    &Violation{Field: "example", Reason: ReasonInvalidValue},
		},
		{
			name:    "bad intSlice element",
			flag:    "ports",
			example: "1,x",
			want:    &Violation{Field: "example", Reason: ReasonInvalidValue},
		},
		{
			name:    "outside enum",
			flag:    "output",
			example: "yaml",
			want:    &Violation{Field: "example", Reason: ReasonNotInEnum},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newCmd(t)
			target := cmd
			if tc.nilCmd {
				target = nil
			}
			flag := cmd.Flags().Lookup(tc.flag)
			if flag != nil {
				flag.Annotations = map[string][]string{exampleAnnotationKey: {"previous"}}
			}

			violation := AddFlagExample(target, tc.flag, tc.example)
			if !reflect.DeepEqual(violation, tc.want) {
				t.Fatalf("AddFlagExample() violation = %+v, want %+v", violation, tc.want)
			}
			if tc.want != nil {
				if flag != nil && !reflect.DeepEqual(flag.Annotations[exampleAnnotationKey], []string{"previous"}) {
					t.Fatalf(
						"failed AddFlagExample changed annotation to %v",
						flag.Annotations[exampleAnnotationKey],
					)
				}
				return
			}
			if got := FlagExample(flag); got != tc.example {
				t.Fatalf("FlagExample() = %q, want %q", got, tc.example)
			}
		})
	}

	t.Run("hand-edited non-finite annotation reads as no example", func(t *testing.T) {
		flag := newCmd(t).Flags().Lookup("ratio")
		flag.Annotations = map[string][]string{exampleAnnotationKey: {"NaN"}}
		if got := FlagExample(flag); got != "" {
			t.Fatalf("FlagExample() = %q, want \"\"", got)
		}
	})

	t.Run("re-declaration replaces", func(t *testing.T) {
		cmd := newCmd(t)
		for _, example := range []string{"svc-a", "svc-b"} {
			if err := AddFlagExample(cmd, "service", example); err != nil {
				t.Fatalf("AddFlagExample(%q): %v", example, err)
			}
		}
		if got := FlagExample(cmd.Flags().Lookup("service")); got != "svc-b" {
			t.Fatalf("FlagExample() = %q, want svc-b", got)
		}
	})
}

func TestAddCapability(t *testing.T) {
	for _, class := range []string{"read-only", "create", "mutate", "delete", "external-network", "admin"} {
		t.Run(class, func(t *testing.T) {
			cmd := &cobra.Command{Use: "app"}
			if err := AddCapability(cmd, class, ""); err != nil {
				t.Fatalf("AddCapability(%q): %v", class, err)
			}
			got, note, ok := CommandCapability(cmd.Annotations)
			if !ok || got != class || note != "" {
				t.Fatalf("CommandCapability = (%q, %q, %v), want (%q, \"\", true)", got, note, ok, class)
			}
		})
	}

	for _, bad := range []string{"", "Read-Only", " mutate", "write"} {
		t.Run("invalid "+bad, func(t *testing.T) {
			cmd := &cobra.Command{Use: "app"}
			if err := AddCapability(cmd, "mutate", "kept"); err != nil {
				t.Fatalf("seed: %v", err)
			}
			before := maps.Clone(cmd.Annotations)
			want := &Violation{Field: "class", Reason: ReasonNotVocabulary}
			if v := AddCapability(cmd, bad, "ignored"); !reflect.DeepEqual(v, want) {
				t.Fatalf("AddCapability(%q) violation = %+v, want %+v", bad, v, want)
			}
			if !reflect.DeepEqual(cmd.Annotations, before) {
				t.Fatalf("failed AddCapability changed annotations to %v", cmd.Annotations)
			}
		})
	}

	t.Run("nil command", func(t *testing.T) {
		want := &Violation{Field: "cmd", Reason: ReasonNilCommand}
		if v := AddCapability(nil, "mutate", ""); !reflect.DeepEqual(v, want) {
			t.Fatalf("AddCapability(nil) violation = %+v, want %+v", v, want)
		}
	})

	t.Run("note trimmed and re-declaration replaces", func(t *testing.T) {
		cmd := &cobra.Command{Use: "app"}
		if err := AddCapability(cmd, "mutate", "  idempotent by name \n"); err != nil {
			t.Fatal(err)
		}
		if class, note, _ := CommandCapability(cmd.Annotations); class != "mutate" || note != "idempotent by name" {
			t.Fatalf("got (%q, %q), want (mutate, idempotent by name)", class, note)
		}
		if err := AddCapability(cmd, "delete", "   "); err != nil {
			t.Fatal(err)
		}
		if _, present := cmd.Annotations[capabilityNoteAnnotationKey]; present {
			t.Fatal("a blank note left the note annotation in place")
		}
		if class, note, _ := CommandCapability(cmd.Annotations); class != "delete" || note != "" {
			t.Fatalf("got (%q, %q), want (delete, \"\")", class, note)
		}
	})
}

func TestCommandCapability(t *testing.T) {
	cases := []struct {
		name        string
		annotations map[string]string
		wantClass   string
		wantNote    string
		wantOK      bool
	}{
		{name: "nil annotations"},
		{name: "unclassified", annotations: map[string]string{"other": "x"}},
		{
			name:        "class and note",
			annotations: map[string]string{capabilityAnnotationKey: "admin", capabilityNoteAnnotationKey: "root only"},
			wantClass:   "admin", wantNote: "root only", wantOK: true,
		},
		{
			name:        "class outside vocabulary fails closed",
			annotations: map[string]string{capabilityAnnotationKey: "write", capabilityNoteAnnotationKey: "n"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			class, note, ok := CommandCapability(tc.annotations)
			if class != tc.wantClass || note != tc.wantNote || ok != tc.wantOK {
				t.Fatalf("CommandCapability = (%q, %q, %v), want (%q, %q, %v)",
					class, note, ok, tc.wantClass, tc.wantNote, tc.wantOK)
			}
		})
	}
}
