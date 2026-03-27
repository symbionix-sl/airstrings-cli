package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

var JSONMode bool

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

// Success prints a green success message.
func Success(msg string) {
	fmt.Printf("✓ %s\n", msg)
}

// Errorf prints to stderr and exits.
func Errorf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
