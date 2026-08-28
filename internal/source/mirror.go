package source

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// git runs a git command, streaming its output to stderr (so clone/fetch
// progress is visible), and wraps failures with the arguments for context.
func git(dir string, args ...string) error {
	full := args
	if dir != "" {
		full = append([]string{"-C", dir}, args...)
	}
	cmd := exec.Command("git", full...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %v: %w", args, err)
	}
	return nil
}

// gitOutput runs a git command and returns its trimmed stdout (stderr still
// streams for diagnostics).
func gitOutput(dir string, args ...string) (string, error) {
	full := args
	if dir != "" {
		full = append([]string{"-C", dir}, args...)
	}
	cmd := exec.Command("git", full...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %v: %w", args, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// mirrorLock is an exclusive advisory lock held over a mirror's fetch+worktree
// window. Concurrent runs of the same repo contend only here — the one piece of
// shared on-disk state — so serializing it keeps a fetch's shallow gc from
// racing a peer's worktree registration.
type mirrorLock struct{ f *os.File }

// lockMirror acquires an exclusive flock on a sidecar lock file for the mirror.
// The lock file lives beside the mirror dir (<mirror>.lock) so it exists before
// the mirror itself does.
//
// The lock file is intentionally never unlinked: deleting it on unlock would
// reintroduce the classic flock unlink race (a peer that already opened the file
// holds a lock on a now-orphaned inode while a new process creates a fresh file
// and locks that instead, so both believe they hold the lock). There is exactly
// one 0-byte lock file per distinct repo URL, so they do not accumulate per run.
func lockMirror(mirrorPath string) (*mirrorLock, error) {
	if err := os.MkdirAll(filepath.Dir(mirrorPath), 0o755); err != nil {
		return nil, fmt.Errorf("creating mirrors dir: %w", err)
	}
	f, err := os.OpenFile(mirrorPath+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening mirror lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("locking mirror: %w", err)
	}
	return &mirrorLock{f: f}, nil
}

func (l *mirrorLock) unlock() {
	if l == nil || l.f == nil {
		return
	}
	syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	l.f.Close()
}

// ensureMirror creates the bare mirror on first use and (re)points its origin
// remote at repoURL. A bare repo with a configured origin lets us shallow-fetch
// arbitrary refs on demand rather than mirroring every ref. It is fully
// idempotent — safe to re-run after a crash that left the bare repo created but
// the remote unconfigured (the origin step no longer sits behind the isGitDir
// early return). Callers hold the mirror lock, so this never races a peer.
func ensureMirror(mirrorPath, repoURL string) error {
	if !isGitDir(mirrorPath) {
		if err := os.MkdirAll(mirrorPath, 0o755); err != nil {
			return fmt.Errorf("creating mirror dir: %w", err)
		}
		if err := git(mirrorPath, "init", "--bare", "--quiet"); err != nil {
			return err
		}
	}
	return ensureOrigin(mirrorPath, repoURL)
}

// ensureOrigin idempotently points the mirror's origin remote at repoURL: it
// adds the remote when absent and updates the URL otherwise. Listing remotes
// first keeps the common first-clone path quiet (probing with `remote get-url`
// on a missing remote would print an error to stderr).
func ensureOrigin(mirrorPath, repoURL string) error {
	remotes, err := gitOutput(mirrorPath, "remote")
	if err != nil {
		return err
	}
	for _, r := range strings.Split(remotes, "\n") {
		if strings.TrimSpace(r) == "origin" {
			return git(mirrorPath, "remote", "set-url", "origin", repoURL)
		}
	}
	return git(mirrorPath, "remote", "add", "origin", repoURL)
}

// fetchRef shallow-fetches the requested ref (or the remote default HEAD when
// ref is empty) into the mirror and resolves it to a concrete commit. The commit
// is what the per-run worktree is created from.
func fetchRef(mirrorPath, ref string) (commit string, err error) {
	target := ref
	if target == "" {
		target = "HEAD"
	}
	if err := git(mirrorPath, "fetch", "--depth", "1", "origin", target); err != nil {
		return "", err
	}
	commit, err = gitOutput(mirrorPath, "rev-parse", "FETCH_HEAD")
	if err != nil {
		return "", err
	}
	return commit, nil
}

func isGitDir(dir string) bool {
	// A bare repo has HEAD at its root; a non-bare has .git/HEAD. The mirror is
	// bare, so check for HEAD directly.
	_, err := os.Stat(filepath.Join(dir, "HEAD"))
	return err == nil
}
