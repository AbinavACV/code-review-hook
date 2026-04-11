package main

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

var useColor = term.IsTerminal(int(os.Stderr.Fd()))

func color(code string) string {
	if useColor {
		return code
	}
	return ""
}

// PrintInfo prints an informational message to stderr.
func PrintInfo(msg string) {
	fmt.Fprintf(os.Stderr, "%s\n", msg)
}

// PrintWarning prints a yellow warning message to stderr.
func PrintWarning(msg string) {
	fmt.Fprintf(os.Stderr, "%s%sWarning:%s %s\n", color("\033[1m"), color("\033[33m"), color("\033[0m"), msg)
}

// PrintError prints a red error message to stderr.
func PrintError(msg string) {
	fmt.Fprintf(os.Stderr, "%s%sError:%s %s\n", color("\033[1m"), color("\033[31m"), color("\033[0m"), msg)
}

// PrintSuccess prints a green success message to stderr.
func PrintSuccess(msg string) {
	fmt.Fprintf(os.Stderr, "%s%s✔%s %s\n", color("\033[1m"), color("\033[32m"), color("\033[0m"), msg)
}

// PrintSection prints a titled section to stderr.
func PrintSection(title, body string) {
	fmt.Fprintf(os.Stderr, "\n%s%s%s\n%s\n", color("\033[1m"), title, color("\033[0m"), body)
}
