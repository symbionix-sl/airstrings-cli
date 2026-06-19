package output

import "testing"

func TestCheck_NoColorWhenNotTTY(t *testing.T) {
	if Check != "✓" {
		t.Errorf("expected plain ✓ when stdout is not a TTY, got %q", Check)
	}
}
