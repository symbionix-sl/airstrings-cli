package guide

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
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
	sn := Snippets("org_x", "proj_x", "env_x", []string{"K"}, "")
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

func TestGuideNoKeyPointsAtProjectKeys(t *testing.T) {
	url := APIKeysURL("https://app.x", "proj_1")
	if url != "https://app.x/projects/proj_1/api-keys" {
		t.Errorf("APIKeysURL = %q", url)
	}
	for _, h := range []Hint{OpenNoKey("production", url), NeedStagingKey("production", "staging", url, "https://app.x/promote")} {
		if strings.Contains(h.NextStep, "env add") || !strings.Contains(h.NextStep, "airstrings init <key>") || !strings.Contains(h.NextStep, "project key") {
			t.Errorf("hint next step = %q", h.NextStep)
		}
	}
}

func TestSnippetsProdUnchanged(t *testing.T) {
	want, err := os.ReadFile("testdata/snippets_prod.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, base := range []string{"", client.DefaultBaseURL} {
		got, _ := json.MarshalIndent(Snippets("org_x", "proj_x", "env_x", []string{"K1", "K2"}, base), "", "  ")
		if string(got) != string(want) {
			t.Errorf("base %q: prod snippets changed:\n%s", base, got)
		}
	}
}

func TestSnippetsCarryNonDefaultAPIBase(t *testing.T) {
	sn := Snippets("org_x", "proj_x", "env_x", []string{"K"}, "https://api-staging.airstrings.com")
	want := map[string]string{
		"web":          "  apiBaseURL: 'https://api-staging.airstrings.com',\n",
		"react_native": "  apiBaseURL: 'https://api-staging.airstrings.com',\n",
		"ios":          `    apiBaseURL: URL(string: "https://api-staging.airstrings.com")!` + "\n",
		"android":      "    apiBaseURL = \"https://api-staging.airstrings.com\",\n",
		"go":           "\tAPIBaseURL:     \"https://api-staging.airstrings.com\",\n",
	}
	for p, line := range want {
		if !strings.Contains(sn[p], line) {
			t.Errorf("%s snippet missing %q:\n%s", p, line, sn[p])
		}
	}
}
