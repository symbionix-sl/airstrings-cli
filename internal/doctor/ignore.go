package doctor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/symbionix-sl/airstrings-cli/internal/workspace"
)

const (
	ignoreFile    = "doctor.json"
	ignoredDetail = "ignored (.airstrings/doctor.json)"
)

type ignoreConfig struct {
	Ignored []string `json:"ignored"`
}

func ignorePath(root string) string {
	return filepath.Join(root, workspace.DirName, ignoreFile)
}

func loadIgnoreList(root string) []string {
	data, err := os.ReadFile(ignorePath(root))
	if err != nil {
		return nil
	}
	var cfg ignoreConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	return cfg.Ignored
}

func appendIgnores(root string, keys []string) error {
	list := loadIgnoreList(root)
	have := make(map[string]bool, len(list))
	for _, k := range list {
		have[k] = true
	}
	for _, k := range keys {
		if !have[k] {
			list = append(list, k)
			have[k] = true
		}
	}
	data, err := json.MarshalIndent(ignoreConfig{Ignored: list}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal doctor ignores: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(ignorePath(root)), 0700); err != nil {
		return fmt.Errorf("create workspace dir: %w", err)
	}
	if err := os.WriteFile(ignorePath(root), data, 0600); err != nil {
		return fmt.Errorf("write doctor ignores: %w", err)
	}
	return nil
}

func applyIgnores(rep *Report, root string) {
	list := loadIgnoreList(root)
	if len(list) == 0 {
		return
	}
	ign := make(map[string]bool, len(list))
	for _, k := range list {
		ign[k] = true
	}
	for i := range rep.Checks {
		c := &rep.Checks[i]
		if ign[c.key(root)] {
			c.Status = StatusIgnored
			c.Detail = ignoredDetail
			c.Fix = ""
		}
	}
}

func (c *Check) key(root string) string {
	if c.Path == "" {
		return c.Name
	}
	return c.Name + ":" + relTo(root, c.Path)
}

func relTo(root, path string) string {
	r, err := filepath.Rel(root, path)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(os.PathSeparator)) {
		return path
	}
	return filepath.ToSlash(r)
}

func PromptIgnores(rep *Report, root string, in io.Reader, out io.Writer) (bool, error) {
	reader := bufio.NewReader(in)
	var added []string
	for i := range rep.Checks {
		c := &rep.Checks[i]
		if c.Status != StatusMissing {
			continue
		}
		label := c.Name
		if c.Path != "" {
			label += " " + relTo(root, c.Path)
		}
		fmt.Fprintf(out, "\n✗ %s\nIgnore this check in future runs? [y/N/q] ", label)
		line, err := reader.ReadString('\n')
		ans := strings.ToLower(strings.TrimSpace(line))
		if ans == "q" {
			break
		}
		if ans == "y" {
			added = append(added, c.key(root))
			c.Status = StatusIgnored
			c.Detail = ignoredDetail
			c.Fix = ""
		}
		if err != nil {
			break
		}
	}
	if len(added) == 0 {
		return false, nil
	}
	if err := appendIgnores(root, added); err != nil {
		return false, err
	}
	return true, nil
}
