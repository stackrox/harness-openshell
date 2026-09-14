package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareCommitProvenance(t *testing.T) {
	remote := makeRemote(t, "repo", "original")
	dir := strings.TrimPrefix(remote, "file://")
	original, err := gitOutput(dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "tag", "-a", "snapshot", "-m", "annotated tag")
	cache := NewCache(t.TempDir())
	first, err := cache.Prepare(remote, "main", "first")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Cleanup() })
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("updated"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "commit", "-am", "advance main", "--quiet")
	latest, err := gitOutput(dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, ref, commit, content string
	}{
		{"branch", "main", latest, "updated"},
		{"default", "", latest, "updated"},
		{"pinned", original, original, "original"},
		{"annotated-tag", "snapshot", original, "original"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prepared, err := cache.Prepare(remote, tc.ref, tc.name)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := prepared.Cleanup(); err != nil {
					t.Error(err)
				}
			}()
			if prepared.Commit != tc.commit {
				t.Fatalf("commit = %q, want %q", prepared.Commit, tc.commit)
			}
			content, err := os.ReadFile(filepath.Join(prepared.Dir, "marker.txt"))
			if err != nil || string(content) != tc.content {
				t.Fatalf("checkout content = %q, error = %v", content, err)
			}
		})
	}
	if first.Commit != original {
		t.Fatalf("earlier checkout provenance changed: %q", first.Commit)
	}
	for _, ref := range []string{"missing-ref", strings.Repeat("a", 40)} {
		if _, err := cache.Prepare(remote, ref, "missing"); err == nil {
			t.Fatalf("missing ref %q must fail, not fall back to default HEAD", ref)
		}
		if _, err := os.Stat(cache.runDir("missing")); !os.IsNotExist(err) {
			t.Fatalf("failed preparation left checkout state: %v", err)
		}
	}
}
