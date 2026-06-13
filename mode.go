package ax

import (
	"fmt"
	"os"
	"strings"
)

// Mode describes whether output should be optimized for agents or humans.
type Mode string

const (
	// ModeJSON is the machine-readable mode used by agents and pipelines.
	ModeJSON Mode = "json"
	// ModeHuman is the human-readable mode used for interactive terminals.
	ModeHuman Mode = "human"
)

const truthyTrue = "true"

// ModeDetectionRule documents the ADR-0001 resolution precedence applied by
// ResolveMode. It is surfaced verbatim in __schema output.
const ModeDetectionRule = "--format flag > AGENT_MODE env > TTY detection"

// String returns the wire value for the mode.
func (m Mode) String() string {
	return string(m)
}

// ParseMode parses an explicit output mode.
func ParseMode(value string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "json", "agent", "machine":
		return ModeJSON, nil
	case string(ModeHuman), "text":
		return ModeHuman, nil
	default:
		return "", fmt.Errorf("unknown output mode %q", value)
	}
}

// ResolveMode applies ADR-0001 precedence:
// explicit --format flag, then AGENT_MODE, then TTY detection.
func ResolveMode(explicitFormat, agentMode string, stdoutIsTTY bool) (Mode, error) {
	if strings.TrimSpace(explicitFormat) != "" {
		return ParseMode(explicitFormat)
	}

	if strings.TrimSpace(agentMode) != "" {
		return parseAgentMode(agentMode)
	}

	if stdoutIsTTY {
		return ModeHuman, nil
	}
	return ModeJSON, nil
}

func parseAgentMode(value string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", truthyTrue, "yes", "on", "json", "agent", "machine":
		return ModeJSON, nil
	case "0", "false", "no", "off", string(ModeHuman), "text":
		return ModeHuman, nil
	default:
		return "", fmt.Errorf("unknown AGENT_MODE value %q", value)
	}
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
