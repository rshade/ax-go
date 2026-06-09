package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go"
)

var version string

const defaultStreamCount = 3
const appName = "ax-integration"
const failCommandName = "fail"

type integrationConfig struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type helloPayload struct {
	Greeting string             `json:"greeting"`
	Name     string             `json:"name"`
	Mode     string             `json:"mode"`
	EntityID string             `json:"entity_id"`
	Config   *integrationConfig `json:"config,omitempty"`
}

type streamPayload struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv))
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, env func(string) string) int {
	resolved := ax.ResolveVersion(version)
	root := newRootCommand(stdin, resolved)
	root.SetArgs(args)

	return ax.Execute(
		ctx,
		root,
		ax.WithStdin(stdin),
		ax.WithStdout(stdout),
		ax.WithStderr(stderr),
		ax.WithEnv(env),
		ax.WithVersion(resolved),
	)
}

func newRootCommand(stdin io.Reader, resolved string) *cobra.Command {
	var name string
	var configPath string

	root := &cobra.Command{
		Use:   appName,
		Short: "Exercise ax-go primitives from a real Cobra command",
		Example: `  ax-integration --format=json --name Ada
  ax-integration --config=config.hujson
  ax-integration stream --count=3
  ax-integration fail
  ax-integration __schema`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger := ax.NewLogger(
				cmd.Context(),
				ax.WithLoggerWriter(cmd.ErrOrStderr()),
				ax.WithLoggerLabels(ax.Labels{
					Application: appName,
					Version:     resolved,
				}),
			)

			mode, _ := ax.ModeFromContext(cmd.Context())
			entityID, err := ax.NewEntityID()
			if err != nil {
				return fmt.Errorf("create entity id: %w", err)
			}

			cfg, hasConfig, err := readConfig(cmd.Context(), stdin, configPath)
			if err != nil {
				return err
			}

			logger.Info(cmd.Context()).Str("event", "integration_run").Str("name", name).Msg("integration example ran")

			payload := helloPayload{
				Greeting: "hello",
				Name:     name,
				Mode:     mode.String(),
				EntityID: entityID,
			}
			if hasConfig {
				payload.Config = &cfg
			}
			return ax.WriteJSON(cmd.OutOrStdout(), ax.NewEnvelope(cmd.Context(), payload))
		},
	}

	root.Flags().StringVar(&name, "name", "agent", "name to include in the JSON payload")
	root.Flags().StringVar(&configPath, "config", "", "optional Hujson config file path, or - for stdin")
	root.AddCommand(newStreamCommand())
	root.AddCommand(newFailCommand())

	return root
}

func newStreamCommand() *cobra.Command {
	var name string
	var count int

	cmd := &cobra.Command{
		Use:   "stream",
		Short: "Emit NDJSON envelopes",
		Example: `  ax-integration stream --format=json --count=3
  ax-integration stream --count=3 --idempotency-key=demo-key`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if count < 0 {
				return ax.NewError(
					cmd.Context(),
					"validation_error",
					"count must be greater than or equal to zero",
					ax.WithActionableFix("pass --count with a non-negative integer"),
					ax.WithErrorExitCode(ax.ExitValidation),
				)
			}

			for i := range count {
				payload := streamPayload{
					Index: i,
					Name:  name,
				}
				if err := ax.WriteJSONLine(cmd.OutOrStdout(), ax.NewEnvelope(cmd.Context(), payload)); err != nil {
					return fmt.Errorf("write stream item: %w", err)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "agent", "name to include in each streamed item")
	cmd.Flags().IntVar(&count, "count", defaultStreamCount, "number of NDJSON items to emit")

	return cmd
}

func newFailCommand() *cobra.Command {
	return &cobra.Command{
		Use:   failCommandName,
		Short: "Return an intentional ax.Error envelope",
		Example: `  ax-integration fail --format=json
  ax-integration fail --idempotency-key=demo-key`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// ax.Error context is intentionally flexible metadata.
			return ax.NewError(
				cmd.Context(),
				"integration_failure",
				"intentional integration failure",
				ax.WithActionableFix("run a non-failing subcommand"),
				ax.WithErrorContext(map[string]any{"example": failCommandName}),
				ax.WithErrorExitCode(ax.ExitValidation),
			)
		},
	}
}

func readConfig(ctx context.Context, stdin io.Reader, path string) (integrationConfig, bool, error) {
	if path == "" {
		return integrationConfig{}, false, nil
	}
	if err := ctx.Err(); err != nil {
		return integrationConfig{}, false, fmt.Errorf("read config canceled: %w", err)
	}

	var cfg integrationConfig
	if path == "-" {
		if err := ax.ParseConfig(ctx, stdin, &cfg); err != nil {
			return integrationConfig{}, false, fmt.Errorf("parse stdin config: %w", err)
		}
		return cfg, true, nil
	}

	if err := ax.ParseConfigFile(ctx, path, &cfg); err != nil {
		return integrationConfig{}, false, fmt.Errorf("parse config: %w", err)
	}
	return cfg, true, nil
}
