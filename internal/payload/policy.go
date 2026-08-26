package payload

import (
	"os"
	"path/filepath"
)

// WriteEffectivePolicy writes policy YAML bytes into dir and returns the file
// path, or "" if policy is empty/nil (caller then omits --policy). dir is a
// caller-owned temp dir (caller creates and removes it). Returns an error only
// on a real write failure.
func WriteEffectivePolicy(dir string, policy []byte) (string, error) {
	// If policy is nil or empty, return empty string with no error
	if len(policy) == 0 {
		return "", nil
	}

	// Write policy bytes to policy.yaml in the given directory
	filePath := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(filePath, policy, 0o644); err != nil {
		return "", err
	}

	return filePath, nil
}
