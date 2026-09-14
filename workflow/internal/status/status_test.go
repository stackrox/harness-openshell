package status

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestCmdRedactsCredentialValues(t *testing.T) {
	old := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	Verbose = true
	Cmd("example", "run", "--credential", "TOKEN=secret", "--env", "API_KEY=also-secret", "--env", "MODE=check")
	_ = writer.Close()
	os.Stderr = old
	Verbose = false

	var output bytes.Buffer
	_, _ = output.ReadFrom(reader)
	got := output.String()
	if strings.Contains(got, "secret") || !strings.Contains(got, "TOKEN=***") || !strings.Contains(got, "API_KEY=***") {
		t.Errorf("redacted command = %q", got)
	}
	if !strings.Contains(got, "MODE=check") {
		t.Errorf("non-sensitive value was hidden: %q", got)
	}
}

func TestHeader(t *testing.T) {
	old := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	Header("Sandboxes")
	_ = writer.Close()
	os.Stdout = old

	var output bytes.Buffer
	_, _ = output.ReadFrom(reader)
	if !strings.Contains(output.String(), "Sandboxes") || !strings.Contains(output.String(), "─") {
		t.Errorf("header output = %q", output.String())
	}
}
