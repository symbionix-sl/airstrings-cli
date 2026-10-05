package guide

import (
	"regexp"
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

func TestJSSnippetsCarryRequiredConfigFields(t *testing.T) {
	required := []string{"organizationId", "projectId", "environmentId", "publicKeys", "locale"}
	field := regexp.MustCompile(`(?m)^  (\w+):`)
	sn := Snippets("org_x", "proj_x", "env_x", []string{"K"})
	for _, p := range []string{"web", "react_native"} {
		var got []string
		for _, m := range field.FindAllStringSubmatch(sn[p], -1) {
			got = append(got, m[1])
		}
		if strings.Join(got, ",") != strings.Join(required, ",") {
			t.Errorf("%s snippet fields = %v, want %v", p, got, required)
		}
	}
}
