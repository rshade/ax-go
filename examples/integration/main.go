package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/rshade/ax-go"
	"github.com/rshade/ax-go/mcp"
)

var version string

const defaultStreamCount = 3
const appName = "ax-integration"
const failCommandName = "fail"
const fetchCommandName = "fetch"
const authzCommandName = "authz"
const crashCommandName = "crash"
const patchConfigCommandName = "patch-config"
const streamCommandName = "stream"
const lokiFlushBudget = 3 * time.Second
const fetchRetryAfterSeconds = 5

// errSimulatedCrash is the sentinel cause wrapped by the crash command. It is a
// bare (non-ax) error on purpose: it shows how ax.Execute maps any unexpected
// error a downstream RunE returns into the framework's internal_error envelope
// with exit code 1.
var errSimulatedCrash = errors.New("simulated downstream component failure")

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

type patchConfigPayload struct {
	Path    string `json:"path"`
	Patched bool   `json:"patched"`
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv))
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, env func(string) string) int {
	return runWithEntityID(ctx, args, stdin, stdout, stderr, env, ax.ResolveVersion(version), ax.NewEntityID)
}

// runWithEntityID is the test seam for run: it injects the resolved version and
// entity-ID generator so golden and determinism tests can pin both. Production
// run passes ax.ResolveVersion(version) and ax.NewEntityID.
func runWithEntityID(
	ctx context.Context,
	args []string,
	stdin io.Reader,
	stdout, stderr io.Writer,
	env func(string) string,
	resolved string,
	newEntityID func() (string, error),
) int {
	root := newRootCommand(stdin, resolved, newEntityID)
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

func newRootCommand(stdin io.Reader, resolved string, newEntityID func() (string, error)) *cobra.Command {
	var name string
	var configPath string

	root := &cobra.Command{
		Use:   appName,
		Short: "Exercise ax-go primitives from a real Cobra command",
		Example: `  ax-integration --format=json --name Ada
  OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 ax-integration --format=json
  AX_OTEL_DEBUG=1 ax-integration --format=json
  ax-integration --config=config.hujson
  ax-integration stream --count=3
  ax-integration patch-config --config=config.hujson --patch='[{"op":"replace","path":"/count","value":5}]'
  ax-integration fail
  ax-integration fetch
  ax-integration authz
  ax-integration crash
  ax-integration __schema`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			logger := ax.NewLogger(
				cmd.Context(),
				ax.WithLoggerWriter(cmd.ErrOrStderr()),
				ax.WithLoggerLabels(ax.Labels{
					Application: appName,
					Version:     resolved,
				}),
				ax.WithLokiFromEnv(),
			)
			defer func() {
				flushCtx, cancel := context.WithTimeout(context.Background(), lokiFlushBudget)
				defer cancel()
				_ = ax.Flush(flushCtx, logger)
			}()

			mode, _ := ax.ModeFromContext(cmd.Context())
			entityID, err := newEntityID()
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
	root.AddCommand(newPatchConfigCommand())
	root.AddCommand(newFailCommand())
	root.AddCommand(newFetchCommand())
	root.AddCommand(newAuthzCommand())
	root.AddCommand(newCrashCommand())

	// Opt in to the MCP server: `ax-integration mcp-server` exposes this CLI's
	// command tree as a live MCP server with no per-tool work (feature 011).
	root.AddCommand(mcp.NewCommand(root, mcp.WithVersion(resolved)))

	return root
}

func newStreamCommand() *cobra.Command {
	var name string
	var count int

	cmd := &cobra.Command{
		Use:   streamCommandName,
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

func newPatchConfigCommand() *cobra.Command {
	var configPath string
	var patchDoc string

	cmd := &cobra.Command{
		Use:   patchConfigCommandName,
		Short: "Apply an RFC 6902 patch to a Hujson config file, preserving comments",
		Example: `  ax-integration patch-config --config=config.hujson --patch='[{"op":"replace","path":"/count","value":5}]'
  ax-integration patch-config --format=json --config=config.hujson --patch='[{"op":"add","path":"/name","value":"Ada"}]'`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if configPath == "" || patchDoc == "" {
				return ax.NewError(
					cmd.Context(),
					"validation_error",
					"both --config and --patch are required",
					ax.WithActionableFix(
						"pass --config with a Hujson file path and --patch with an RFC 6902 JSON array",
					),
					ax.WithErrorExitCode(ax.ExitValidation),
				)
			}

			if err := ax.Perform(
				cmd.Context(),
				func(ctx context.Context) error { return rehearsePatchConfig(ctx, configPath, patchDoc) },
				func(ctx context.Context) error {
					return ax.PatchConfigFile(ctx, configPath, []byte(patchDoc))
				},
			); err != nil {
				return err
			}

			payload := patchConfigPayload{Path: configPath, Patched: true}
			return ax.WriteJSON(cmd.OutOrStdout(), ax.NewEnvelope(cmd.Context(), payload))
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Hujson config file path to patch in place")
	cmd.Flags().StringVar(&patchDoc, "patch", "", "RFC 6902 JSON patch array to apply")

	return cmd
}

// rehearsePatchConfig is the dry-run rehearsal passed to ax.Perform: it reads
// the config and applies the patch in memory so a dry-run surfaces the same
// errors as a real run (missing file, invalid Hujson, invalid patch), then
// discards the result. The success envelope is byte-identical to a real run
// apart from meta.dry_run.
func rehearsePatchConfig(ctx context.Context, configPath, patchDoc string) error {
	file, err := os.Open(configPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = ax.PatchConfig(ctx, file, []byte(patchDoc))
	return err
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
				ax.WithRetryable(false),
				ax.WithErrorExitCode(ax.ExitValidation),
			)
		},
	}
}

// newFetchCommand models a network/timeout failure: exit code 3 (ExitNetwork).
// It demonstrates the feature 013 recovery fields — a network failure is
// retryable, so the envelope advertises retryable=true and a retry_after_seconds
// backoff hint an agent can honor before retrying.
func newFetchCommand() *cobra.Command {
	return &cobra.Command{
		Use:   fetchCommandName,
		Short: "Model a network/timeout failure with retry recovery fields (exit 3)",
		Example: `  ax-integration fetch --format=json
  ax-integration fetch --idempotency-key=demo-key`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return ax.NewError(
				cmd.Context(),
				"upstream_unreachable",
				"upstream service did not respond within the timeout",
				ax.WithActionableFix("retry after the backoff window or check upstream health"),
				ax.WithRetryable(true),
				ax.WithRetryAfterSeconds(fetchRetryAfterSeconds),
				ax.WithErrorExitCode(ax.ExitNetwork),
			)
		},
	}
}

// newAuthzCommand models an authentication/permission failure: exit code 4
// (ExitAuth). A permission failure is not fixable by a naive retry, so the
// envelope advertises retryable=false.
func newAuthzCommand() *cobra.Command {
	return &cobra.Command{
		Use:   authzCommandName,
		Short: "Model an authentication/permission failure (exit 4)",
		Example: `  ax-integration authz --format=json
  ax-integration authz --idempotency-key=demo-key`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return ax.NewError(
				cmd.Context(),
				"permission_denied",
				"the supplied credentials lack permission for this action",
				ax.WithActionableFix("re-authenticate or request access to the resource"),
				ax.WithRetryable(false),
				ax.WithErrorExitCode(ax.ExitAuth),
			)
		},
	}
}

// newCrashCommand models an unexpected internal error: exit code 1
// (ExitInternal). It returns a bare (non-ax) error so ax.Execute wraps it into
// the framework's internal_error envelope — the pattern any downstream bug
// follows.
func newCrashCommand() *cobra.Command {
	return &cobra.Command{
		Use:     crashCommandName,
		Short:   "Return an unexpected non-ax error to demonstrate the internal_error mapping (exit 1)",
		Example: `  ax-integration crash --format=json`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("processing the request: %w", errSimulatedCrash)
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
