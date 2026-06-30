package orchestrator

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const SentinelPrefix = "OPENSHELL_AGENT_RESULT "

type CycleResult struct {
	Status      string   `json:"status"`
	Reason      string   `json:"reason,omitempty"`
	PollSeconds int      `json:"next_poll_seconds,omitempty"`
	Artifacts   []string `json:"artifacts,omitempty"`
	ExitCode    int      `json:"exit_code,omitempty"`
}

func (r *CycleResult) IsTerminal() bool {
	return r.Status == "complete" || r.Status == "terminal_failure"
}

// ParseSentinel scans agent stdout for the last OPENSHELL_AGENT_RESULT line
// and parses the JSON payload. Returns nil if no sentinel is found.
func ParseSentinel(output []byte) (*CycleResult, error) {
	lines := bytes.Split(bytes.TrimRight(output, "\n"), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if !bytes.HasPrefix(line, []byte(SentinelPrefix)) {
			continue
		}
		payload := line[len(SentinelPrefix):]
		var result CycleResult
		if err := json.Unmarshal(payload, &result); err != nil {
			return nil, fmt.Errorf("malformed sentinel JSON: %w", err)
		}
		return &result, nil
	}
	return nil, nil
}

// SyntheticResult creates a CycleResult from a process exit code when no
// sentinel protocol is used.
func SyntheticResult(exitCode int) *CycleResult {
	if exitCode == 0 {
		return &CycleResult{Status: "complete", Reason: "exit_0", ExitCode: 0}
	}
	return &CycleResult{Status: "terminal_failure", Reason: fmt.Sprintf("exit_%d", exitCode), ExitCode: exitCode}
}
