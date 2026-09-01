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
	var line strings.Builder
	line.WriteString("$ ")
	line.WriteString(name)
	redactNext := false
	redactEnv := false
	for _, arg := range args {
		line.WriteByte(' ')
		switch {
		case redactNext:
			line.WriteString(redactValue(arg))
			redactNext = false
		case redactEnv:
			if isSensitive(arg) {
				line.WriteString(redactValue(arg))
			} else {
				line.WriteString(arg)
			}
			redactEnv = false
		case arg == "--credential" || arg == "--material" || arg == "--secret-material-key":
			line.WriteString(arg)
			redactNext = true
		case arg == "--env":
			line.WriteString(arg)
			redactEnv = true
		case strings.HasPrefix(arg, "--from-literal=") && isSensitive(arg):
			line.WriteString(redactFromLiteral(arg))
		default:
			line.WriteString(arg)
		}
	}
	return line.String()
}

func redactValue(value string) string {
	if index := strings.IndexByte(value, '='); index >= 0 {
		return value[:index+1] + "***"
	}
	return value
}

func isSensitive(value string) bool {
	upper := strings.ToUpper(value)
	for _, keyword := range []string{"TOKEN", "SECRET", "PASSWORD", "KEY", "CREDENTIAL"} {
		if strings.Contains(upper, keyword) {
			return true
		}
	}
	return false
}

func redactFromLiteral(value string) string {
	const prefix = "--from-literal="
	rest := value[len(prefix):]
	if index := strings.IndexByte(rest, '='); index >= 0 {
		return prefix + rest[:index+1] + "***"
	}
	return value
}

func OK(msg string)                 { fmt.Println("  ✓ " + msg) }
func OKf(format string, a ...any)   { fmt.Printf("  ✓ "+format+"\n", a...) }
func Fail(msg string)               { fmt.Println("  ✗ " + msg) }
func Failf(format string, a ...any) { fmt.Printf("  ✗ "+format+"\n", a...) }
func Warnf(format string, a ...any) { fmt.Printf("  ! "+format+"\n", a...) }
func Info(msg string)               { fmt.Println("  - " + msg) }
func Infof(format string, a ...any) { fmt.Printf("  - "+format+"\n", a...) }
func Section(title string)          { fmt.Printf("\n=== %s ===\n", title) }
func Done(msg string) {
	fmt.Println()
	fmt.Println(msg)
}

func Header(title string) {
	fmt.Printf("\n%s\n", title)
	fmt.Println(strings.Repeat("─", len(title)))
}
