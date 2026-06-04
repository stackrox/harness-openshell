package runner

import (
	"os"
	"os/exec"
	"path/filepath"
)

// RunScript executes a bash script from bin/scripts/ with stdout/stderr passthrough.
func RunScript(harnessDir, scriptName string, args ...string) error {
	path := filepath.Join(harnessDir, "bin", "scripts", scriptName)
	cmd := exec.Command("bash", append([]string{path}, args...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = harnessDir
	return cmd.Run()
}
