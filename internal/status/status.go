package status

import "fmt"

func OK(msg string)                  { fmt.Println("  ✓ " + msg) }
func OKf(format string, a ...any)    { fmt.Printf("  ✓ "+format+"\n", a...) }
func Fail(msg string)                { fmt.Println("  ✗ " + msg) }
func Failf(format string, a ...any)  { fmt.Printf("  ✗ "+format+"\n", a...) }
func Warn(msg string)                { fmt.Println("  ! " + msg) }
func Warnf(format string, a ...any)  { fmt.Printf("  ! "+format+"\n", a...) }
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
