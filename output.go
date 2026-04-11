package main

import (
	"fmt"
	"os"
)

const (
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBold   = "\033[1m"
	colorReset  = "\033[0m"
)

// PrintInfo prints an informational message to stderr.
func PrintInfo(msg string) {
	fmt.Fprintf(os.Stderr, "%s\n", msg)
}

// PrintWarning prints a yellow warning message to stderr.
func PrintWarning(msg string) {
	fmt.Fprintf(os.Stderr, "%s%sWarning:%s %s\n", colorBold, colorYellow, colorReset, msg)
}

// PrintError prints a red error message to stderr.
func PrintError(msg string) {
	fmt.Fprintf(os.Stderr, "%s%sError:%s %s\n", colorBold, colorRed, colorReset, msg)
}

// PrintSuccess prints a green success message to stderr.
func PrintSuccess(msg string) {
	fmt.Fprintf(os.Stderr, "%s%s✔%s %s\n", colorBold, colorGreen, colorReset, msg)
}

// PrintSection prints a titled section to stderr.
func PrintSection(title, body string) {
	fmt.Fprintf(os.Stderr, "\n%s%s%s\n%s\n", colorBold, title, colorReset, body)
}
