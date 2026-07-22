package schema_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/internal/schema"
)

type taggedLeaf struct {
	ID string `json:"id" ax:"nondeterministic"`
}

type EmbeddedTagged struct {
	EmbeddedID string `json:"embedded_id" ax:"nondeterministic"`
}

type recursivePayload struct {
	ID   string            `json:"id"   ax:"nondeterministic"`
	Next *recursivePayload `json:"next"`
}

func newCmd(use string) *cobra.Command {
	return &cobra.Command{
		Use:     use,
		Short:   use + " short",
		Long:    use + " long",
		Example: use + " --flag value",
	}
}

func duplicateLocatorType() reflect.Type {
	return reflect.StructOf([]reflect.StructField{
		{Name: "Zed", Type: reflect.TypeFor[string](), Tag: `json:"zed" ax:"nondeterministic"`},
		{Name: "FirstID", Type: reflect.TypeFor[string](), Tag: `json:"id" ax:"nondeterministic"`},
		{Name: "SecondID", Type: reflect.TypeFor[string](), Tag: `json:"id" ax:"nondeterministic"`},
		{Name: "Generated", Type: reflect.TypeFor[string](), Tag: `json:"generated" ax:"nondeterministic"`},
	})
}

func TestDataLocators(t *testing.T) {
	tests := []struct {
		name string
		typ  reflect.Type
		want []string
	}{
		{
			name: "direct field",
			typ: reflect.TypeFor[struct {
				GeneratedAt string `json:"generated_at" ax:"nondeterministic"`
			}](),
			want: []string{"data.generated_at"},
		},
		{
			name: "default JSON field name",
			typ: reflect.TypeFor[struct {
				GeneratedAt string `ax:"nondeterministic"`
			}](),
			want: []string{"data.GeneratedAt"},
		},
		{
			name: "ignored JSON field",
			typ: reflect.TypeFor[struct {
				Secret string `json:"-" ax:"nondeterministic"`
			}](),
			want: []string{},
		},
		{
			name: "literal dash JSON field name is not ignored",
			typ: reflect.TypeFor[struct {
				Dash string `json:"-," ax:"nondeterministic"`
			}](),
			want: []string{"data.-"},
		},
		{
			name: "nested struct",
			typ: reflect.TypeFor[struct {
				Item taggedLeaf `json:"item"`
			}](),
			want: []string{"data.item.id"},
		},
		{
			name: "pointer root and pointer field",
			typ: reflect.TypeFor[*struct {
				Item *taggedLeaf `json:"item"`
			}](),
			want: []string{"data.item.id"},
		},
		{
			name: "slice element",
			typ: reflect.TypeFor[struct {
				Items []taggedLeaf `json:"items"`
			}](),
			want: []string{"data.items.id"},
		},
		{
			name: "array element",
			typ: reflect.TypeFor[struct {
				Items [2]taggedLeaf `json:"items"`
			}](),
			want: []string{"data.items.id"},
		},
		{
			name: "map value",
			typ: reflect.TypeFor[struct {
				Items map[string]taggedLeaf `json:"items"`
			}](),
			want: []string{"data.items.id"},
		},
		{
			name: "embedded struct is inlined",
			typ: reflect.TypeFor[struct {
				EmbeddedTagged
			}](),
			want: []string{"data.embedded_id"},
		},
		{
			name: "unexported field is skipped",
			typ: reflect.TypeFor[struct {
				visible string `ax:"nondeterministic"`
			}](),
			want: []string{},
		},
		{
			name: "zero tagged fields",
			typ: reflect.TypeFor[struct {
				Name string `json:"name"`
			}](),
			want: []string{},
		},
		{
			name: "non struct",
			typ:  reflect.TypeFor[string](),
			want: []string{},
		},
		{
			name: "nil type",
			typ:  nil,
			want: []string{},
		},
		{
			name: "self reference is bounded",
			typ:  reflect.TypeFor[recursivePayload](),
			want: []string{"data.id"},
		},
		{
			name: "sorted and deduplicated",
			typ:  duplicateLocatorType(),
			want: []string{"data.generated", "data.id", "data.zed"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := schema.DataLocators(tc.typ)
			if got == nil {
				t.Fatal("DataLocators() returned nil, want explicit empty slice")
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("DataLocators() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRegisterEnvelopeAndNonDeterministicFields(t *testing.T) {
	t.Run("registered union is sorted and deduplicated", func(t *testing.T) {
		cmd := newCmd("root")
		cmd.Annotations = map[string]string{"consumer.example/preserved": "true"}

		schema.RegisterEnvelope(cmd, []string{"data.zed", "data.alpha", "data.zed"})

		want := []string{
			"data.alpha",
			"data.zed",
			"meta.idempotency_key",
			"meta.span_id",
			"meta.trace_id",
		}
		got := schema.NonDeterministicFields(cmd.Annotations)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("NonDeterministicFields() = %v, want %v", got, want)
		}
		if cmd.Annotations["consumer.example/preserved"] != "true" {
			t.Error("RegisterEnvelope removed an unrelated command annotation")
		}
	})

	t.Run("unregistered annotations return explicit empty slice", func(t *testing.T) {
		tests := []struct {
			name        string
			annotations map[string]string
		}{
			{name: "nil", annotations: nil},
			{name: "unrelated", annotations: map[string]string{"consumer.example/key": "value"}},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				got := schema.NonDeterministicFields(tc.annotations)
				if got == nil {
					t.Fatal("NonDeterministicFields() returned nil, want explicit empty slice")
				}
				if len(got) != 0 {
					t.Errorf("NonDeterministicFields() = %v, want empty", got)
				}
			})
		}
	})

	t.Run("second registration overwrites", func(t *testing.T) {
		cmd := newCmd("root")
		schema.RegisterEnvelope(cmd, []string{"data.first"})
		schema.RegisterEnvelope(cmd, []string{"data.second"})

		got := schema.NonDeterministicFields(cmd.Annotations)
		if slices.Contains(got, "data.first") {
			t.Errorf("NonDeterministicFields() = %v, retained overwritten locator", got)
		}
		if !slices.Contains(got, "data.second") {
			t.Errorf("NonDeterministicFields() = %v, missing last registration", got)
		}
	})

	t.Run("nil command is no op", func(_ *testing.T) {
		schema.RegisterEnvelope(nil, []string{"data.field"})
	})
}

func TestBuildCommand_AnnotationsCopied(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
	}{
		{name: "set", annotations: map[string]string{"consumer.example/key": "value"}},
		{name: "nil", annotations: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := newCmd("root")
			root.Annotations = tc.annotations

			got := schema.BuildCommand(root)

			if !reflect.DeepEqual(got.Annotations, tc.annotations) {
				t.Errorf("Annotations = %#v, want %#v", got.Annotations, tc.annotations)
			}
			if tc.annotations != nil {
				got.Annotations["mutated"] = "true"
				if _, ok := root.Annotations["mutated"]; ok {
					t.Error("mutating Command.Annotations corrupted the source *cobra.Command's Annotations map")
				}
			}
		})
	}
}
