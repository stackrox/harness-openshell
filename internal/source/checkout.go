package source

import (
	"fmt"
	"os"
)

// Prepared is the result of Prepare: an isolated, self-contained per-run
// checkout ready to upload, plus a Cleanup that removes it.
type Prepared struct {
	// Dir is the absolute path to the per-run checkout. Its basename is the repo
	// name, so uploading it lands the tree at /sandbox/<repo-name>. The checkout
	// has a real .git directory holding its own objects (no link back to the
	// shared mirror), so git still works inside the sandbox after upload.
	Dir string
	// Cleanup removes the per-run checkout directory. Safe to call once; errors
	// are returned for the caller to log, never fatal.
	Cleanup func() error
}

// Prepare updates the shared bare mirror for repoURL and builds a self-contained
// per-run checkout at ref (branch, tag, or the remote default when ref is ""),
// with submodules initialized.
//
// The mirror update and the local object copy into the checkout run under an
// exclusive per-mirror lock so concurrent runs of the same repo cannot corrupt
// the shared object store or race a shallow gc. Once the objects are copied the
// checkout is fully independent of the mirror, so the checkout and submodule init
// run outside the lock — unrelated runs are not serialized on network submodule
// fetches.
//
// The clone happens on the host; git credentials never enter the sandbox — only
// the returned checkout contents are uploaded. The checkout is a real repository
// (self-contained .git), so the agent can run git inside the sandbox.
func (c *Cache) Prepare(repoURL, ref, runID string) (Prepared, error) {
	dir := c.checkoutPath(runID, RepoName(repoURL))

	if err := c.fetchIntoCheckout(repoURL, ref, dir); err != nil {
		// The checkout dir may have been created before the failing step (a bad
		// ref or a network blip on fetch is expected); don't leak it.
		_ = os.RemoveAll(c.runDir(runID))
		return Prepared{}, err
	}

	// checkout + submodules run outside the mirror lock: the checkout already
	// holds every object it needs, so it no longer touches the shared mirror.
	if err := git(dir, "checkout", "--detach", "--quiet", "FETCH_HEAD"); err != nil {
		_ = os.RemoveAll(c.runDir(runID))
		return Prepared{}, err
	}
	if err := git(dir, "submodule", "update", "--init", "--depth", "1"); err != nil {
		_ = os.RemoveAll(c.runDir(runID))
		return Prepared{}, err
	}

	cleanup := func() error { return os.RemoveAll(c.runDir(runID)) }
	return Prepared{Dir: dir, Cleanup: cleanup}, nil
}

// fetchIntoCheckout, under the per-mirror lock, updates the shared mirror for
// repoURL at ref and copies the resolved commit's objects into a fresh
// repository at dir, leaving the commit as the checkout's FETCH_HEAD. Holding the
// lock across both the mirror update and the local object copy keeps a concurrent
// run's shallow gc from deleting packs mid-read.
func (c *Cache) fetchIntoCheckout(repoURL, ref, dir string) error {
	mirror := c.MirrorPath(repoURL)

	lock, err := lockMirror(mirror)
	if err != nil {
		return err
	}
	defer lock.unlock()

	if err := ensureMirror(mirror, repoURL); err != nil {
		return err
	}
	commit, err := fetchRef(mirror, ref)
	if err != nil {
		return err
	}

	// Fresh, isolated checkout dir (clear any stale remnant from a reused run id
	// or a crashed prior run).
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("clearing checkout dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating checkout dir: %w", err)
	}
	if err := git(dir, "init", "--quiet"); err != nil {
		return err
	}
	// Copy just the resolved commit's objects from the local mirror into the
	// checkout's own object store. No alternates are configured, so the checkout
	// stays valid after the mirror (or the whole host cache) is gone — which is
	// what makes git usable inside the sandbox once only this dir is uploaded.
	if err := git(dir, "fetch", "--depth", "1", mirror, commit); err != nil {
		return err
	}
	return nil
}
