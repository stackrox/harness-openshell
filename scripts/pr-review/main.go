// pr-review is trusted repository automation, not a sandbox tool. It fetches
// PR content only as data; the worker receives neither GitHub nor Vertex secrets.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const maxDiff = 200 * 1024

var repoPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*/[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var hunkPattern = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

type pull struct {
	State  string
	Draft  bool
	Head   struct{ SHA string }
	Base   struct{ SHA string }
	Labels []struct{ Name string }
}

type input struct {
	Repository string `json:"repository"`
	PR         int    `json:"pr"`
	Base       string `json:"base"`
	Head       string `json:"head"`
	DiffSHA256 string `json:"diffSha256"`
}

type finding struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type review struct {
	Findings []finding `json:"findings"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	err := run(ctx, os.Args[1:])
	if err != nil {
		// Errors contain trusted stage names/statuses, never HTTP/model bodies.
		fmt.Fprintln(os.Stderr, err)
		_ = summary("AI review failed. See workflow logs and diagnostic artifacts.\n")
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) != 1 || (args[0] != "prepare" && args[0] != "run") {
		return errors.New("usage: pr-review prepare|run")
	}
	dir := os.Getenv("REVIEW_DIR")
	if dir == "" {
		return errors.New("REVIEW_DIR is required")
	}
	if args[0] == "prepare" {
		return prepare(ctx, dir)
	}
	data, err := os.ReadFile(filepath.Join(dir, "input.json"))
	if err != nil {
		return err
	}
	var in input
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	diff, err := os.ReadFile(filepath.Join(dir, "pr.diff"))
	if err != nil {
		return err
	}
	if digest(diff) != in.DiffSHA256 {
		return errors.New("diff changed after preparation")
	}
	active, err := current(ctx, in)
	if err != nil {
		return err
	}
	if !active {
		return summary("AI review skipped: label, PR state, or source revisions changed.\n")
	}
	r, err := executeReview(ctx, dir, diff)
	if err != nil {
		return err
	}
	active, err = current(ctx, in)
	if err != nil {
		return err
	}
	if !active {
		return summary("AI review superseded: label, PR state, or source revisions changed. No findings published.\n")
	}
	if err := writeJSON(filepath.Join(dir, "review.json"), r); err != nil {
		return err
	}
	body, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	text := fmt.Sprintf("## AI review — PR #%d\n\nHead: `%s`\n\nDiff-only, untrusted model findings; not an approval.\n\n<pre>%s</pre>\n", in.PR, in.Head, html.EscapeString(string(body)))
	return summary(text)
}

func eligible(p pull, expected string) bool {
	if p.State != "open" || p.Draft || (expected != "" && p.Head.SHA != expected) {
		return false
	}
	for _, label := range p.Labels {
		if label.Name == "ai-review" {
			return true
		}
	}
	return false
}

func getPull(ctx context.Context, repo string, number int) (pull, error) {
	var p pull
	if !repoPattern.MatchString(repo) || number < 1 {
		return p, errors.New("invalid repository or PR")
	}
	data, err := github(ctx, fmt.Sprintf("repos/%s/pulls/%d", repo, number), "application/vnd.github+json", 2*1024*1024)
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return p, errors.New("invalid PR metadata")
	}
	if !shaPattern.MatchString(p.Head.SHA) || !shaPattern.MatchString(p.Base.SHA) {
		return p, errors.New("invalid PR revisions")
	}
	return p, nil
}

func current(ctx context.Context, in input) (bool, error) {
	p, err := getPull(ctx, in.Repository, in.PR)
	return eligible(p, in.Head) && p.Base.SHA == in.Base, err
}

func prepare(ctx context.Context, dir string) error {
	repo := os.Getenv("REVIEW_REPOSITORY")
	number, err := strconv.Atoi(os.Getenv("REVIEW_PR"))
	if err != nil {
		return errors.New("invalid REVIEW_PR")
	}
	p, err := getPull(ctx, repo, number)
	if err != nil {
		return err
	}
	if !eligible(p, os.Getenv("REVIEW_HEAD")) {
		return summary("AI review disabled or superseded; requires an open, non-draft PR labeled ai-review.\n")
	}
	diff, err := github(ctx, fmt.Sprintf("repos/%s/compare/%s...%s", repo, p.Base.SHA, p.Head.SHA), "application/vnd.github.diff", maxDiff)
	if err != nil {
		return err
	}
	if len(diff) == 0 {
		return summary("AI review skipped: empty diff.\n")
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "pr.diff"), diff, 0o600); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "input.json"), input{repo, number, p.Base.SHA, p.Head.SHA, digest(diff)}); err != nil {
		return err
	}
	if path := os.Getenv("GITHUB_OUTPUT"); path != "" {
		return appendFile(path, "eligible=true\n")
	}
	return nil
}

func github(ctx context.Context, path, accept string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/"+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv("GH_TOKEN"))
	req.Header.Set("Accept", accept)
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("GitHub request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub request returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, errors.New("reading GitHub response failed")
	}
	if int64(len(data)) > limit {
		return nil, errors.New("GitHub response exceeds review size limit; refusing partial review")
	}
	return data, nil
}

func executeReview(ctx context.Context, dir string, diff []byte) (r review, runErr error) {
	for _, name := range []string{"VERTEX_AI_PROJECT_ID", "GOOGLE_VERTEX_AI_TOKEN"} {
		if os.Getenv(name) == "" {
			return r, fmt.Errorf("%s is required", name)
		}
	}
	var id [6]byte
	if _, err := rand.Read(id[:]); err != nil {
		return r, err
	}
	workspace := "rev-" + hex.EncodeToString(id[:])
	gateway := os.Getenv("OPENSHELL_GATEWAY")
	if gateway == "" {
		gateway = "openshell"
	}
	region := os.Getenv("VERTEX_AI_REGION")
	if region == "" {
		region = "global"
	}
	base := []string{"--gateway", gateway, "--workspace", workspace}
	providerCreated := false
	if err := command(ctx, nil, nil, "openshell", "workspace", "create", "--gateway", gateway, "--name", workspace); err != nil {
		return r, err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		runErr = errors.Join(runErr, command(cleanupCtx, nil, nil, "./harness", append([]string{"delete"}, append(base, "--sandboxes")...)...))
		if providerCreated {
			runErr = errors.Join(runErr, command(cleanupCtx, nil, nil, "openshell", append([]string{"provider", "delete"}, append(base, "vertex-review")...)...))
		}
		runErr = errors.Join(runErr, command(cleanupCtx, nil, nil, "openshell", "workspace", "delete", "--gateway", gateway, workspace))
	}()
	args := append([]string{"provider", "create"}, base...)
	args = append(args, "--name", "vertex-review", "--type", "google-vertex-ai", "--from-existing", "--config", "VERTEX_AI_PROJECT_ID="+os.Getenv("VERTEX_AI_PROJECT_ID"), "--config", "VERTEX_AI_REGION="+region)
	if err := command(ctx, nil, nil, "openshell", args...); err != nil {
		return r, err
	}
	providerCreated = true
	args = append([]string{"inference", "set"}, base...)
	if err := command(ctx, nil, nil, "openshell", append(args, "--provider", "vertex-review", "--model", "gemini-3.8-flash")...); err != nil {
		return r, err
	}
	out, err := os.OpenFile(filepath.Join(dir, "agent.ndjson"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return r, err
	}
	defer out.Close()
	args = append([]string{"apply", "-f", "examples/github-pr-reviewer/opencode-harness.yaml", "--result-file", filepath.Join(dir, "execution.json")}, base...)
	absDiff, err := filepath.Abs(filepath.Join(dir, "pr.diff"))
	if err != nil {
		return r, err
	}
	if err := command(ctx, out, []string{"REVIEW_DIFF=" + absDiff}, "./harness", args...); err != nil {
		return r, err
	}
	if err := out.Close(); err != nil {
		return r, err
	}
	data, err := os.Open(filepath.Join(dir, "agent.ndjson"))
	if err != nil {
		return r, err
	}
	defer data.Close()
	output, err := io.ReadAll(io.LimitReader(data, 1024*1024+1))
	if err != nil || len(output) > 1024*1024 {
		return r, errors.New("unreadable or oversized worker output")
	}
	return parseReview(output, diff)
}

func command(ctx context.Context, stdout io.Writer, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	// Only trusted tools run here. Never echo worker-controlled output into logs.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = 35 * time.Second
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s step failed: %w", filepath.Base(name), err)
	}
	return nil
}

func parseReview(data, diff []byte) (review, error) {
	var text strings.Builder
	complete := false
	scan := bufio.NewScanner(bytes.NewReader(data))
	scan.Buffer(make([]byte, 4096), 1024*1024)
	for scan.Scan() {
		var event struct {
			Type string
			Part struct{ Text, Reason string }
		}
		if json.Unmarshal(scan.Bytes(), &event) != nil {
			continue
		} // harness status lines
		if event.Type == "error" || event.Type == "tool_use" {
			return review{}, errors.New("worker failed or attempted a tool call")
		}
		if event.Type == "step_finish" && event.Part.Reason != "stop" {
			return review{}, errors.New("worker did not complete its response")
		}
		if event.Type == "step_finish" {
			complete = true
		}
		if event.Type == "text" {
			text.WriteString(event.Part.Text)
		}
	}
	if scan.Err() != nil || !complete {
		return review{}, errors.New("incomplete or oversized worker output")
	}
	var r review
	decoder := json.NewDecoder(strings.NewReader(text.String()))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&r) != nil || r.Findings == nil || len(r.Findings) > 3 {
		return r, errors.New("invalid findings contract")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return r, errors.New("trailing review output")
	}
	for _, f := range r.Findings {
		if (f.Severity != "high" && f.Severity != "medium" && f.Severity != "low") || len(f.Message) == 0 || len(f.Message) > 2000 || !inHunk(diff, f.File, f.Line) {
			return r, errors.New("invalid finding location or content")
		}
	}
	return r, nil
}

func inHunk(diff []byte, file string, line int) bool {
	if file == "" || len(file) > 300 || line < 1 || strings.ContainsAny(file, "\r\n") {
		return false
	}
	current := ""
	header := false
	for _, s := range strings.Split(string(diff), "\n") {
		if strings.HasPrefix(s, "diff --git ") {
			current = ""
			header = true
		}
		if header && strings.HasPrefix(s, "+++ b/") {
			current = strings.TrimPrefix(s, "+++ b/")
		}
		m := hunkPattern.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		header = false
		if current != file {
			continue
		}
		start, _ := strconv.Atoi(m[1])
		count := 1
		if m[2] != "" {
			count, _ = strconv.Atoi(m[2])
		}
		if line >= start && line-start < count {
			return true
		}
	}
	return false
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
func appendFile(path, text string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := io.WriteString(f, text)
	return errors.Join(writeErr, f.Close())
}
func summary(text string) error {
	if dir := os.Getenv("REVIEW_DIR"); dir != "" {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			if err := os.WriteFile(filepath.Join(dir, "summary.md"), []byte(text), 0o600); err != nil {
				return err
			}
		}
	}
	if path := os.Getenv("GITHUB_STEP_SUMMARY"); path != "" {
		return appendFile(path, text)
	}
	fmt.Print(text)
	return nil
}
