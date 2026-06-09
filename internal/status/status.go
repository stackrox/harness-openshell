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

// ShowEquivalentCmd displays the equivalent openshell CLI command for
// an operation, regardless of how it was actually executed. Use this
// in gRPC Gateway implementations to show the CLI equivalent.
func ShowEquivalentCmd(name string, args ...string) {
	if !ShowCommands {
		return
	}
	fmt.Printf("  %s\n", formatCmdLine(name, args))
}

func formatCmdLine(name string, args []string) string {
	var b strings.Builder
	b.WriteString("$ ")
	b.WriteString(name)
	redactNext := false
	for _, a := range args {
		b.WriteByte(' ')
		if redactNext {
			b.WriteString(redactValue(a))
			redactNext = false
			continue
		}
		if a == "--credential" || a == "--material" || a == "--secret-material-key" {
			redactNext = true
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
func Info(msg string)                { fmt.Println("  - " + msg) }
func Infof(format string, a ...any)  { fmt.Printf("  - "+format+"\n", a...) }
func Detail(msg string)              { fmt.Println("    " + msg) }
func Detailf(format string, a ...any){ fmt.Printf("    "+format+"\n", a...) }
func Sub(msg string)                 { fmt.Println("      " + msg) }
func Step(n int, msg string)         { fmt.Printf("\n=== Step %d: %s ===\n", n, msg) }
func Section(title string)           { fmt.Printf("\n=== %s ===\n", title) }
func Summary(ok bool) {
	if ok {
		fmt.Println("✓ Ready to launch")
	} else {
		fmt.Println("✗ Not ready — fix issues above")
	}
}
func Done(msg string) {
	fmt.Println()
	fmt.Println(msg)
}

func Header(title string) {
	fmt.Printf("\n%s\n", title)
	fmt.Println(strings.Repeat("─", len(title)))
}

func Table(headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	for i, h := range headers {
		if i > 0 {
			fmt.Print("  ")
		}
		fmt.Printf("%-*s", widths[i], h)
	}
	fmt.Println()
	for i := range headers {
		if i > 0 {
			fmt.Print("  ")
		}
		fmt.Print(strings.Repeat("─", widths[i]))
	}
	fmt.Println()
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				fmt.Print("  ")
			}
			if i < len(widths) {
				fmt.Printf("%-*s", widths[i], cell)
			}
		}
		fmt.Println()
	}
}
