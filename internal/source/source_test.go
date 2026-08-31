package source

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

func TestCanonicalizeURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://github.com/org/repo.git", "https://github.com/org/repo"},
		{"https://github.com/org/repo/", "https://github.com/org/repo"},
		{"https://GitHub.com/org/repo", "https://github.com/org/repo"},
		{"HTTPS://github.com/org/repo", "https://github.com/org/repo"},
		{"  https://github.com/org/repo.git  ", "https://github.com/org/repo"},
		{"https://user:secret@github.com/org/repo.git", "https://github.com/org/repo"},
		{"git@github.com:org/repo.git", "git@github.com:org/repo"},
	}
	for _, c := range cases {
		if got := CanonicalizeURL(c.in); got != c.want {
			t.Errorf("CanonicalizeURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCredentialsShareMirrorKey(t *testing.T) {
	c := NewCache(t.TempDir())
	first := c.MirrorPath("https://alice:first-token@github.com/org/repo.git")
	second := c.MirrorPath("https://bob:rotated-token@github.com/org/repo.git")
	if first != second {
		t.Fatalf("credential variants mapped to different mirrors: %q != %q", first, second)
	}
}

func TestEnsureOriginDoesNotPersistUserinfo(t *testing.T) {
	for _, existingOrigin := range []bool{false, true} {
		t.Run(map[bool]string{false: "add", true: "update"}[existingOrigin], func(t *testing.T) {
			parent := t.TempDir()
			mirror := filepath.Join(parent, "mirror.git")
			runGit(t, parent, "init", "--bare", "--quiet", mirror)
			if existingOrigin {
				runGit(t, mirror, "remote", "add", "origin", "https://alice:first-token@github.com/org/repo.git")
			}
			if err := ensureOrigin(mirror, "https://bob:rotated-token@github.com/org/repo.git"); err != nil {
				t.Fatalf("ensureOrigin: %v", err)
			}
			// Read the stored config value directly. `git remote get-url`
			// applies the caller's global insteadOf rules and may report a
			// rewritten transport.
			got, err := gitOutput(mirror, "config", "--get", "remote.origin.url")
			if err != nil {
				t.Fatalf("get origin URL: %v", err)
			}
			if want := "https://github.com/org/repo.git"; got != want {
				t.Fatalf("origin URL = %q, want %q", got, want)
			}
		})
	}
}

func TestCanonicalizeURLVariantsShareKey(t *testing.T) {
	// All spellings of the same repo must canonicalize identically so they hash
	// to one mirror.
	variants := []string{
		"https://github.com/org/repo",
		"https://github.com/org/repo.git",
		"https://github.com/org/repo/",
		"https://GitHub.com/org/repo.git",
	}
	first := CanonicalizeURL(variants[0])
	for _, v := range variants[1:] {
		if got := CanonicalizeURL(v); got != first {
			t.Errorf("CanonicalizeURL(%q) = %q, want %q", v, got, first)
		}
	}
}

func TestMirrorPathBasenameCollision(t *testing.T) {
	// Two different repos that share a basename ("repo") must map to different
	// mirrors — the whole point of URL-hashing instead of basename-keying.
	c := NewCache(t.TempDir())
	a := c.MirrorPath("https://github.com/org-a/repo.git")
	b := c.MirrorPath("https://github.com/org-b/repo.git")
	if a == b {
		t.Fatalf("same-basename repos collided on one mirror: %s", a)
	}
	// And URL variants of the SAME repo must map to one mirror.
	if c.MirrorPath("https://github.com/org-a/repo") != a {
		t.Errorf("URL variant did not reuse the mirror for org-a/repo")
	}
}

func TestRepoName(t *testing.T) {
	cases := map[string]string{
		"https://github.com/org/repo.git": "repo",
		"https://github.com/org/repo/":    "repo",
		"git@github.com:org/tools.git":    "tools",
	}
	for in, want := range cases {
		if got := RepoName(in); got != want {
			t.Errorf("RepoName(%q) = %q, want %q", in, got, want)
		}
	}
}

// makeRemote creates a local non-bare git repo with one commit on `main` and a
// file, then returns a file:// URL with the given basename so tests can exercise
// real git without a network.
func makeRemote(t *testing.T, basename, fileContent string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), basename)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "init", "--quiet", "-b", "main")
	runGit(t, dir, "config", "user.email", "t@example.com")
	runGit(t, dir, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte(fileContent), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "--quiet", "-m", "init")
	return "file://" + dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestPrepareCheckoutContent(t *testing.T) {
	remote := makeRemote(t, "repo", "hello")
	c := NewCache(t.TempDir())

	p, err := c.Prepare(remote, "main", "run1")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	// The checkout basename must be the repo name (so it uploads to
	// /sandbox/<repo-name>), not the run id.
	if got := filepath.Base(p.Dir); got != "repo" {
		t.Errorf("checkout basename = %q, want %q", got, "repo")
	}
	got, err := os.ReadFile(filepath.Join(p.Dir, "marker.txt"))
	if err != nil {
		t.Fatalf("reading checked-out file: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("marker.txt = %q, want %q", got, "hello")
	}

	if err := p.Cleanup(); err != nil {
		t.Errorf("Cleanup: %v", err)
	}
	if _, err := os.Stat(p.Dir); !os.IsNotExist(err) {
		t.Errorf("worktree still present after cleanup: %v", err)
	}
}

func TestPrepareCheckoutSelfContained(t *testing.T) {
	// The uploaded checkout must be a real repo whose git works after the mirror
	// (and the whole host cache) is gone — inside the sandbox only this dir
	// exists. A linked worktree would leave a `.git` *file* pointing at a host
	// path that breaks there; this pins that it is a real `.git` directory with
	// its own objects.
	remote := makeRemote(t, "repo", "portable")
	cacheRoot := t.TempDir()
	c := NewCache(cacheRoot)

	p, err := c.Prepare(remote, "main", "run1")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	info, err := os.Stat(filepath.Join(p.Dir, ".git"))
	if err != nil || !info.IsDir() {
		t.Fatalf(".git must be a real directory, got isDir=%v err=%v", info.IsDir(), err)
	}

	// Copy the checkout elsewhere, then delete the entire cache (mirror + run
	// dirs) to simulate uploading only the checkout into a fresh sandbox.
	uploaded := filepath.Join(t.TempDir(), "repo")
	if out, err := exec.Command("cp", "-R", p.Dir, uploaded).CombinedOutput(); err != nil {
		t.Fatalf("copy checkout: %v\n%s", err, out)
	}
	if err := os.RemoveAll(cacheRoot); err != nil {
		t.Fatalf("removing cache: %v", err)
	}

	// git must still work against the copied checkout with no mirror present.
	if out, err := exec.Command("git", "-C", uploaded, "status", "--porcelain").CombinedOutput(); err != nil {
		t.Fatalf("git status in uploaded checkout: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", uploaded, "log", "--oneline", "-1").CombinedOutput(); err != nil {
		t.Fatalf("git log in uploaded checkout: %v\n%s", err, out)
	}
}

func TestPrepareDefaultRef(t *testing.T) {
	// ref "" resolves the remote default branch.
	remote := makeRemote(t, "repo", "default")
	c := NewCache(t.TempDir())
	p, err := c.Prepare(remote, "", "run1")
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	defer p.Cleanup()
	got, _ := os.ReadFile(filepath.Join(p.Dir, "marker.txt"))
	if string(got) != "default" {
		t.Errorf("marker.txt = %q, want %q", got, "default")
	}
}

func TestPrepareConcurrentSameRepo(t *testing.T) {
	// Two simultaneous runs of the same repo must get independent worktrees with
	// correct content and no error — the mirror lock serializes the shared fetch.
	remote := makeRemote(t, "repo", "shared")
	c := NewCache(t.TempDir())

	const n = 6
	var wg sync.WaitGroup
	results := make([]Prepared, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			runID, _ := NewRunID()
			results[i], errs[i] = c.Prepare(remote, "main", runID)
		}(i)
	}
	wg.Wait()

	seen := map[string]bool{}
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("run %d Prepare: %v", i, errs[i])
		}
		if seen[results[i].Dir] {
			t.Fatalf("run %d shared a checkout path: %s", i, results[i].Dir)
		}
		seen[results[i].Dir] = true
		got, err := os.ReadFile(filepath.Join(results[i].Dir, "marker.txt"))
		if err != nil || string(got) != "shared" {
			t.Errorf("run %d content = %q (err %v), want %q", i, got, err, "shared")
		}
	}
	for i := 0; i < n; i++ {
		if err := results[i].Cleanup(); err != nil {
			t.Errorf("run %d Cleanup: %v", i, err)
		}
	}
}

func TestPrepareBadRefNoLeak(t *testing.T) {
	// A failed prepare (here: a ref that does not exist) must not leave an
	// orphaned checkout dir behind — network/ref failures are expected in
	// production and would otherwise pile up under checkouts/.
	remote := makeRemote(t, "repo", "x")
	root := t.TempDir()
	c := NewCache(root)

	if _, err := c.Prepare(remote, "no-such-branch", "run1"); err == nil {
		t.Fatal("Prepare succeeded on a nonexistent ref, want error")
	}
	runDir := filepath.Join(root, "checkouts", "run1")
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Errorf("checkout run dir leaked after failed prepare: %v", err)
	}
}

func TestPrepareTwoBasenameReposNoCollision(t *testing.T) {
	// Two distinct repos sharing basename "repo" prepared in the same cache must
	// yield the content of their own remote, proving the mirrors are separate.
	c := NewCache(t.TempDir())
	remoteA := makeRemote(t, "repo", "content-A")
	remoteB := makeRemote(t, "repo", "content-B")

	pa, err := c.Prepare(remoteA, "main", "runA")
	if err != nil {
		t.Fatal(err)
	}
	defer pa.Cleanup()
	pb, err := c.Prepare(remoteB, "main", "runB")
	if err != nil {
		t.Fatal(err)
	}
	defer pb.Cleanup()

	ga, _ := os.ReadFile(filepath.Join(pa.Dir, "marker.txt"))
	gb, _ := os.ReadFile(filepath.Join(pb.Dir, "marker.txt"))
	if string(ga) != "content-A" {
		t.Errorf("repo A content = %q, want content-A", ga)
	}
	if string(gb) != "content-B" {
		t.Errorf("repo B content = %q, want content-B", gb)
	}
}
