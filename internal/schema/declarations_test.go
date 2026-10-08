package schema

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func declarationTree() (*cobra.Command, map[string]*cobra.Command) {
	root := &cobra.Command{Use: "app", RunE: noopRunE}
	group := &cobra.Command{Use: "group"}
	leaf := &cobra.Command{Use: "leaf", RunE: noopRunE}
	hidden := &cobra.Command{Use: "secret", Hidden: true, RunE: noopRunE}
	hiddenChild := &cobra.Command{Use: "inner", RunE: noopRunE}
	reserved := &cobra.Command{Use: "__schema", RunE: noopRunE}
	sibling := &cobra.Command{Use: "zeta", RunE: noopRunE}

	group.AddCommand(leaf)
	hidden.AddCommand(hiddenChild)
	root.AddCommand(group, hidden, reserved, sibling)

	return root, map[string]*cobra.Command{
		"root": root, "group": group, "leaf": leaf, "hidden": hidden,
		"hiddenChild": hiddenChild, "reserved": reserved, "sibling": sibling,
	}
}

func TestWalkDeclarationCommandsPrunesOnlyHidden(t *testing.T) {
	root, _ := declarationTree()

	var got []string
	WalkDeclarationCommands(root, func(cmd *cobra.Command) {
		got = append(got, cmd.CommandPath())
	})

	want := []string{"app", "app __schema", "app group", "app group leaf", "app zeta"}
	if !slices.Equal(got, want) {
		t.Fatalf("walk = %v, want %v", got, want)
	}
}

func TestWalkDeclarationCommandsHiddenRootVisitsNothing(t *testing.T) {
	root := &cobra.Command{Use: "app", Hidden: true}
	root.AddCommand(&cobra.Command{Use: "child", RunE: noopRunE})

	WalkDeclarationCommands(root, func(cmd *cobra.Command) {
		t.Fatalf("visited %q under a hidden root", cmd.CommandPath())
	})
}

func TestValidName(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "simple", input: "triage", want: true},
		{name: "full charset", input: "a-Z_0.9", want: true},
		{name: "empty", input: "", want: false},
		{name: "space", input: "two words", want: false},
		{name: "slash", input: "a/b", want: false},
		{name: "non-ascii", input: "café", want: false},
		{name: "brace", input: "{x}", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validName(tc.input); got != tc.want {
				t.Fatalf("validName(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestTemplatePlaceholders(t *testing.T) {
	cases := []struct {
		name     string
		template string
		want     []string
	}{
		{name: "none", template: "run app report", want: nil},
		{name: "single", template: "since {{window}}", want: []string{"window"}},
		{name: "multiple in order", template: "{{a}} then {{b}} then {{a}}", want: []string{"a", "b", "a"}},
		{name: "at start and end", template: "{{a}}x{{b}}", want: []string{"a", "b"}},
		{name: "inner spaces are literal", template: "{{ a }}", want: nil},
		{name: "unterminated is literal", template: "{{a and more", want: nil},
		{name: "empty braces are literal", template: "{{}}", want: nil},
		{name: "invalid charset is literal", template: "{{a!}} {{b c}}", want: nil},
		{name: "single braces are literal", template: "{a} }} {{", want: nil},
		{name: "literal then placeholder", template: "{{ x }} {{y}}", want: []string{"y"}},
		{name: "triple open brace is literal", template: "{{{x}}}", want: nil},
		{name: "extra closing brace is literal", template: "{{a}}}", want: []string{"a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := templatePlaceholders(tc.template); !slices.Equal(got, tc.want) {
				t.Fatalf("templatePlaceholders(%q) = %q, want %q", tc.template, got, tc.want)
			}
		})
	}
}

func validPrompt() Prompt {
	return Prompt{
		Name:        "triage",
		Title:       "Triage a spike",
		Description: "Find the top cost driver.",
		Arguments: []PromptArgument{
			{Name: "window", Title: "Window", Description: "lookback", Required: true},
			{Name: "unused", Description: "steers the workflow only"},
		},
		Template: "Run app report --since={{window}}.",
	}
}

func TestValidatePrompt(t *testing.T) {
	invalidUTF8 := string([]byte{0xff, 0xfe})
	cases := []struct {
		name   string
		mutate func(*Prompt)
		want   *Violation
	}{
		{name: "valid", mutate: func(*Prompt) {}},
		{name: "valid without arguments", mutate: func(p *Prompt) {
			p.Arguments = nil
			p.Template = "static text"
		}},
		{name: "empty name", mutate: func(p *Prompt) { p.Name = "" },
			want: &Violation{Field: "name", Reason: ReasonRequired}},
		{name: "bad name charset", mutate: func(p *Prompt) { p.Name = "two words" },
			want: &Violation{Field: "name", Reason: ReasonInvalidCharset}},
		{name: "invalid utf8 title", mutate: func(p *Prompt) { p.Title = invalidUTF8 },
			want: &Violation{Field: "title", Reason: ReasonInvalidUTF8}},
		{name: "invalid utf8 description", mutate: func(p *Prompt) { p.Description = invalidUTF8 },
			want: &Violation{Field: "description", Reason: ReasonInvalidUTF8}},
		{name: "empty argument name", mutate: func(p *Prompt) { p.Arguments[1].Name = "" },
			want: &Violation{Field: "arguments[1].name", Reason: ReasonRequired}},
		{name: "bad argument name", mutate: func(p *Prompt) { p.Arguments[0].Name = "a b" },
			want: &Violation{Field: "arguments[0].name", Reason: ReasonInvalidCharset}},
		{name: "invalid utf8 argument title", mutate: func(p *Prompt) { p.Arguments[0].Title = invalidUTF8 },
			want: &Violation{Field: "arguments[0].title", Reason: ReasonInvalidUTF8}},
		{
			name:   "invalid utf8 argument description",
			mutate: func(p *Prompt) { p.Arguments[1].Description = invalidUTF8 },
			want:   &Violation{Field: "arguments[1].description", Reason: ReasonInvalidUTF8},
		},
		{name: "duplicate argument name", mutate: func(p *Prompt) { p.Arguments[1].Name = "window" },
			want: &Violation{Field: "arguments[1].name", Reason: ReasonDuplicate}},
		{name: "empty template", mutate: func(p *Prompt) { p.Template = "" },
			want: &Violation{Field: "template", Reason: ReasonRequired}},
		{name: "invalid utf8 template", mutate: func(p *Prompt) { p.Template = invalidUTF8 },
			want: &Violation{Field: "template", Reason: ReasonInvalidUTF8}},
		{name: "undeclared placeholder", mutate: func(p *Prompt) { p.Template = "{{window}} {{missing}}" },
			want: &Violation{Field: "template", Reason: ReasonUndeclaredPlaceholder}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prompt := validPrompt()
			tc.mutate(&prompt)
			if got := ValidatePrompt(prompt); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ValidatePrompt = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestAddPromptAppendsInDeclarationOrder(t *testing.T) {
	cmd := &cobra.Command{Use: "app", Annotations: map[string]string{"keep": "me"}}
	first := validPrompt()
	second := validPrompt()
	second.Name = "second"

	for _, prompt := range []Prompt{first, second} {
		if v := AddPrompt(cmd, prompt); v != nil {
			t.Fatalf("AddPrompt(%q) = %+v, want nil", prompt.Name, v)
		}
	}

	got := Prompts(cmd.Annotations)
	if !reflect.DeepEqual(got, []Prompt{first, second}) {
		t.Fatalf("Prompts = %+v, want both prompts in declaration order", got)
	}
	if cmd.Annotations["keep"] != "me" {
		t.Fatal("AddPrompt dropped an unrelated annotation")
	}
}

func TestAddPromptRejectionLeavesCommandUnchanged(t *testing.T) {
	invalid := validPrompt()
	invalid.Name = ""

	t.Run("nil command", func(t *testing.T) {
		want := &Violation{Field: "cmd", Reason: ReasonNilCommand}
		if got := AddPrompt(nil, validPrompt()); !reflect.DeepEqual(got, want) {
			t.Fatalf("AddPrompt(nil) = %+v, want %+v", got, want)
		}
	})
	t.Run("invalid prompt keeps nil annotations nil", func(t *testing.T) {
		cmd := &cobra.Command{Use: "app"}
		if v := AddPrompt(cmd, invalid); v == nil {
			t.Fatal("AddPrompt accepted an invalid prompt")
		}
		if cmd.Annotations != nil {
			t.Fatalf("Annotations = %v, want nil", cmd.Annotations)
		}
	})
	t.Run("same-command duplicate", func(t *testing.T) {
		cmd := &cobra.Command{Use: "app"}
		if v := AddPrompt(cmd, validPrompt()); v != nil {
			t.Fatalf("first AddPrompt = %+v", v)
		}
		before := maps.Clone(cmd.Annotations)
		want := &Violation{Field: "name", Reason: ReasonDuplicate}
		if got := AddPrompt(cmd, validPrompt()); !reflect.DeepEqual(got, want) {
			t.Fatalf("duplicate AddPrompt = %+v, want %+v", got, want)
		}
		if !maps.Equal(cmd.Annotations, before) {
			t.Fatal("rejected duplicate modified the annotations")
		}
	})
}

func TestPromptsFailsClosed(t *testing.T) {
	cases := []struct {
		name  string
		value *string
		want  []string
	}{
		{name: "absent", value: nil, want: nil},
		{name: "undecodable", value: new("{not json"), want: nil},
		{name: "wrong shape", value: new(`{"name":"x"}`), want: nil},
		{
			name: "drops invalid entry",
			value: new(
				`[{"name":"ok","template":"t"},{"name":"bad name","template":"t"},{"name":"also","template":"{{missing}}"}]`,
			),
			want: []string{"ok"},
		},
		{name: "all invalid", value: new(`[{"name":"","template":"t"}]`), want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			annotations := map[string]string{}
			if tc.value != nil {
				annotations[promptsAnnotationKey] = *tc.value
			}
			var names []string
			for _, prompt := range Prompts(annotations) {
				names = append(names, prompt.Name)
			}
			if !slices.Equal(names, tc.want) {
				t.Fatalf("Prompts names = %v, want %v", names, tc.want)
			}
		})
	}
}

func TestFindDuplicate(t *testing.T) {
	prompt := func(name string) Prompt { return Prompt{Name: name, Template: "t"} }

	t.Run("none", func(t *testing.T) {
		root, cmds := declarationTree()
		mustAddPrompt(t, root, prompt("a"))
		mustAddPrompt(t, cmds["leaf"], prompt("b"))
		if got := FindDuplicate(root); got != nil {
			t.Fatalf("FindDuplicate = %+v, want nil", got)
		}
	})
	t.Run("prompt across commands", func(t *testing.T) {
		root, cmds := declarationTree()
		mustAddPrompt(t, cmds["leaf"], prompt("a"))
		mustAddPrompt(t, cmds["sibling"], prompt("a"))
		want := &Conflict{Kind: KindPrompt, Key: "a", Commands: []string{"app group leaf", "app zeta"}}
		if got := FindDuplicate(root); !reflect.DeepEqual(got, want) {
			t.Fatalf("FindDuplicate = %+v, want %+v", got, want)
		}
	})
	t.Run("duplicate on reserved command counts", func(t *testing.T) {
		root, cmds := declarationTree()
		mustAddPrompt(t, cmds["reserved"], prompt("a"))
		mustAddPrompt(t, cmds["sibling"], prompt("a"))
		if got := FindDuplicate(root); got == nil {
			t.Fatal("FindDuplicate ignored a declaration on a reserved command")
		}
	})
	t.Run("hidden subtree ignored", func(t *testing.T) {
		root, cmds := declarationTree()
		mustAddPrompt(t, root, prompt("a"))
		mustAddPrompt(t, cmds["hiddenChild"], prompt("a"))
		if got := FindDuplicate(root); got != nil {
			t.Fatalf("FindDuplicate = %+v, want nil for a hidden duplicate", got)
		}
	})
	t.Run("hand-written same-command duplicate", func(t *testing.T) {
		root := &cobra.Command{Use: "app", Annotations: map[string]string{
			promptsAnnotationKey: `[{"name":"a","template":"t"},{"name":"a","template":"u"}]`,
		}}
		want := &Conflict{Kind: KindPrompt, Key: "a", Commands: []string{"app", "app"}}
		if got := FindDuplicate(root); !reflect.DeepEqual(got, want) {
			t.Fatalf("FindDuplicate = %+v, want %+v", got, want)
		}
	})
}

func TestCollectDeclarationsKeepsFirstInWalkOrder(t *testing.T) {
	root, cmds := declarationTree()
	mustAddPrompt(t, cmds["leaf"], Prompt{Name: "a", Template: "leaf"})
	mustAddPrompt(t, cmds["leaf"], Prompt{Name: "b", Template: "leaf"})
	mustAddPrompt(t, cmds["group"], Prompt{Name: "c", Template: "group"})
	mustAddPrompt(t, cmds["sibling"], Prompt{Name: "a", Template: "sibling"})
	mustAddPrompt(t, cmds["hiddenChild"], Prompt{Name: "h", Template: "hidden"})

	prompts, _ := CollectDeclarations(root)

	var got []string
	for _, prompt := range prompts {
		got = append(got, prompt.Name+":"+prompt.Template)
	}
	want := []string{"c:group", "a:leaf", "b:leaf"}
	if !slices.Equal(got, want) {
		t.Fatalf("collected prompts = %v, want %v", got, want)
	}
}

func TestEncodedPromptsAreCopies(t *testing.T) {
	cmd := &cobra.Command{Use: "app"}
	prompt := validPrompt()
	mustAddPrompt(t, cmd, prompt)

	prompt.Arguments[0].Name = "mutated"
	prompt.Template = "mutated"

	got := Prompts(cmd.Annotations)
	if len(got) != 1 || got[0].Arguments[0].Name != "window" || strings.Contains(got[0].Template, "mutated") {
		t.Fatalf("stored prompt changed after caller mutation: %+v", got)
	}
}

func mustAddPrompt(t *testing.T, cmd *cobra.Command, prompt Prompt) {
	t.Helper()
	if v := AddPrompt(cmd, prompt); v != nil {
		t.Fatalf("AddPrompt(%q) on %q = %+v", prompt.Name, cmd.Name(), v)
	}
}

func TestValidateResource(t *testing.T) {
	invalidUTF8 := string([]byte{0xff})
	valid := Resource{
		URI:         "app://docs/pricing",
		Name:        "pricing",
		Title:       "Pricing",
		Description: "d",
		MIMEType:    "text/markdown",
	}
	cases := []struct {
		name   string
		mutate func(*Resource)
		want   *Violation
	}{
		{name: "valid", mutate: func(*Resource) {}},
		{name: "valid https with query", mutate: func(r *Resource) { r.URI = "https://example.com/a?b=c#d" }},
		{name: "valid non-ascii path", mutate: func(r *Resource) { r.URI = "app://docs/café" }},
		{
			name:   "valid at length cap",
			mutate: func(r *Resource) { r.URI = "app://" + strings.Repeat("a", maxResourceURIBytes-6) },
		},
		{name: "empty uri", mutate: func(r *Resource) { r.URI = "" },
			want: &Violation{Field: "uri", Reason: ReasonRequired}},
		{
			name:   "uri over cap",
			mutate: func(r *Resource) { r.URI = "app://" + strings.Repeat("a", maxResourceURIBytes-5) },
			want:   &Violation{Field: "uri", Reason: ReasonTooLong},
		},
		{name: "uri invalid utf8", mutate: func(r *Resource) { r.URI = "app://" + invalidUTF8 },
			want: &Violation{Field: "uri", Reason: ReasonInvalidUTF8}},
		{name: "uri with space", mutate: func(r *Resource) { r.URI = "app://a b" },
			want: &Violation{Field: "uri", Reason: ReasonInvalidCharacter}},
		{name: "uri with newline", mutate: func(r *Resource) { r.URI = "app://a\nb" },
			want: &Violation{Field: "uri", Reason: ReasonInvalidCharacter}},
		{name: "relative uri", mutate: func(r *Resource) { r.URI = "docs/pricing" },
			want: &Violation{Field: "uri", Reason: ReasonNotAbsolute}},
		{name: "unparseable uri", mutate: func(r *Resource) { r.URI = "app://%zz" },
			want: &Violation{Field: "uri", Reason: ReasonNotAbsolute}},
		{name: "empty name", mutate: func(r *Resource) { r.Name = "" },
			want: &Violation{Field: "name", Reason: ReasonRequired}},
		{name: "invalid utf8 name", mutate: func(r *Resource) { r.Name = invalidUTF8 },
			want: &Violation{Field: "name", Reason: ReasonInvalidUTF8}},
		{name: "invalid utf8 title", mutate: func(r *Resource) { r.Title = invalidUTF8 },
			want: &Violation{Field: "title", Reason: ReasonInvalidUTF8}},
		{name: "invalid utf8 description", mutate: func(r *Resource) { r.Description = invalidUTF8 },
			want: &Violation{Field: "description", Reason: ReasonInvalidUTF8}},
		{name: "invalid utf8 mime type", mutate: func(r *Resource) { r.MIMEType = invalidUTF8 },
			want: &Violation{Field: "mime_type", Reason: ReasonInvalidUTF8}},
		{name: "control character in mime type", mutate: func(r *Resource) { r.MIMEType = "text/plain\r\n" },
			want: &Violation{Field: "mime_type", Reason: ReasonInvalidCharacter}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resource := valid
			tc.mutate(&resource)
			if got := ValidateResource(resource); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ValidateResource = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestAddResource(t *testing.T) {
	resource := Resource{URI: "app://docs", Name: "docs"}

	t.Run("nil command", func(t *testing.T) {
		want := &Violation{Field: "cmd", Reason: ReasonNilCommand}
		if got := AddResource(nil, resource); !reflect.DeepEqual(got, want) {
			t.Fatalf("AddResource(nil) = %+v, want %+v", got, want)
		}
	})
	t.Run("appends and preserves annotations", func(t *testing.T) {
		cmd := &cobra.Command{Use: "app", Annotations: map[string]string{"keep": "me"}}
		second := Resource{URI: "app://other", Name: "other"}
		for _, r := range []Resource{resource, second} {
			if v := AddResource(cmd, r); v != nil {
				t.Fatalf("AddResource(%q) = %+v", r.URI, v)
			}
		}
		if got := Resources(cmd.Annotations); !reflect.DeepEqual(got, []Resource{resource, second}) {
			t.Fatalf("Resources = %+v", got)
		}
		if cmd.Annotations["keep"] != "me" {
			t.Fatal("AddResource dropped an unrelated annotation")
		}
	})
	t.Run("invalid keeps nil annotations nil", func(t *testing.T) {
		cmd := &cobra.Command{Use: "app"}
		if v := AddResource(cmd, Resource{URI: "relative", Name: "x"}); v == nil {
			t.Fatal("AddResource accepted a relative URI")
		}
		if cmd.Annotations != nil {
			t.Fatalf("Annotations = %v, want nil", cmd.Annotations)
		}
	})
	t.Run("same-command duplicate URI", func(t *testing.T) {
		cmd := &cobra.Command{Use: "app"}
		if v := AddResource(cmd, resource); v != nil {
			t.Fatalf("first AddResource = %+v", v)
		}
		before := maps.Clone(cmd.Annotations)
		want := &Violation{Field: "uri", Reason: ReasonDuplicate}
		if got := AddResource(cmd, Resource{URI: resource.URI, Name: "renamed"}); !reflect.DeepEqual(got, want) {
			t.Fatalf("duplicate AddResource = %+v, want %+v", got, want)
		}
		if !maps.Equal(cmd.Annotations, before) {
			t.Fatal("rejected duplicate modified the annotations")
		}
	})
}

func TestResourcesFailsClosed(t *testing.T) {
	annotations := map[string]string{
		resourcesAnnotationKey: `[{"uri":"app://ok","name":"ok"},{"uri":"relative","name":"bad"}]`,
	}
	if got := Resources(annotations); len(got) != 1 || got[0].URI != "app://ok" {
		t.Fatalf("Resources = %+v, want only the valid entry", got)
	}
	annotations[resourcesAnnotationKey] = "not json"
	if got := Resources(annotations); got != nil {
		t.Fatalf("Resources(undecodable) = %+v, want nil", got)
	}
}

func TestFindDuplicateResources(t *testing.T) {
	t.Run("resource across commands", func(t *testing.T) {
		root, cmds := declarationTree()
		for _, cmd := range []*cobra.Command{cmds["group"], cmds["sibling"]} {
			if v := AddResource(cmd, Resource{URI: "app://docs", Name: cmd.Name()}); v != nil {
				t.Fatalf("AddResource: %+v", v)
			}
		}
		want := &Conflict{Kind: KindResource, Key: "app://docs", Commands: []string{"app group", "app zeta"}}
		if got := FindDuplicate(root); !reflect.DeepEqual(got, want) {
			t.Fatalf("FindDuplicate = %+v, want %+v", got, want)
		}
	})
	t.Run("prompt conflict reported before an earlier resource conflict", func(t *testing.T) {
		root, cmds := declarationTree()
		for _, cmd := range []*cobra.Command{root, cmds["group"]} {
			if v := AddResource(cmd, Resource{URI: "app://docs", Name: "r"}); v != nil {
				t.Fatalf("AddResource: %+v", v)
			}
		}
		mustAddPrompt(t, cmds["leaf"], Prompt{Name: "p", Template: "t"})
		mustAddPrompt(t, cmds["sibling"], Prompt{Name: "p", Template: "t"})
		if got := FindDuplicate(root); got == nil || got.Kind != KindPrompt {
			t.Fatalf("FindDuplicate = %+v, want the prompt conflict first", got)
		}
	})
	t.Run("same key in prompt and resource namespaces is allowed", func(t *testing.T) {
		root, cmds := declarationTree()
		mustAddPrompt(t, root, Prompt{Name: "docs", Template: "t"})
		if v := AddResource(cmds["leaf"], Resource{URI: "app://docs", Name: "docs"}); v != nil {
			t.Fatalf("AddResource: %+v", v)
		}
		if got := FindDuplicate(root); got != nil {
			t.Fatalf("FindDuplicate = %+v, want nil", got)
		}
	})
}
