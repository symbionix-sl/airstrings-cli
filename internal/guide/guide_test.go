package guide

import (
	"strings"
	"testing"
)

func TestShellArg(t *testing.T) {
	cases := map[string]string{
		"production":   "production",
		"qa-1":         "qa-1",
		"prod; rm -rf": "'prod; rm -rf'",
		"$(id)":        "'$(id)'",
		"it's":         `'it'\''s'`,
		"a\x1b[31mb":   "'a[31mb'",
	}
	for in, want := range cases {
		if got := ShellArg(in); got != want {
			t.Errorf("ShellArg(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStatusHintQuotesEnvName(t *testing.T) {
	h := Status("staging", false, "prod $(id)", false, true, "", "")
	if !strings.Contains(h.NextStep, "`airstrings env use 'prod $(id)'`") {
		t.Errorf("unquoted env name in hint: %q", h.NextStep)
	}
	h = ProtectedNoKey("prod\x1b[2J", "")
	if strings.Contains(h.Message+h.NextStep, "\x1b") {
		t.Errorf("control characters in hint: %q", h.Message+h.NextStep)
	}
}
