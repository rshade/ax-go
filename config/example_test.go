package config_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rshade/ax-go/config"
)

func ExampleParse() {
	const hujson = `{
		// comments and trailing commas are allowed on reads
		"name": "ax",
		"replicas": 3,
	}`

	var cfg struct {
		Name     string `json:"name"`
		Replicas int    `json:"replicas"`
	}
	if err := config.Parse(
		context.Background(),
		strings.NewReader(hujson),
		&cfg,
		config.WithMaxBytes(1<<10),
	); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%s x%d\n", cfg.Name, cfg.Replicas)
	// Output: ax x3
}

func ExamplePatch() {
	const existing = `{
	// service endpoint
	"host": "localhost",
	"port": 8080,
}`
	patch := []byte(`[{"op":"replace","path":"/port","value":9090}]`)

	patched, err := config.Patch(
		context.Background(),
		strings.NewReader(existing),
		patch,
	)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(strings.Contains(string(patched), "// service endpoint"))
	fmt.Println(strings.Contains(string(patched), "9090"))
	// Output:
	// true
	// true
}

func ExampleParseFile() {
	dir, err := os.MkdirTemp("", "ax-config")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "config.hujson")
	if err := os.WriteFile(path, []byte(`{"name": "ax"}`), 0o600); err != nil {
		fmt.Println("error:", err)
		return
	}

	var cfg struct {
		Name string `json:"name"`
	}
	if err := config.ParseFile(context.Background(), path, &cfg); err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(cfg.Name)
	// Output: ax
}

func ExamplePatchFile() {
	dir, err := os.MkdirTemp("", "ax-patch")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "config.hujson")
	initial := []byte(`{
	// production endpoint
	"host": "prod.example.com",
	"port": 443,
}`)
	if err := os.WriteFile(path, initial, 0o600); err != nil {
		fmt.Println("error:", err)
		return
	}

	patch := []byte(`[{"op":"replace","path":"/port","value":8443}]`)
	if err := config.PatchFile(context.Background(), path, patch); err != nil {
		fmt.Println("error:", err)
		return
	}

	result, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(strings.Contains(string(result), "// production endpoint"))
	fmt.Println(strings.Contains(string(result), "8443"))
	// Output:
	// true
	// true
}
