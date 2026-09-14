package review

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestOutputProtocol(t *testing.T) {
	for _, tc := range []struct {
		name, extra string
		pass        bool
	}{
		{"complete", "", true},
		{"malformed-after-stop", "{\"type\":\"error\",", false},
		{"error", `{"type":"error"}`, false},
		{"unfinished-tool", `{"type":"tool_use"}`, false},
		{"read-tool", `{"type":"tool_use","part":{"tool":"read","state":{"status":"completed","metadata":{},"output":"read succeeded"}}}`, true},
		{"bash-missing-exit", `{"type":"tool_use","part":{"tool":"bash","state":{"status":"completed","metadata":{}}}}`, false},
		{"read-tool-failed", `{"type":"tool_use","part":{"tool":"read","state":{"status":"error","metadata":{}}}}`, false},
		{"read-tool-nonzero", `{"type":"tool_use","part":{"tool":"read","state":{"status":"completed","metadata":{"exit":7}}}}`, false},
		{"truncated", `{"type":"step_finish","part":{"reason":"length"}}`, false},
		{"tool-calls", `{"type":"step_finish","part":{"reason":"tool-calls"}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "agent.ndjson")
			mustWrite(t, path, successOutput+tc.extra)
			_, err := ValidateOutput(path)
			if (err == nil) != tc.pass {
				t.Fatalf("validation: %v", err)
			}
		})
	}
	for _, tc := range []struct {
		name, output string
		exit         any
		pass         bool
	}{
		{"success", "ok", 0, true},
		{"failure", "ordinary command failed", 7, false},
		{"missing-exit", "missing exit", nil, false},
		{"shell-parser", "unexpected EOF while looking for matching quote", 2, true},
		{"shell-syntax", "syntax error near unexpected token", 2, true},
		{"position", "comment position is invalid", 1, true},
		{"api-location", "422 unprocessable entity: review comment line outside diff hunk", 1, true},
		{"unrelated-422", "build failed at record 422", 1, false},
		{"unrelated-line", "build failed at line 422", 1, false},
		{"unrelated-comment", "comment delivery failed with status 422", 1, false},
		{"comment-formatting", "comment formatting failed", 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(map[string]any{"type": "tool_use", "part": map[string]any{"state": map[string]any{"status": "completed", "metadata": map[string]any{"exit": tc.exit}, "output": tc.output}}})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "agent.ndjson")
			mustWrite(t, path, successOutput+string(data))
			_, err = ValidateOutput(path)
			if (err == nil) != tc.pass {
				t.Fatalf("validation: %v", err)
			}
		})
	}
	for _, data := range []string{"", `{"type":"text","part":{"text":"review"}}`, "{\"type\":\"text\",\"part\":{\"text\":\" \"}}\n{\"type\":\"step_finish\",\"part\":{\"reason\":\"stop\"}}"} {
		path := filepath.Join(t.TempDir(), "agent.ndjson")
		mustWrite(t, path, data)
		if _, err := ValidateOutput(path); err == nil {
			t.Fatalf("accepted incomplete output %q", data)
		}
	}
}
