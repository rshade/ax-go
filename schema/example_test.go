package schema_test

import (
	"fmt"

	"github.com/spf13/cobra"

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
	}
	root.Flags().String("config", "", "config file")

	got := schema.BuildMCPSchema(root)
	fmt.Println(got.Tools[0].Name)
	fmt.Println(got.Tools[0].InputSchema["type"])
	// Output:
	// app
	// object
}
