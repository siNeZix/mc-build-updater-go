// Package console выводит короткие, единообразные сообщения CLI.
package console

import (
	"fmt"
	"io"
	"os"
)

const (
	reset  = "\x1b[0m"
	red    = "\x1b[31m"
	green  = "\x1b[32m"
	yellow = "\x1b[33m"
	cyan   = "\x1b[36m"
)

var colorEnabled = interactive(os.Stdout) && os.Getenv("NO_COLOR") == ""

func Info(format string, arguments ...any)    { write(os.Stdout, cyan, "i", format, arguments...) }
func Success(format string, arguments ...any) { write(os.Stdout, green, "✓", format, arguments...) }
func Warning(format string, arguments ...any) { write(os.Stderr, yellow, "!", format, arguments...) }
func Error(format string, arguments ...any)   { write(os.Stderr, red, "✗", format, arguments...) }

func interactive(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func write(destination io.Writer, color, mark, format string, arguments ...any) {
	prefix := mark + " "
	if colorEnabled {
		prefix = color + prefix + reset
	}
	_, _ = fmt.Fprintf(destination, prefix+format+"\n", arguments...)
}
