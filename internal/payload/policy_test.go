package payload

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteEffectivePolicy_NonEmpty(t *testing.T) {
	dir := t.TempDir()
	policy := []byte("kind: policy\napiVersion: v1\n")

	filePath, err := WriteEffectivePolicy(dir, policy)

	if err != nil {
		t.Fatalf("WriteEffectivePolicy failed: %v", err)
	}

	if filePath == "" {
		t.Fatal("expected non-empty filepath, got empty string")
	}

	// Verify the file exists at the returned path
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("file at returned path does not exist: %v", err)
	}

	// Verify the file is within the given dir
	rel, err := filepath.Rel(dir, filePath)
	if err != nil {
		t.Fatalf("could not compute relative path: %v", err)
	}
	if filepath.IsAbs(rel) {
		t.Fatal("returned path is not within dir")
	}

	// Verify the exact content
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(content) != string(policy) {
		t.Fatalf("file content mismatch: got %q, want %q", string(content), string(policy))
	}
}

func TestWriteEffectivePolicy_Nil(t *testing.T) {
	dir := t.TempDir()

	filePath, err := WriteEffectivePolicy(dir, nil)

	if err != nil {
		t.Fatalf("WriteEffectivePolicy failed: %v", err)
	}

	if filePath != "" {
		t.Fatalf("expected empty filepath, got %q", filePath)
	}

	// Verify no file was created
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no files created, but found %d entries", len(entries))
	}
}

func TestWriteEffectivePolicy_Empty(t *testing.T) {
	dir := t.TempDir()

	filePath, err := WriteEffectivePolicy(dir, []byte{})

	if err != nil {
		t.Fatalf("WriteEffectivePolicy failed: %v", err)
	}

	if filePath != "" {
		t.Fatalf("expected empty filepath, got %q", filePath)
	}

	// Verify no file was created
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no files created, but found %d entries", len(entries))
	}
}

func TestWriteEffectivePolicy_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	policy := []byte("test policy content")

	filePath, err := WriteEffectivePolicy(dir, policy)

	if err != nil {
		t.Fatalf("WriteEffectivePolicy failed: %v", err)
	}

	// Verify file is readable
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	// Check that the file has user read permission (affected by umask)
	if (info.Mode().Perm() & 0o400) == 0 {
		t.Fatalf("expected file to be readable by user, got mode %o", info.Mode().Perm())
	}
}
