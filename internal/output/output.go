package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

var JSONMode bool

// Exit codes for distinct failure classes, so scripts and agents can branch
// without parsing messages.
const (
	ExitGeneric     = 1
	ExitUsage       = 2
	ExitAuth        = 3
	ExitNotFound    = 4
	ExitNetwork     = 5
	ExitRateLimited = 6
)

var useColor = colorEnabled()

// Check is the success marker, colorized only when stdout is an interactive
// terminal and NO_COLOR is unset.
var Check = checkMark()

func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func checkMark() string {
	if useColor {
		return "\x1b[32m✓\x1b[0m"
	}
	return "✓"
}

// JSON prints v as indented JSON.
func JSON(v any) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

// Table prints rows with aligned columns.
func Table(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	fmt.Fprintln(w, strings.Repeat("─", len(strings.Join(headers, "  "))))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
}

// Auto picks JSON or table based on --json flag.
func Auto(v any, headers []string, rows [][]string) {
	if JSONMode {
		JSON(v)
	} else {
		Table(headers, rows)
	}
}

// Success prints a success message. In JSON mode it is suppressed so callers
// can emit a structured result instead.
func Success(msg string) {
	fmt.Printf("%s %s\n", Check, msg)
}

// Fail prints to stderr and exits with the given code.
func Fail(code int, format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(code)
}

// Errorf prints to stderr and exits with the generic error code.
func Errorf(format string, args ...any) {
	Fail(ExitGeneric, format, args...)
}

// Warnf prints a non-fatal warning to stderr.
func Warnf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Warning: "+format+"\n", args...)
}
