package output

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestCheck_NoColorWhenNotTTY(t *testing.T) {
	if Check != "✓" {
		t.Errorf("expected plain ✓ when stdout is not a TTY, got %q", Check)
	}
}

func TestTable_HeaderAlignsWithRows(t *testing.T) {
	r, w, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = w
	Table([]string{"ID", "NAME"}, [][]string{{"proj_0123456789", "x"}})
	w.Close()
	os.Stdout = stdout
	out, _ := io.ReadAll(r)
	lines := strings.Split(string(out), "\n")
	if strings.Index(lines[0], "NAME") != strings.Index(lines[2], "x") {
		t.Errorf("misaligned:\n%s", out)
	}
}
