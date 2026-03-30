package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/symbionix/airstrings-cli/internal/workspace"
)

var toolDefs = []ToolDef{
	{
		Name:        "airstrings_init",
		Description: "Initialize an AirStrings workspace in the current directory. Creates .airstrings/ folder. You must run airstrings login afterwards to authenticate.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"dir": {Type: "string", Description: "Directory to initialize. Uses current working directory if omitted."},
			},
		},
	},
	{
		Name:        "airstrings_local_set",
		Description: "Add or update a string in the local workspace CSV. Writes to .airstrings/<section>/<section>.csv or .airstrings/strings.csv. Does not call the API — changes are local until pushed.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"key":     {Type: "string", Description: "The string key (e.g., 'onboarding.welcome')"},
				"values":  {Type: "string", Description: "JSON object of locale=value pairs, e.g. {\"en\": \"Hello\", \"it\": \"Ciao\"}"},
				"format":  {Type: "string", Description: "String format: 'text' (default) or 'icu'"},
				"section": {Type: "string", Description: "Section name. If omitted, string goes to flat strings.csv"},
			},
			Required: []string{"key", "values"},
		},
	},
	{
		Name:        "airstrings_local_rm",
		Description: "Remove a string from the local workspace CSV. Does not call the API — changes are local until pushed.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"key":     {Type: "string", Description: "The string key to remove"},
				"locale":  {Type: "string", Description: "Remove only this locale. If omitted, removes all locales for the key."},
				"section": {Type: "string", Description: "Section to remove from. If omitted, removes from flat strings.csv"},
			},
			Required: []string{"key"},
		},
	},
	{
		Name:        "airstrings_local_ls",
		Description: "List all strings in the local workspace. Returns strings from local CSV files, not from the API.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"section": {Type: "string", Description: "Filter to a specific section. If omitted, lists all sections."},
			},
		},
	},
	{
		Name:        "airstrings_push",
		Description: "Push local workspace strings to the AirStrings API. Upserts each key with its locale values. Creates sections remotely if they don't exist.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"section": {Type: "string", Description: "Push only this section. If omitted, pushes all."},
			},
		},
	},
	{
		Name:        "airstrings_pull",
		Description: "Pull strings from the AirStrings API into the local workspace. Overwrites local CSVs with remote state.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"section": {Type: "string", Description: "Pull only this section. If omitted, pulls all."},
			},
		},
	},
	{
		Name:        "airstrings_publish",
		Description: "Publish bundles to the CDN. Bundles are signed and delivered to SDKs.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"locales": {Type: "string", Description: "Comma-separated locales to publish. If omitted, publishes all."},
			},
		},
	},
}

type toolHandler func(args json.RawMessage) *CallToolResult

var toolHandlers = map[string]toolHandler{
	"airstrings_init":      handleToolInit,
	"airstrings_local_set": handleToolLocalSet,
	"airstrings_local_rm":  handleToolLocalRm,
	"airstrings_local_ls":  handleToolLocalLs,
	"airstrings_push":      handleToolPush,
	"airstrings_pull":      handleToolPull,
	"airstrings_publish":   handleToolPublish,
}

func handleToolInit(raw json.RawMessage) *CallToolResult {
	var args struct {
		Dir string `json:"dir"`
	}
	json.Unmarshal(raw, &args)

	dir := args.Dir
	if dir == "" {
		dir, _ = os.Getwd()
	}

	// Check if workspace already exists
	wsDir := filepath.Join(dir, ".airstrings")
	if _, err := os.Stat(filepath.Join(wsDir, "config.json")); err == nil {
		return errorResult(fmt.Sprintf("workspace already exists at %s", wsDir))
	}

	wsCfg := workspace.WorkspaceConfig{}
	if err := workspace.Init(dir, wsCfg); err != nil {
		return errorResult(fmt.Sprintf("init workspace: %s", err))
	}

	result, _ := json.Marshal(map[string]any{
		"workspace": wsDir,
		"message":   "Workspace initialized. Run: airstrings login <api-key>",
	})
	return textResult(string(result))
}

func handleToolLocalSet(raw json.RawMessage) *CallToolResult {
	var args struct {
		Key     string `json:"key"`
		Values  string `json:"values"`
		Format  string `json:"format"`
		Section string `json:"section"`
	}
	json.Unmarshal(raw, &args)

	if args.Key == "" {
		return errorResult("key is required")
	}

	var values map[string]string
	if err := json.Unmarshal([]byte(args.Values), &values); err != nil {
		return errorResult(fmt.Sprintf("invalid values JSON: %s", err))
	}

	if len(values) == 0 {
		return errorResult("at least one locale=value pair is required")
	}

	format := args.Format
	if format == "" {
		format = "text"
	}

	wsDir, err := workspace.Find()
	if err != nil {
		return errorResult(err.Error())
	}

	if args.Section != "" {
		if err := workspace.ValidateSectionName(args.Section); err != nil {
			return errorResult(err.Error())
		}
	}

	path := workspace.CSVPath(wsDir, args.Section)
	if err := workspace.SetRows(path, args.Key, values, format); err != nil {
		return errorResult(fmt.Sprintf("set rows: %s", err))
	}

	result, _ := json.Marshal(map[string]any{
		"key":     args.Key,
		"locales": len(values),
		"section": args.Section,
		"format":  format,
	})
	return textResult(string(result))
}

func handleToolLocalRm(raw json.RawMessage) *CallToolResult {
	var args struct {
		Key     string `json:"key"`
		Locale  string `json:"locale"`
		Section string `json:"section"`
	}
	json.Unmarshal(raw, &args)

	if args.Key == "" {
		return errorResult("key is required")
	}

	wsDir, err := workspace.Find()
	if err != nil {
		return errorResult(err.Error())
	}

	path := workspace.CSVPath(wsDir, args.Section)
	if err := workspace.RemoveRows(path, args.Key, args.Locale); err != nil {
		return errorResult(fmt.Sprintf("remove rows: %s", err))
	}

	return textResult(fmt.Sprintf("removed %s", args.Key))
}

func handleToolLocalLs(raw json.RawMessage) *CallToolResult {
	var args struct {
		Section string `json:"section"`
	}
	json.Unmarshal(raw, &args)

	wsDir, err := workspace.Find()
	if err != nil {
		return errorResult(err.Error())
	}

	type entry struct {
		Section string `json:"section"`
		Key     string `json:"key"`
		Locale  string `json:"locale"`
		Value   string `json:"value"`
		Format  string `json:"format"`
	}

	var entries []entry

	if args.Section != "" {
		path := workspace.CSVPath(wsDir, args.Section)
		rows, err := workspace.ReadCSV(path)
		if err != nil {
			return errorResult(fmt.Sprintf("read CSV: %s", err))
		}
		for _, r := range rows {
			entries = append(entries, entry{Section: args.Section, Key: r.Key, Locale: r.Locale, Value: r.Value, Format: r.Format})
		}
	} else {
		paths, err := workspace.AllCSVPaths(wsDir)
		if err != nil {
			return errorResult(fmt.Sprintf("scan workspace: %s", err))
		}
		for secName, path := range paths {
			rows, err := workspace.ReadCSV(path)
			if err != nil {
				return errorResult(fmt.Sprintf("read %s: %s", path, err))
			}
			for _, r := range rows {
				entries = append(entries, entry{Section: secName, Key: r.Key, Locale: r.Locale, Value: r.Value, Format: r.Format})
			}
		}
	}

	result, _ := json.Marshal(entries)
	return textResult(string(result))
}

func handleToolPush(raw json.RawMessage) *CallToolResult {
	var args struct {
		Section string `json:"section"`
	}
	json.Unmarshal(raw, &args)

	wsDir, err := workspace.Find()
	if err != nil {
		return errorResult(err.Error())
	}

	wsCfg, err := workspace.LoadConfig(wsDir)
	if err != nil {
		return errorResult(err.Error())
	}

	c, err := workspace.ResolveClient(wsCfg)
	if err != nil {
		return errorResult(err.Error())
	}

	result, err := workspace.Push(c, wsDir, args.Section)
	if err != nil {
		return errorResult(fmt.Sprintf("push: %s", err))
	}

	out, _ := json.Marshal(result)
	return textResult(string(out))
}

func handleToolPull(raw json.RawMessage) *CallToolResult {
	var args struct {
		Section string `json:"section"`
	}
	json.Unmarshal(raw, &args)

	wsDir, err := workspace.Find()
	if err != nil {
		return errorResult(err.Error())
	}

	wsCfg, err := workspace.LoadConfig(wsDir)
	if err != nil {
		return errorResult(err.Error())
	}

	c, err := workspace.ResolveClient(wsCfg)
	if err != nil {
		return errorResult(err.Error())
	}

	result, err := workspace.Pull(c, wsDir, args.Section)
	if err != nil {
		return errorResult(fmt.Sprintf("pull: %s", err))
	}

	out, _ := json.Marshal(result)
	return textResult(string(out))
}

func handleToolPublish(raw json.RawMessage) *CallToolResult {
	var args struct {
		Locales string `json:"locales"`
	}
	json.Unmarshal(raw, &args)

	wsDir, err := workspace.Find()
	if err != nil {
		return errorResult(err.Error())
	}

	wsCfg, err := workspace.LoadConfig(wsDir)
	if err != nil {
		return errorResult(err.Error())
	}

	c, err := workspace.ResolveClient(wsCfg)
	if err != nil {
		return errorResult(err.Error())
	}

	var locales []string
	if args.Locales != "" {
		for _, l := range splitComma(args.Locales) {
			if l != "" {
				locales = append(locales, l)
			}
		}
	}

	resp, err := c.PublishBundles(locales)
	if err != nil {
		return errorResult(fmt.Sprintf("publish: %s", err))
	}

	out, _ := json.Marshal(resp)
	return textResult(string(out))
}

func splitComma(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}
