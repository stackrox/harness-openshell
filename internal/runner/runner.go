package runner

import (
	"io"
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

// RunCLI executes the openshell CLI with stdout/stderr passthrough.
func RunCLI(cli string, args ...string) error {
	cmd := exec.Command(cli, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunCLISilent executes the openshell CLI with discarded output.
func RunCLISilent(cli string, args ...string) error {
	cmd := exec.Command(cli, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

// RunCLIOutput executes the openshell CLI and returns stdout.
func RunCLIOutput(cli string, args ...string) ([]byte, error) {
	cmd := exec.Command(cli, args...)
	cmd.Stderr = io.Discard
	return cmd.Output()
}

// Exec replaces the current process with the given command (unix exec).
func Exec(name string, args ...string) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return err
	}
	return execSyscall(path, append([]string{name}, args...), os.Environ())
}
