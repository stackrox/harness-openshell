package orchestrator

import "testing"

func TestParseSentinelValid(t *testing.T) {
	output := []byte(`some agent output
working on stuff
OPENSHELL_AGENT_RESULT {"status":"complete","reason":"done"}
`)
	result, err := ParseSentinel(output)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Status != "complete" {
		t.Errorf("status = %q, want complete", result.Status)
	}
	if result.Reason != "done" {
		t.Errorf("reason = %q, want done", result.Reason)
	}
}

func TestParseSentinelWithPollSeconds(t *testing.T) {
	output := []byte(`OPENSHELL_AGENT_RESULT {"status":"waiting","reason":"checks_pending","next_poll_seconds":120}`)
	result, err := ParseSentinel(output)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "waiting" {
		t.Errorf("status = %q, want waiting", result.Status)
	}
	if result.PollSeconds != 120 {
		t.Errorf("poll_seconds = %d, want 120", result.PollSeconds)
	}
}

func TestParseSentinelMultipleLines(t *testing.T) {
	output := []byte(`OPENSHELL_AGENT_RESULT {"status":"waiting","reason":"first"}
some output
OPENSHELL_AGENT_RESULT {"status":"complete","reason":"last"}
`)
	result, err := ParseSentinel(output)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "complete" {
		t.Errorf("should return last sentinel, got status %q", result.Status)
	}
	if result.Reason != "last" {
		t.Errorf("reason = %q, want last", result.Reason)
	}
}

func TestParseSentinelNoSentinel(t *testing.T) {
	output := []byte("just regular output\nno sentinel here\n")
	result, err := ParseSentinel(output)
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Errorf("expected nil result for no sentinel, got %+v", result)
	}
}

func TestParseSentinelMalformedJSON(t *testing.T) {
	output := []byte(`OPENSHELL_AGENT_RESULT {not valid json}`)
	_, err := ParseSentinel(output)
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestParseSentinelEmptyOutput(t *testing.T) {
	result, err := ParseSentinel([]byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Error("expected nil for empty output")
	}
}

func TestSyntheticResultSuccess(t *testing.T) {
	r := SyntheticResult(0)
	if r.Status != "complete" {
		t.Errorf("status = %q, want complete", r.Status)
	}
}

func TestSyntheticResultFailure(t *testing.T) {
	r := SyntheticResult(1)
	if r.Status != "terminal_failure" {
		t.Errorf("status = %q, want terminal_failure", r.Status)
	}
	if r.ExitCode != 1 {
		t.Errorf("exit_code = %d, want 1", r.ExitCode)
	}
}

func TestCycleResultIsTerminal(t *testing.T) {
	cases := []struct {
		status   string
		terminal bool
	}{
		{"complete", true},
		{"terminal_failure", true},
		{"waiting", false},
		{"blocked", false},
		{"transient_failure", false},
		{"propose", false},
	}
	for _, tc := range cases {
		r := CycleResult{Status: tc.status}
		if r.IsTerminal() != tc.terminal {
			t.Errorf("IsTerminal(%q) = %v, want %v", tc.status, r.IsTerminal(), tc.terminal)
		}
	}
}
