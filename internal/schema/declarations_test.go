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

// TestWalkDeclarationCommandsMatchesBuildCommand pins the walk to
// BuildCommand's pruning on every fixture, including a hidden root: BuildCommand
// never prunes the root it is handed, so neither may the walk, or the ax-native
// and MCP projections disagree and duplicates escape FindDuplicate.
func TestWalkDeclarationCommandsMatchesBuildCommand(t *testing.T) {
	hiddenRoot := func() *cobra.Command {
		root := &cobra.Command{Use: "app", Hidden: true, RunE: noopRunE}
		child := &cobra.Command{Use: "child", RunE: noopRunE}
		child.AddCommand(&cobra.Command{Use: "gone", Hidden: true, RunE: noopRunE})
		root.AddCommand(child)
		return root
	}
	fixture := func() *cobra.Command { root, _ := declarationTree(); return root }

	for name, build := range map[string]func() *cobra.Command{"hidden root": hiddenRoot, "fixture": fixture} {
		t.Run(name, func(t *testing.T) {
			root := build()
			var walked []string
			WalkDeclarationCommands(root, func(cmd *cobra.Command) {
				walked = append(walked, cmd.Use)
			})
			if built := builtUses(BuildCommand(root)); !slices.Equal(walked, built) {
				t.Fatalf("walk = %v, BuildCommand = %v", walked, built)
			}
		})
	}
}

func builtUses(cmd Command) []string {
	uses := []string{cmd.Use}
	for _, child := range cmd.Commands {
		uses = append(uses, builtUses(child)...)
	}
	return uses
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
		{name: "odd brace run is literal", template: "{{{{{a}}", want: nil},
		{name: "even brace run reaches the name", template: "{{{{a}}", want: []string{"a"}},
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
		want := &Conflict{
			Kind:     KindPrompt,
			Key:      "a",
			Reason:   ReasonDuplicate,
			Commands: []string{"app group leaf", "app zeta"},
		}
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
		want := &Conflict{Kind: KindPrompt, Key: "a", Reason: ReasonDuplicate, Commands: []string{"app", "app"}}
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
		{name: "malformed uri with a scheme", mutate: func(r *Resource) { r.URI = "app://%zz" },
			want: &Violation{Field: "uri", Reason: ReasonMalformed}},
		{name: "valid urn", mutate: func(r *Resource) { r.URI = "urn:app:pricing" }},
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
		{name: "valid mime with parameters", mutate: func(r *Resource) { r.MIMEType = "text/plain; charset=utf-8" }},
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
			mustAddResource(t, cmd, r)
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
		mustAddResource(t, cmd, resource)
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
			mustAddResource(t, cmd, Resource{URI: "app://docs", Name: cmd.Name()})
		}
		want := &Conflict{
			Kind:     KindResource,
			Key:      "app://docs",
			Reason:   ReasonDuplicate,
			Commands: []string{"app group", "app zeta"},
		}
		if got := FindDuplicate(root); !reflect.DeepEqual(got, want) {
			t.Fatalf("FindDuplicate = %+v, want %+v", got, want)
		}
	})
	t.Run("prompt conflict reported before an earlier resource conflict", func(t *testing.T) {
		root, cmds := declarationTree()
		for _, cmd := range []*cobra.Command{root, cmds["group"]} {
			mustAddResource(t, cmd, Resource{URI: "app://docs", Name: "r"})
		}
		mustAddPrompt(t, cmds["leaf"], Prompt{Name: "p", Template: "t"})
		mustAddPrompt(t, cmds["sibling"], Prompt{Name: "p", Template: "t"})
		if got := FindDuplicate(root); got == nil || got.Kind != KindPrompt {
			t.Fatalf("FindDuplicate = %+v, want the prompt conflict first", got)
		}
	})
	t.Run("duplicate under a hidden root still fails", func(t *testing.T) {
		root := &cobra.Command{Use: "app", Hidden: true}
		child := &cobra.Command{Use: "child", RunE: noopRunE}
		root.AddCommand(child)
		mustAddResource(t, root, Resource{URI: "app://docs", Name: "a"})
		mustAddResource(t, child, Resource{URI: "app://docs", Name: "b"})
		if got := FindDuplicate(root); got == nil {
			t.Fatal("FindDuplicate missed a duplicate under a hidden root")
		}
	})
}

func TestAddRefusesToRewriteCorruptAnnotation(t *testing.T) {
	cases := []struct {
		name string
		key  string
		raw  string
		add  func(*cobra.Command) *Violation
	}{
		{name: "undecodable prompts", key: promptsAnnotationKey, raw: "{not json",
			add: func(c *cobra.Command) *Violation { return AddPrompt(c, Prompt{Name: "ok", Template: "t"}) }},
		{name: "invalid prompt entry", key: promptsAnnotationKey, raw: `[{"name":"bad name","template":"x"}]`,
			add: func(c *cobra.Command) *Violation { return AddPrompt(c, Prompt{Name: "ok", Template: "t"}) }},
		{name: "undecodable resources", key: resourcesAnnotationKey, raw: "{not json",
			add: func(c *cobra.Command) *Violation { return AddResource(c, Resource{URI: "app://ok", Name: "ok"}) }},
		{name: "invalid resource entry", key: resourcesAnnotationKey, raw: `[{"uri":"relative","name":"x"}]`,
			add: func(c *cobra.Command) *Violation { return AddResource(c, Resource{URI: "app://ok", Name: "ok"}) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "app", Annotations: map[string]string{tc.key: tc.raw}}
			want := &Violation{Field: "annotation", Reason: ReasonCorruptAnnotation}
			if got := tc.add(cmd); !reflect.DeepEqual(got, want) {
				t.Fatalf("add = %+v, want %+v", got, want)
			}
			if cmd.Annotations[tc.key] != tc.raw {
				t.Fatalf("annotation rewritten to %q; it must be left for __schema to report", cmd.Annotations[tc.key])
			}
		})
	}
}

func TestFindCorrupt(t *testing.T) {
	t.Run("clean tree", func(t *testing.T) {
		root, cmds := declarationTree()
		mustAddPrompt(t, cmds["leaf"], Prompt{Name: "p", Template: "t"})
		mustAddResource(t, root, Resource{URI: "app://r", Name: "r"})
		if got := FindCorrupt(root); got != nil {
			t.Fatalf("FindCorrupt = %+v, want nil", got)
		}
	})
	t.Run("first corrupt command in walk order, prompts before resources", func(t *testing.T) {
		root, cmds := declarationTree()
		cmds["group"].Annotations = map[string]string{resourcesAnnotationKey: "nope", promptsAnnotationKey: "nope"}
		cmds["sibling"].Annotations = map[string]string{promptsAnnotationKey: "nope"}
		want := &Conflict{
			Kind:     KindPrompt,
			Key:      promptsAnnotationKey,
			Reason:   ReasonCorruptAnnotation,
			Commands: []string{"app group"},
		}
		if got := FindCorrupt(root); !reflect.DeepEqual(got, want) {
			t.Fatalf("FindCorrupt = %+v, want %+v", got, want)
		}
	})
	t.Run("corrupt resources", func(t *testing.T) {
		root, cmds := declarationTree()
		cmds["leaf"].Annotations = map[string]string{resourcesAnnotationKey: `[{"uri":"relative","name":"x"}]`}
		want := &Conflict{
			Kind:     KindResource,
			Key:      resourcesAnnotationKey,
			Reason:   ReasonCorruptAnnotation,
			Commands: []string{"app group leaf"},
		}
		if got := FindCorrupt(root); !reflect.DeepEqual(got, want) {
			t.Fatalf("FindCorrupt = %+v, want %+v", got, want)
		}
	})
	t.Run("hidden subtree ignored", func(t *testing.T) {
		root, cmds := declarationTree()
		cmds["hiddenChild"].Annotations = map[string]string{promptsAnnotationKey: "nope"}
		if got := FindCorrupt(root); got != nil {
			t.Fatalf("FindCorrupt = %+v, want nil for a hidden subtree", got)
		}
	})
}

func mustAddResource(t *testing.T, cmd *cobra.Command, resource Resource) {
	t.Helper()
	if v := AddResource(cmd, resource); v != nil {
		t.Fatalf("AddResource(%q) on %q = %+v", resource.URI, cmd.Name(), v)
	}
}

// referencePlaceholders is the original one-"{{"-at-a-time Option A scanner,
// kept as an oracle: the production scanner's linear jump must agree with it.
func referencePlaceholders(template string) []string {
	var names []string
	for rest := template; ; {
		open := strings.Index(rest, "{{")
		if open < 0 {
			return names
		}
		rest = rest[open+2:]
		end := strings.Index(rest, "}}")
		if end < 0 {
			return names
		}
		if name := rest[:end]; validName(name) {
			names = append(names, name)
			rest = rest[end+2:]
		}
	}
}

func FuzzTemplatePlaceholders(f *testing.F) {
	for _, seed := range []string{
		"", "{{a}}", "{{a}} {{b}}", "{{ a }}", "{{{x}}}", "{{a}}}", "{{{{{a}}", "{{{{a}}",
		"{{a{{b}}", "{{a", "}}{{", "{{}}", "x{{y}}z{{", "{{a.b-c_d}}", "{{é}}",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, template string) {
		got := templatePlaceholders(template)
		if want := referencePlaceholders(template); !slices.Equal(got, want) {
			t.Fatalf("templatePlaceholders(%q) = %q, reference = %q", template, got, want)
		}
		for _, name := range got {
			if !validName(name) || !strings.Contains(template, "{{"+name+"}}") {
				t.Fatalf("placeholder %q is not a literal {{valid name}} in %q", name, template)
			}
		}

		var args []PromptArgument
		seen := map[string]bool{}
		for _, name := range got {
			if !seen[name] {
				seen[name] = true
				args = append(args, PromptArgument{Name: name})
			}
		}
		prompt := Prompt{Name: "fuzz", Arguments: args, Template: template}
		if ValidatePrompt(prompt) != nil {
			return
		}
		cmd := &cobra.Command{Use: "app"}
		if v := AddPrompt(cmd, prompt); v != nil {
			t.Fatalf("AddPrompt rejected a valid prompt: %+v", v)
		}
		if stored := Prompts(cmd.Annotations); len(stored) != 1 || !reflect.DeepEqual(stored[0], prompt) {
			t.Fatalf("prompt did not round-trip: %+v", stored)
		}
	})
}

func FuzzValidateResource(f *testing.F) {
	f.Add("app://docs", "docs", "text/plain; charset=utf-8")
	f.Add("urn:app:x", "x", "")
	f.Add("relative", "x", "text/plain")
	f.Add("app://%zz", "x", "")
	f.Add("app://a b", "x", "\n")
	f.Fuzz(func(t *testing.T, uri, name, mimeType string) {
		resource := Resource{URI: uri, Name: name, MIMEType: mimeType}
		if ValidateResource(resource) != nil {
			return
		}
		cmd := &cobra.Command{Use: "app"}
		if v := AddResource(cmd, resource); v != nil {
			t.Fatalf("AddResource rejected a valid resource: %+v", v)
		}
		if stored := Resources(cmd.Annotations); len(stored) != 1 || stored[0] != resource {
			t.Fatalf("resource did not round-trip: %+v", stored)
		}
	})
}
