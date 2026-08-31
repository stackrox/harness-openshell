// Package source manages the on-disk cache of git repositories cloned outside
// the sandbox for upload.
//
// It replaces the old basename-keyed cache (~/.cache/harness-openshell/repos/
// <repo-name>/) — which collided when two repos shared a basename and raced when
// two runs shared one repo — with URL-hashed bare mirrors plus per-run,
// self-contained checkouts:
//
//	~/.cache/harness-openshell/
//	  mirrors/<sha256(canonical-url)>.git   bare, shallow, updated in place, shared
//	  checkouts/<run-id>/<repo-name>/       real repo (own .git), per run, removed after run
//
// The mirror is the only shared state; every write to it is serialized under a
// per-mirror file lock. Checkouts are per-run, never shared, and hold their own
// objects (no alternates into the mirror) so git keeps working after only the
// checkout dir is uploaded into the sandbox.
package source

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Cache locates the on-disk cache roots. The zero value is unusable; construct
// with DefaultCache or NewCache.
type Cache struct {
	root string // ~/.cache/harness-openshell
}

// DefaultCache resolves the cache under the user's home directory.
func DefaultCache() (*Cache, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("determining home dir: %w", err)
	}
	return NewCache(filepath.Join(home, ".cache", "harness-openshell")), nil
}

// NewCache builds a cache rooted at the given directory (used by tests).
func NewCache(root string) *Cache { return &Cache{root: root} }

// CanonicalizeURL normalizes a repo URL into a stable key for "same repo".
// It trims surrounding space, removes URL userinfo, strips a trailing slash and
// a ".git" suffix, and lowercases the scheme and host only — repository paths
// stay case-sensitive because many hosts treat them so. Non-URL inputs (e.g.
// scp-style git@host:org/repo) are returned trimmed of the same suffixes without
// further change, which is still stable per distinct spelling.
func CanonicalizeURL(raw string) string {
	s := stripURLUserinfo(strings.TrimSpace(raw))
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		u.Scheme = strings.ToLower(u.Scheme)
		u.Host = strings.ToLower(u.Host)
		s = u.String()
	}
	s = strings.TrimRight(s, "/")
	s = strings.TrimSuffix(s, ".git")
	return s
}

// stripURLUserinfo removes embedded credentials from a URL before it is used
// as a cache identity or persisted in git configuration. Authentication is
// resolved by git's configured credential helper instead.
func stripURLUserinfo(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	u.User = nil
	return u.String()
}

// RepoName derives the directory basename a repo is uploaded under
// (/sandbox/<repo-name>), matching the old cache's behavior.
func RepoName(repoURL string) string {
	return strings.TrimSuffix(path.Base(strings.TrimRight(strings.TrimSpace(repoURL), "/")), ".git")
}

// MirrorPath is the bare-mirror directory for a repo URL, keyed by the sha256 of
// its canonical form so distinct repos with the same basename never collide.
func (c *Cache) MirrorPath(repoURL string) string {
	sum := sha256.Sum256([]byte(CanonicalizeURL(repoURL)))
	return filepath.Join(c.root, "mirrors", hex.EncodeToString(sum[:])+".git")
}

// runDir is the per-run checkout parent (checkouts/<run-id>), removed wholesale
// on cleanup.
func (c *Cache) runDir(runID string) string {
	return filepath.Join(c.root, "checkouts", runID)
}

// checkoutPath nests the checkout as checkouts/<run-id>/<repo-name> so its
// basename stays <repo-name>: `openshell --upload` copies the source dir by
// name, so this is what makes the tree land at /sandbox/<repo-name> rather than
// /sandbox/<run-id>.
func (c *Cache) checkoutPath(runID, repoName string) string {
	return filepath.Join(c.runDir(runID), repoName)
}

// NewRunID returns a random hex id identifying one run's checkout. 128 bits so
// concurrent runs never collide on a checkout path (a collision would let one
// run's cleanup delete another's tree).
func NewRunID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generating run id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
