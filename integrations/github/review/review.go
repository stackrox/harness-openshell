// Package review adapts GitHub PRs to a trusted sandbox task. It owns no gateway
// administration or credential provisioning; Execute supplies the shared runner.
package review

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const MaxDiffBytes = 256 * 1024
const MaxOutputBytes = 2 * 1024 * 1024

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var providerPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// Options contain references and review inputs, never provider credentials.
type Options struct {
	Dir, Repository, Head                      string
	PR                                         int
	AllowDrafts                                bool
	Workflow, Skill, SkillRoot, PolicyTemplate string
	Gateway, Workspace, GitHubProvider         string
}

type Task struct {
	File, Name, Gateway, Workspace, ResultFile, OutputDir string
	Variables                                             map[string]string
}

type Executor func(context.Context, Task, io.Writer, io.Writer) error

type Service struct {
	// API is a bounded, read-only GitHub GET. Nil uses the authenticated gh CLI.
	API     func(context.Context, string, string, int) ([]byte, error)
	Execute Executor
}

type input struct {
	Repository string `json:"repository"`
	PR         int    `json:"pr"`
	Head       string `json:"head"`
	Base       string `json:"base"`
	DiffSHA256 string `json:"diffSha256"`
}

type pullRequest struct {
	State  string `json:"state"`
	Draft  bool   `json:"draft"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Head struct {
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		SHA string `json:"sha"`
	} `json:"base"`
}

func (o Options) validate() error {
	if !filepath.IsAbs(o.Dir) || !repositoryPattern.MatchString(o.Repository) || o.PR < 1 {
		return errors.New("review requires an absolute directory, owner/repository, and positive PR number")
	}
	if o.Head != "" && !shaPattern.MatchString(o.Head) {
		return errors.New("expected head must be a 40-character commit SHA")
	}
	return nil
}

func (s Service) current(ctx context.Context, o Options, head, base string) (pullRequest, bool, error) {
	api := s.API
	if api == nil {
		api = GitHubGET
	}
	data, err := api(ctx, fmt.Sprintf("repos/%s/pulls/%d", o.Repository, o.PR), "application/vnd.github+json", 1024*1024)
	if err != nil {
		return pullRequest{}, false, err
	}
	var pr pullRequest
	if err := json.Unmarshal(data, &pr); err != nil {
		return pr, false, fmt.Errorf("invalid PR metadata: %w", err)
	}
	eligible := pr.State == "open" && (o.AllowDrafts || !pr.Draft) && (head == "" || head == pr.Head.SHA) && (base == "" || base == pr.Base.SHA)
	labeled := false
	for _, label := range pr.Labels {
		labeled = labeled || label.Name == "ai-review"
	}
	return pr, eligible && labeled, nil
}

// Prepare refuses an existing directory and fetches the exact base...head diff.
// A false result is a successful skip; only true should enable CI setup.
func (s Service) Prepare(ctx context.Context, o Options) (eligible bool, retErr error) {
	if err := o.validate(); err != nil {
		return false, err
	}
	if err := os.Mkdir(o.Dir, 0o700); err != nil {
		return false, err
	}
	state := "skipped or superseded"
	defer func() {
		if retErr != nil {
			state = "failed"
		}
		retErr = errors.Join(retErr, writeSummary(o, o.Head, state))
	}()
	pr, eligible, err := s.current(ctx, o, o.Head, "")
	if err != nil || !eligible {
		return false, err
	}
	if !shaPattern.MatchString(pr.Head.SHA) || !shaPattern.MatchString(pr.Base.SHA) {
		return false, errors.New("invalid PR revision")
	}
	api := s.API
	if api == nil {
		api = GitHubGET
	}
	diff, err := api(ctx, fmt.Sprintf("repos/%s/compare/%s...%s", o.Repository, pr.Base.SHA, pr.Head.SHA), "application/vnd.github.diff", MaxDiffBytes)
	if err != nil {
		return false, err
	}
	if len(diff) == 0 || len(diff) > MaxDiffBytes {
		return false, errors.New("review diff is empty or exceeds 256 KiB")
	}
	if err := writeNew(filepath.Join(o.Dir, "pr.diff"), diff); err != nil {
		return false, err
	}
	digest := sha256.Sum256(diff)
	data, err := json.MarshalIndent(input{o.Repository, o.PR, pr.Head.SHA, pr.Base.SHA, hex.EncodeToString(digest[:])}, "", "  ")
	if err != nil {
		return false, err
	}
	if err := writeNew(filepath.Join(o.Dir, "input.json"), append(data, '\n')); err != nil {
		return false, err
	}
	if err := appendEnvironmentFile("GITHUB_OUTPUT", "eligible=true\n"); err != nil {
		return false, err
	}
	o.Head, state = pr.Head.SHA, "prepared"
	return true, nil
}

// Run checks the prepared input before calling Execute once. The runner owns
// sandbox deletion on success, failure, and cancellation. It must return errors
// for execution or cleanup failures, regardless of what the agent printed.
func (s Service) Run(ctx context.Context, o Options) (retErr error) {
	if err := o.validate(); err != nil {
		return err
	}
	info, err := os.Lstat(o.Dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("review directory must be a real directory")
	}
	state, head := "failed", o.Head
	defer func() { retErr = errors.Join(retErr, writeSummary(o, head, state)) }()
	data, err := readBounded(filepath.Join(o.Dir, "input.json"), 4096)
	if err != nil {
		return err
	}
	var in input
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	if in.Repository != o.Repository || in.PR != o.PR || !shaPattern.MatchString(in.Head) || !shaPattern.MatchString(in.Base) || (o.Head != "" && o.Head != in.Head) {
		return errors.New("prepared input does not match this review")
	}
	head = in.Head
	diff, err := readBounded(filepath.Join(o.Dir, "pr.diff"), MaxDiffBytes)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(diff)
	if len(diff) == 0 || hex.EncodeToString(digest[:]) != in.DiffSHA256 {
		return errors.New("prepared diff checksum mismatch")
	}
	_, eligible, err := s.current(ctx, o, in.Head, in.Base)
	if err != nil {
		return err
	}
	if !eligible {
		state = "skipped or superseded"
		return nil
	}
	if s.Execute == nil {
		return errors.New("review executor is required")
	}
	if !providerPattern.MatchString(o.GitHubProvider) {
		return errors.New("invalid GitHub provider reference")
	}
	skill, err := selectSkill(o.Skill, o.SkillRoot)
	if err != nil {
		return err
	}
	policy, err := readBounded(o.PolicyTemplate, MaxOutputBytes)
	if err != nil {
		return err
	}
	policy = []byte(strings.NewReplacer("${REVIEW_REPOSITORY}", in.Repository, "${REVIEW_PR}", strconv.Itoa(in.PR), "${REVIEW_GITHUB_PROVIDER}", o.GitHubProvider).Replace(string(policy)))
	if strings.Contains(string(policy), "${") {
		return errors.New("unresolved variable in review policy")
	}
	policyPath := filepath.Join(o.Dir, "review-policy.yaml")
	if err := writeNew(policyPath, policy); err != nil {
		return err
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return err
	}
	name := "review-" + hex.EncodeToString(id)
	if err := writeNew(filepath.Join(o.Dir, "sandbox-name.txt"), []byte(name+"\n")); err != nil {
		return err
	}
	stdout, err := os.OpenFile(filepath.Join(o.Dir, "agent.ndjson"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	stderr, err := os.OpenFile(filepath.Join(o.Dir, "agent.stderr"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return errors.Join(err, stdout.Close())
	}
	runCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	out := &boundedWriter{writer: stdout, remaining: MaxOutputBytes, cancel: cancel}
	diagnostics := &boundedWriter{writer: stderr, remaining: MaxOutputBytes, cancel: cancel}
	err = s.Execute(runCtx, Task{
		File: o.Workflow, Name: name, Gateway: o.Gateway, Workspace: o.Workspace,
		ResultFile: filepath.Join(o.Dir, "execution.json"), OutputDir: filepath.Join(o.Dir, "outputs"),
		Variables: map[string]string{
			"REVIEW_REPOSITORY": in.Repository, "REVIEW_PR": strconv.Itoa(in.PR), "REVIEW_HEAD": in.Head,
			"REVIEW_DIFF": filepath.Join(o.Dir, "pr.diff"), "REVIEW_SKILL": skill,
			"REVIEW_POLICY": policyPath, "REVIEW_GITHUB_PROVIDER": o.GitHubProvider,
		},
	}, out, diagnostics)
	err = errors.Join(err, runCtx.Err(), out.err, diagnostics.err, stdout.Close(), stderr.Close())
	if err != nil {
		return err
	}
	text, err := ValidateOutput(filepath.Join(o.Dir, "agent.ndjson"))
	if err != nil {
		return err
	}
	_, eligible, err = s.current(ctx, o, in.Head, in.Base)
	if err != nil {
		return err
	}
	if !eligible {
		state = "skipped or superseded"
		return nil
	}
	if err := writeNew(filepath.Join(o.Dir, "review.txt"), []byte(text)); err != nil {
		return err
	}
	state = "completed"
	return nil
}

func selectSkill(path, root string) (string, error) {
	if root != "" {
		if !filepath.IsLocal(path) {
			return "", errors.New("caller skill must be relative to its trusted checkout")
		}
		resolvedRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			return "", err
		}
		resolvedPath, err := filepath.EvalSymlinks(filepath.Join(resolvedRoot, path))
		if err != nil {
			return "", err
		}
		rel, err := filepath.Rel(resolvedRoot, resolvedPath)
		if err != nil || !filepath.IsLocal(rel) {
			return "", errors.New("caller skill escapes its trusted checkout")
		}
		path = resolvedPath
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("review skill must be a regular file")
	}
	return path, nil
}

func writeNew(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	return errors.Join(err, file.Close())
}

func readBounded(path string, limit int) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("review input must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	err = errors.Join(err, file.Close())
	if len(data) > limit {
		return nil, errors.New("review input exceeds size limit")
	}
	return data, err
}

func appendEnvironmentFile(name, content string) error {
	path := os.Getenv(name)
	if path == "" {
		return nil
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	_, err = io.WriteString(file, content)
	return errors.Join(err, file.Close())
}

func writeSummary(o Options, head, state string) error {
	text := fmt.Sprintf("## AI review: %s\n\nHead: `%s`\n\nThe sandbox may post inline comments during execution. Cleanup does not remove comments. Artifacts are diagnostics, not approval of findings.\n", state, head)
	// The summary contains only validated host metadata, never model output.
	if err := os.WriteFile(filepath.Join(o.Dir, "summary.md"), []byte(text), 0o600); err != nil {
		return err
	}
	if state != "prepared" {
		return appendEnvironmentFile("GITHUB_STEP_SUMMARY", text)
	}
	return nil
}
