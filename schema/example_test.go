package schema_test

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go/contract"
	"github.com/rshade/ax-go/schema"
)

func ExampleBuildSchema() {
	root := &cobra.Command{
		Use:     "app",
		Short:   "test app",
		Example: "app run",
	}
	root.Flags().String("config", "", "config file")

	got := schema.BuildSchema(root, schema.WithSchemaVersion("v0.1.0"))
	fmt.Println(got.Tool)
	fmt.Println(got.Version)
	fmt.Println(got.Command.Flags[0].Name)
	// Output:
	// app
	// v0.1.0
	// config
}

func ExampleBuildMCPSchema() {
	root := &cobra.Command{
		Use:   "app",
		Short: "test app",
		RunE:  func(*cobra.Command, []string) error { return nil },
	}
	root.Flags().String("config", "", "config file")

	got := schema.BuildMCPSchema(root)
	fmt.Println(got.Tools[0].Name)
	fmt.Println(got.Tools[0].InputSchema["type"])
	// Output:
	// app
	// object
}

func ExampleDeclarePrompt() {
	root := &cobra.Command{Use: "app"}
	err := schema.DeclarePrompt(root, schema.Prompt{
		Name:  "triage-spike",
		Title: "Triage a cost spike",
		Arguments: []schema.PromptArgument{
			{Name: "window", Description: "lookback, e.g. 7d", Required: true},
		},
		Template: "Run `app report --since={{window}}`, then explain the top line item.",
	})
	if err != nil {
		fmt.Println(err)
		return
	}

	prompt := schema.BuildMCPSchema(root).Prompts[0]
	fmt.Println(prompt.Name, prompt.Arguments[0].Name)

	// A placeholder must name a declared argument.
	err = schema.DeclarePrompt(root, schema.Prompt{Name: "broken", Template: "{{missing}}"})
	fmt.Println(err)
	// Output:
	// triage-spike window
	// invalid prompt declaration "broken": template undeclared_placeholder
}

func ExampleDeclareResource() {
	root := &cobra.Command{Use: "app"}
	err := schema.DeclareResource(root, schema.Resource{
		URI:      "app://docs/pricing-model",
		Name:     "pricing-model",
		MIMEType: "text/markdown",
	})
	if err != nil {
		fmt.Println(err)
		return
	}

	resource := schema.BuildSchema(root).Command.Resources[0]
	fmt.Println(resource.URI, resource.MIMEType)

	// Resources are addressed by an absolute URI.
	err = schema.DeclareResource(root, schema.Resource{URI: "docs/pricing", Name: "relative"})
	fmt.Println(err)
	// Output:
	// app://docs/pricing-model text/markdown
	// invalid resource declaration "docs/pricing": uri not_absolute
}

func ExampleDeclareFlagEnum() {
	deploy := &cobra.Command{Use: "deploy"}
	deploy.Flags().String("output", "json", "output format")
	if err := schema.DeclareFlagEnum(deploy, "output", "json", "table", "yaml"); err != nil {
		fmt.Println(err)
		return
	}

	if err := contract.WriteJSON(os.Stdout, schema.BuildSchema(deploy).Command.Flags); err != nil {
		fmt.Println(err)
	}
	// Output:
	// [{"name":"output","type":"string","default":"json","usage":"output format","enum":["json","table","yaml"]}]
}

func ExampleDeclareFlagExample() {
	deploy := &cobra.Command{Use: "deploy"}
	deploy.Flags().Duration("timeout", 30*time.Second, "deadline")
	if err := schema.DeclareFlagExample(deploy, "timeout", "45s"); err != nil {
		fmt.Println(err)
		return
	}

	if err := contract.WriteJSON(os.Stdout, schema.BuildSchema(deploy).Command.Flags); err != nil {
		fmt.Println(err)
	}
	// Output:
	// [{"name":"timeout","type":"duration","default":"30s","usage":"deadline","example":"45s"}]
}

func ExampleDeclareCapability() {
	root := &cobra.Command{Use: "app"}
	deploy := &cobra.Command{Use: "deploy"}
	root.AddCommand(deploy)
	if err := schema.DeclareCapability(deploy, schema.CapabilityMutate, "idempotent by release name"); err != nil {
		fmt.Println(err)
		return
	}

	if err := contract.WriteJSON(os.Stdout, schema.BuildSchema(root).Command.Commands[0].Capability); err != nil {
		fmt.Println(err)
	}
	// Output:
	// {"class":"mutate","note":"idempotent by release name"}
}
