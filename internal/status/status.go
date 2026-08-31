package status

import (
	"fmt"
	"os"
	"strings"
)

var Verbose bool
var ShowCommands bool

func Cmd(name string, args ...string) {
	if !Verbose && !ShowCommands {
		return
	}
	line := formatCmdLine(name, args)
	if ShowCommands {
		fmt.Printf("  %s\n", line)
	}
	if Verbose {
		fmt.Fprintf(os.Stderr, "  %s\n", line)
	}
}

func formatCmdLine(name string, args []string) string {
	var b strings.Builder
	b.WriteString("$ ")
	b.WriteString(name)
	redactNext := false
	redactNextEnvIfSensitive := false
	for _, a := range args {
		b.WriteByte(' ')
		if redactNext {
			b.WriteString(redactValue(a))
			redactNext = false
			continue
		}
		if redactNextEnvIfSensitive {
			// --env values carry secrets on the acknowledged-plaintext path
			// (the gateway also warns the agent can read them). Mask the value
			// when the key looks sensitive; keep KEY visible either way, and
			// leave benign env (e.g. ANTHROPIC_BASE_URL) readable for debugging.
			if isSensitiveLiteral(a) {
				b.WriteString(redactValue(a))
			} else {
				b.WriteString(a)
			}
			redactNextEnvIfSensitive = false
			continue
		}
		if a == "--credential" || a == "--material" || a == "--secret-material-key" {
			redactNext = true
			b.WriteString(a)
			continue
		}
		if a == "--env" {
			redactNextEnvIfSensitive = true
			b.WriteString(a)
			continue
		}
		if strings.HasPrefix(a, "--from-literal=") && isSensitiveLiteral(a) {
			b.WriteString(redactFromLiteral(a))
			continue
		}
		b.WriteString(a)
	}
	return b.String()
}

// redactValue replaces the value portion of KEY=VALUE with ***.
func redactValue(s string) string {
	if i := strings.IndexByte(s, '='); i >= 0 {
		return s[:i+1] + "***"
	}
	return s
}

// isSensitiveLiteral checks if a --from-literal=KEY=VALUE arg contains a secret key.
func isSensitiveLiteral(s string) bool {
	upper := strings.ToUpper(s)
	for _, keyword := range []string{"TOKEN", "SECRET", "PASSWORD", "KEY", "CREDENTIAL"} {
		if strings.Contains(upper, keyword) {
			return true
		}
	}
	return false
}

// redactFromLiteral redacts the value in --from-literal=KEY=VALUE.
func redactFromLiteral(s string) string {
	// s is "--from-literal=KEY=VALUE", find the second '='
	prefix := "--from-literal="
	rest := s[len(prefix):]
	if i := strings.IndexByte(rest, '='); i >= 0 {
		return prefix + rest[:i+1] + "***"
	}
	return s
}

func OK(msg string)                  { fmt.Println("  ✓ " + msg) }
func OKf(format string, a ...any)    { fmt.Printf("  ✓ "+format+"\n", a...) }
func Fail(msg string)                { fmt.Println("  ✗ " + msg) }
func Failf(format string, a ...any)  { fmt.Printf("  ✗ "+format+"\n", a...) }
func Warn(msg string)                { fmt.Println("  ! " + msg) }
func Warnf(format string, a ...any)  { fmt.Printf("  ! "+format+"\n", a...) }
func Info(msg string)                { fmt.Println("  - " + msg) }
func Infof(format string, a ...any)  { fmt.Printf("  - "+format+"\n", a...) }
func Section(title string)           { fmt.Printf("\n=== %s ===\n", title) }
func Done(msg string) {
	fmt.Println()
	fmt.Println(msg)
}

func Header(title string) {
	fmt.Printf("\n%s\n", title)
	fmt.Println(strings.Repeat("─", len(title)))
}
