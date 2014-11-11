package log

import (
	"os"

	"github.com/fatih/color"
)

func init() {
	color.Output = os.Stderr
}

var (
	// Verbose enables verbose printing.
	Verbose = false
)

var (
	normal  = color.New()
	verbose = color.New(color.FgCyan)
	warn    = color.New(color.FgYellow, color.Bold)
	fatal   = color.New(color.FgRed, color.Bold)
)

// Printf prints normal information.
func Printf(format string, args ...interface{}) {
	printWith(normal, format, args...)
}

// Verbosef prints verbose information.
func Verbosef(format string, args ...interface{}) {
	if Verbose {
		printWith(verbose, format, args...)
	}
}

// Warnf prints warning information.
func Warnf(format string, args ...interface{}) {
	printWith(warn, format, args...)
}

// Fatalf prints error information, and terminates execution.
func Fatalf(format string, args ...interface{}) {
	printWith(fatal, format, args...)
	os.Exit(1)
}

func printWith(c *color.Color, format string, args ...interface{}) {
	c.Printf(format+"\n", args...)
}
