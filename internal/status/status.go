package status

import "fmt"

func OK(msg string)                  { fmt.Println("  ✓ " + msg) }
func OKf(format string, a ...any)    { fmt.Printf("  ✓ "+format+"\n", a...) }
func Fail(msg string)                { fmt.Println("  ✗ " + msg) }
func Failf(format string, a ...any)  { fmt.Printf("  ✗ "+format+"\n", a...) }
func Info(msg string)                { fmt.Println("  - " + msg) }
func Detail(msg string)              { fmt.Println("    " + msg) }
func Sub(msg string)                 { fmt.Println("      " + msg) }
func Section(title string)           { fmt.Printf("\n=== %s ===\n", title) }
func Summary(ok bool) {
	if ok {
		fmt.Println("✓ Ready to launch")
	} else {
		fmt.Println("✗ Not ready — fix issues above")
	}
}
