package ax

import (
	"os"

	"github.com/rshade/ax-go/contract"
)

// Mode describes whether output should be optimized for agents or humans.
type Mode = contract.Mode

const (
	// ModeJSON is the machine-readable mode used by agents and pipelines.
	ModeJSON = contract.ModeJSON
	// ModeHuman is the human-readable mode used for interactive terminals.
	ModeHuman = contract.ModeHuman
)

// ModeDetectionRule documents the output-mode resolution precedence applied by
// ResolveMode. It is surfaced verbatim in __schema output.
const ModeDetectionRule = contract.ModeDetectionRule

// ParseMode parses an explicit output mode.
func ParseMode(value string) (Mode, error) {
	return contract.ParseMode(value)
}

// ResolveMode applies output-mode precedence:
// explicit --format flag, then AGENT_MODE, then TTY detection.
func ResolveMode(explicitFormat, agentMode string, stdoutIsTTY bool) (Mode, error) {
	return contract.ResolveMode(explicitFormat, agentMode, stdoutIsTTY)
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
