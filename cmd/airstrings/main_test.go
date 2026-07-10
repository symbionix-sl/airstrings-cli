package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "airstrings-cli-test")
	if err != nil {
		panic(err)
	}
	binPath = filepath.Join(dir, "airstrings")
	out, err := exec.Command("go", "build", "-o", binPath, ".").CombinedOutput()
	if err != nil {
		os.RemoveAll(dir)
		panic("build failed: " + err.Error() + "\n" + string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func scrubbedEnv() []string {
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "AIRSTRINGS_") {
			continue
		}
		env = append(env, e)
	}
	return env
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Dir = t.TempDir()
	cmd.Env = scrubbedEnv()
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run %v: %v", args, err)
		}
		code = exitErr.ExitCode()
	}
	return code, stdout.String(), stderr.String()
}

func TestCLISubprocess(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantExit   int
		wantStdout string
		wantStderr string
	}{
		{"push help", []string{"push", "--help"}, 0, "Usage: airstrings push [--section <name>]", ""},
		{"publish help", []string{"publish", "--help"}, 0, "Usage: airstrings publish [locale...]", ""},
		{"pull help", []string{"pull", "--help"}, 0, "Usage: airstrings pull [--section <name>]", ""},
		{"apikey help", []string{"apikey", "--help"}, 0, "Usage: airstrings apikey rotate [--env <name>]", ""},
		{"status help", []string{"status", "--help"}, 0, "Usage: airstrings status", ""},
		{"strings help", []string{"strings", "--help"}, 0, "Usage: airstrings strings <ls|get|set|rm>", ""},
		{"init help", []string{"init", "--help"}, 0, "Usage: airstrings init <api-key> [--url <base-url>] [--purge]", ""},
		{"env help", []string{"env", "--help"}, 0, "Usage: airstrings env [use|add|rm|create]", ""},
		{"push short help", []string{"push", "-h"}, 0, "Usage: airstrings push [--section <name>]", ""},
		{"status unknown flag", []string{"status", "--totallyfake"}, 2, "", "unknown flag: --totallyfake"},
		{"push unknown flag", []string{"push", "--totallyfake"}, 2, "", "unknown flag: --totallyfake"},
		{"publish unknown flag", []string{"publish", "--totallyfake"}, 2, "", "unknown flag: --totallyfake"},
		{"pull unknown flag", []string{"pull", "--totallyfake"}, 2, "", "unknown flag: --totallyfake"},
		{"strings ls unknown flag", []string{"strings", "ls", "--totallyfake"}, 2, "", "unknown flag: --totallyfake"},
		{"strings set unknown flag", []string{"strings", "set", "k", "en=v", "--format", "text", "--bogus"}, 2, "", "unknown flag: --bogus"},
		{"top-level help flag", []string{"--help"}, 0, "Usage: airstrings <command> [options]", ""},
		{"help command", []string{"help"}, 0, "Usage: airstrings <command> [options]", ""},
		{"version", []string{"version"}, 0, "airstrings", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runCLI(t, tc.args...)
			if code != tc.wantExit {
				t.Fatalf("exit code = %d, want %d\nstdout: %s\nstderr: %s", code, tc.wantExit, stdout, stderr)
			}
			if tc.wantStdout != "" && !strings.Contains(stdout, tc.wantStdout) {
				t.Errorf("stdout missing %q\nstdout: %s", tc.wantStdout, stdout)
			}
			if tc.wantStderr != "" && !strings.Contains(stderr, tc.wantStderr) {
				t.Errorf("stderr missing %q\nstderr: %s", tc.wantStderr, stderr)
			}
		})
	}
}
