package review

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

var commentLocation = regexp.MustCompile(`(?i)comment\s+(position|line)\s+(is|was)\s+(invalid|unresolvable|not\s+part\s+of\s+the\s+diff)`)
var unprocessable = regexp.MustCompile(`(?i)422|unprocessable\s+entity`)
var commentContext = regexp.MustCompile(`(?i)comment|review|pull\s+request`)
var diffLocation = regexp.MustCompile(`(?i)position|line|side|diff\s+hunk`)
var shellParse = regexp.MustCompile(`(?i)unexpected EOF while looking for matching|syntax error near unexpected token`)

type event struct {
	Type string `json:"type"`
	Part struct {
		Tool   string `json:"tool"`
		Text   string `json:"text"`
		Reason string `json:"reason"`
		State  struct {
			Status   string `json:"status"`
			Metadata struct {
				Exit *int `json:"exit"`
			} `json:"metadata"`
			Output string `json:"output"`
			Error  string `json:"error"`
		} `json:"state"`
	} `json:"part"`
}

// ValidateOutput checks the OpenCode review protocol, not finding quality or
// publication correctness. Comments may already exist when validation runs.
func ValidateOutput(path string) (string, error) {
	data, err := readBounded(path, MaxOutputBytes)
	if err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), MaxOutputBytes+1)
	var text strings.Builder
	hasText, stopped := false, false
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		if line[0] != '{' && line[0] != '[' {
			continue
		}
		var e event
		if err := json.Unmarshal(line, &e); err != nil {
			return "", errors.New("malformed JSON event in agent output")
		}
		switch e.Type {
		case "text":
			hasText = hasText || strings.TrimSpace(e.Part.Text) != ""
			text.WriteString(e.Part.Text + "\n")
		case "error":
			return "", errors.New("agent reported an error")
		case "step_finish":
			if e.Part.Reason != "stop" && e.Part.Reason != "tool-calls" {
				return "", errors.New("agent did not complete its review")
			}
			stopped = stopped || e.Part.Reason == "stop"
		case "tool_use":
			state := e.Part.State
			if state.Status != "completed" {
				return "", errors.New("agent tool did not complete")
			}
			if state.Metadata.Exit == nil {
				if e.Part.Tool == "" || e.Part.Tool == "bash" {
					return "", errors.New("shell tool result requires an exit code")
				}
				continue // Completed non-shell tools need not have a process exit code.
			}
			exit := *state.Metadata.Exit
			output := state.Output
			if output == "" {
				output = state.Error
			}
			recoverable := exit == 1 && (commentLocation.MatchString(output) || (unprocessable.MatchString(output) && commentContext.MatchString(output) && diffLocation.MatchString(output)))
			recoverable = recoverable || (exit > 0 && shellParse.MatchString(output))
			if exit != 0 && !recoverable {
				return "", errors.New("agent tool failed")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if !hasText || !stopped {
		return "", errors.New("agent output requires nonempty text and a terminal stop event")
	}
	return text.String(), nil
}
