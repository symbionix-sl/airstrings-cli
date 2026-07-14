package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
	"github.com/symbionix-sl/airstrings-cli/internal/workspace"
)

const (
	stringsSetDescription = "Add or update a string in the local workspace CSV. Writes to .airstrings/<section>/<section>.csv or .airstrings/strings.csv. Local by default; set push=true to also sync the key to the API immediately."
	stringsRmDescription  = "Remove a string from the local workspace CSV. Local by default; set push=true to also mirror the removal to the API immediately."
	stringsLsDescription  = "List all strings in the local workspace. Returns strings from local CSV files, not from the API."
)

var stringsSetSchema = InputSchema{
	Type: "object",
	Properties: map[string]Property{
		"key":     {Type: "string", Description: "The string key (e.g., 'onboarding.welcome')"},
		"values":  {Type: "string", Description: "JSON object of locale=value pairs, e.g. {\"en\": \"Hello\", \"it\": \"Ciao\"}"},
		"format":  {Type: "string", Description: "String format: 'text' or 'icu'. Required."},
		"section": {Type: "string", Description: "Section name. If omitted, string goes to flat strings.csv"},
		"push":    {Type: "boolean", Description: "Also push this key to the API immediately after the local write."},
	},
	Required: []string{"key", "values", "format"},
}

var stringsRmSchema = InputSchema{
	Type: "object",
	Properties: map[string]Property{
		"key":     {Type: "string", Description: "The string key to remove"},
		"locale":  {Type: "string", Description: "Remove only this locale. If omitted, removes all locales for the key."},
		"section": {Type: "string", Description: "Section to remove from. If omitted, removes from flat strings.csv"},
		"push":    {Type: "boolean", Description: "Also mirror the removal to the API immediately."},
	},
	Required: []string{"key"},
}

var stringsLsSchema = InputSchema{
	Type: "object",
	Properties: map[string]Property{
		"section": {Type: "string", Description: "Filter to a specific section. If omitted, lists all sections."},
	},
}

var toolDefs = []ToolDef{
	{
		Name:        "airstrings_init",
		Description: "Initialize an AirStrings workspace in the current directory. Requires an API key to authenticate and set up the project.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"api_key":  {Type: "string", Description: "AirStrings API key for authentication."},
				"base_url": {Type: "string", Description: "API base URL. Defaults to https://api.airstrings.com if omitted."},
				"dir":      {Type: "string", Description: "Directory to initialize. Uses current working directory if omitted."},
			},
			Required: []string{"api_key"},
		},
	},
	{
		Name:        "airstrings_strings_set",
		Description: stringsSetDescription,
		InputSchema: stringsSetSchema,
	},
	{
		Name:        "airstrings_strings_rm",
		Description: stringsRmDescription,
		InputSchema: stringsRmSchema,
	},
	{
		Name:        "airstrings_strings_ls",
		Description: stringsLsDescription,
		InputSchema: stringsLsSchema,
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
	{
		Name:        "airstrings_promote_preview",
		Description: "Read-only diff of what promoting strings from one environment to another would change (added, updated, and extra keys per locale). Applies nothing — a human reviews the diff and applies the promotion in the webapp.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"source_env": {Type: "string", Description: "Source environment NAME. Defaults to the active environment if omitted."},
				"target_env": {Type: "string", Description: "Target environment NAME. Defaults to the default environment if omitted."},
			},
		},
	},
	{
		Name:        "airstrings_variant_set",
		Description: "Create or replace the A/B experiment for a string key in the active environment. Replaces the whole experiment: allocation maps each variant name to an integer weight, variants maps each variant name to an object of locale=value pairs.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"key":        {Type: "string", Description: "The string key the experiment belongs to (e.g., 'onboarding.welcome')."},
				"allocation": {Type: "object", Description: "Object mapping variant name to integer weight, e.g. {\"control\": 50, \"treatment\": 50}."},
				"variants":   {Type: "object", Description: "Object mapping variant name to an object of locale=value pairs, e.g. {\"control\": {\"en\": \"Hello\"}, \"treatment\": {\"en\": \"Hi\"}}."},
			},
			Required: []string{"key"},
		},
	},
	{
		Name:        "airstrings_variant_status",
		Description: "Get the current A/B experiment for a string key in the active environment (status, allocation, variants).",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"key": {Type: "string", Description: "The string key whose experiment to read."},
			},
			Required: []string{"key"},
		},
	},
	{
		Name:        "airstrings_variant_start",
		Description: "Start the A/B experiment for a string key so its variants are served. If the active environment is protected production, the server will refuse — start the experiment in a writable environment instead. Does not promise success.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"key": {Type: "string", Description: "The string key whose experiment to start."},
			},
			Required: []string{"key"},
		},
	},
	{
		Name:        "airstrings_variant_stop",
		Description: "Stop the A/B experiment for a string key. Serving reverts to the base string.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"key": {Type: "string", Description: "The string key whose experiment to stop."},
			},
			Required: []string{"key"},
		},
	},
	{
		Name:        "airstrings_variant_promote",
		Description: "Promote one variant of a string's experiment to become the published value. If the active environment is protected production, the server will refuse — promote in a writable environment instead. Does not promise success.",
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"key":     {Type: "string", Description: "The string key whose experiment to promote."},
				"variant": {Type: "string", Description: "The variant name to promote to the winning value."},
			},
			Required: []string{"key", "variant"},
		},
	},
}

type toolHandler func(args json.RawMessage) *CallToolResult

var toolHandlers = map[string]toolHandler{
	"airstrings_init":            handleToolInit,
	"airstrings_strings_set":     handleToolStringsSet,
	"airstrings_strings_rm":      handleToolStringsRm,
	"airstrings_strings_ls":      handleToolStringsLs,
	"airstrings_push":            handleToolPush,
	"airstrings_pull":            handleToolPull,
	"airstrings_publish":         handleToolPublish,
	"airstrings_promote_preview": handleToolPromotePreview,
	"airstrings_variant_set":     handleToolVariantSet,
	"airstrings_variant_status":  handleToolVariantStatus,
	"airstrings_variant_start":   handleToolVariantStart,
	"airstrings_variant_stop":    handleToolVariantStop,
	"airstrings_variant_promote": handleToolVariantPromote,
}

func handleToolInit(raw json.RawMessage) *CallToolResult {
	var args struct {
		APIKey  string `json:"api_key"`
		BaseURL string `json:"base_url"`
		Dir     string `json:"dir"`
	}
	json.Unmarshal(raw, &args)

	if args.APIKey == "" {
		return errorResult("api_key is required")
	}

	dir := args.Dir
	if dir == "" {
		dir, _ = os.Getwd()
	}

	// Check if workspace already exists
	wsDir := filepath.Join(dir, ".airstrings")
	if _, err := os.Stat(filepath.Join(wsDir, "config.json")); err == nil {
		return errorResult(fmt.Sprintf("workspace already exists at %s", wsDir))
	}

	// Validate key and discover project/environments
	c := client.New(args.APIKey, args.BaseURL, "", "")
	proj, err := c.GetProject()
	if err != nil {
		return errorResult(fmt.Sprintf("invalid API key: %s", err))
	}

	c2 := client.New(args.APIKey, args.BaseURL, proj.ID, "")
	envs, err := c2.ListEnvironments()
	if err != nil {
		return errorResult(fmt.Sprintf("list environments: %s", err))
	}

	// Build workspace config with credentials
	wsCfg := workspace.WorkspaceConfig{
		ProjectID:   proj.ID,
		ProjectName: proj.Name,
	}

	var activeEnvName string
	for _, env := range envs {
		cred := workspace.Credential{
			APIKey:  args.APIKey,
			BaseURL: args.BaseURL,
			EnvID:   env.ID,
			EnvName: env.Name,
		}
		wsCfg.AddOrUpdate(cred)
		if env.IsDefault || wsCfg.ActiveEnv == "" {
			wsCfg.ActiveEnv = env.ID
			activeEnvName = env.Name
		}
	}

	if err := workspace.Init(dir, wsCfg); err != nil {
		return errorResult(fmt.Sprintf("init workspace: %s", err))
	}

	// Create section dirs for remote sections
	c3 := client.New(args.APIKey, args.BaseURL, proj.ID, wsCfg.ActiveEnv)
	sections, err := c3.ListSections()
	sectionCount := 0
	if err == nil {
		for _, sec := range sections.Data {
			workspace.CreateSectionDir(wsDir, sec.Name)
		}
		sectionCount = len(sections.Data)
	}

	result, _ := json.Marshal(map[string]any{
		"project":      proj.Name,
		"environment":  activeEnvName,
		"environments": len(envs),
		"sections":     sectionCount,
	})
	return textResult(string(result))
}

func resolvePushClient(wsDir string) (*client.Client, *CallToolResult) {
	wsCfg, err := workspace.LoadConfig(wsDir)
	if err != nil {
		return nil, errorResult(err.Error())
	}
	c, err := workspace.ResolveClient(wsCfg)
	if err != nil {
		return nil, errorResult(err.Error())
	}
	return c, nil
}

func handleToolStringsSet(raw json.RawMessage) *CallToolResult {
	var args struct {
		Key     string `json:"key"`
		Values  string `json:"values"`
		Format  string `json:"format"`
		Section string `json:"section"`
		Push    bool   `json:"push"`
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
		return errorResult("format is required — must be 'text' or 'icu'")
	}
	if err := workspace.ValidateFormat(format); err != nil {
		return errorResult(err.Error())
	}

	var warning string
	if flagged := workspace.FlagICUInText(format, values); len(flagged) > 0 {
		warning = fmt.Sprintf("text value with {…} placeholders in locale(s) %s — did you mean format 'icu'? text is served verbatim, braces are not interpolated", strings.Join(flagged, ", "))
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

	if args.Push {
		c, errRes := resolvePushClient(wsDir)
		if errRes != nil {
			return errRes
		}
		if err := workspace.PushKey(c, args.Key, values, format, args.Section); err != nil {
			return errorResult(fmt.Sprintf("push %s: %s", args.Key, err))
		}
	}

	out := map[string]any{
		"key":     args.Key,
		"locales": len(values),
		"section": args.Section,
		"format":  format,
		"pushed":  args.Push,
	}
	if warning != "" {
		out["warning"] = warning
	}
	result, _ := json.Marshal(out)
	return textResult(string(result))
}

func handleToolStringsRm(raw json.RawMessage) *CallToolResult {
	var args struct {
		Key     string `json:"key"`
		Locale  string `json:"locale"`
		Section string `json:"section"`
		Push    bool   `json:"push"`
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

	if args.Push {
		c, errRes := resolvePushClient(wsDir)
		if errRes != nil {
			return errRes
		}
		if err := workspace.PushKeyRemoval(c, args.Key, args.Locale); err != nil {
			return errorResult(fmt.Sprintf("push removal %s: %s", args.Key, err))
		}
	}

	msg := fmt.Sprintf("removed %s", args.Key)
	if args.Push {
		msg += " (pushed)"
	}
	return textResult(msg)
}

func handleToolStringsLs(raw json.RawMessage) *CallToolResult {
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

	result, err := workspace.Push(c, wsDir, args.Section, nil)
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

func handleToolPromotePreview(raw json.RawMessage) *CallToolResult {
	var args struct {
		SourceEnv string `json:"source_env"`
		TargetEnv string `json:"target_env"`
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

	envs, err := c.ListEnvironments()
	if err != nil {
		return errorResult(fmt.Sprintf("list environments: %s", err))
	}

	sourceID := c.EnvID()
	if args.SourceEnv != "" {
		id, errRes := resolveEnvID(envs, args.SourceEnv)
		if errRes != nil {
			return errRes
		}
		sourceID = id
	}

	targetID := ""
	for _, e := range envs {
		if e.IsDefault {
			targetID = e.ID
			break
		}
	}
	if args.TargetEnv != "" {
		id, errRes := resolveEnvID(envs, args.TargetEnv)
		if errRes != nil {
			return errRes
		}
		targetID = id
	}

	if sourceID == "" {
		return errorResult("could not resolve source environment — pass source_env")
	}
	if targetID == "" {
		return errorResult("could not resolve target environment — pass target_env")
	}
	if sourceID == targetID {
		return errorResult("source and target are the same environment — pass an explicit source_env")
	}

	resp, err := c.PromotionPreview(sourceID, targetID)
	if err != nil {
		return errorResult(fmt.Sprintf("promotion preview: %s", err))
	}

	out, _ := json.Marshal(resp)
	return textResult(string(out))
}

func handleToolVariantSet(raw json.RawMessage) *CallToolResult {
	var args struct {
		Key        string                       `json:"key"`
		Allocation map[string]int               `json:"allocation"`
		Variants   map[string]map[string]string `json:"variants"`
	}
	json.Unmarshal(raw, &args)

	if args.Key == "" {
		return errorResult("key is required")
	}

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

	exp, err := c.PutExperiment(args.Key, args.Allocation, args.Variants)
	if err != nil {
		return errorResult(fmt.Sprintf("variant set: %s", err))
	}

	out, _ := json.Marshal(exp)
	return textResult(string(out))
}

func handleToolVariantStatus(raw json.RawMessage) *CallToolResult {
	var args struct {
		Key string `json:"key"`
	}
	json.Unmarshal(raw, &args)

	if args.Key == "" {
		return errorResult("key is required")
	}

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

	exp, err := c.GetExperiment(args.Key)
	if err != nil {
		return errorResult(fmt.Sprintf("variant status: %s", err))
	}

	out, _ := json.Marshal(exp)
	return textResult(string(out))
}

func handleToolVariantStart(raw json.RawMessage) *CallToolResult {
	var args struct {
		Key string `json:"key"`
	}
	json.Unmarshal(raw, &args)

	if args.Key == "" {
		return errorResult("key is required")
	}

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

	exp, err := c.StartExperiment(args.Key)
	if err != nil {
		return errorResult(fmt.Sprintf("variant start: %s", err))
	}

	out, _ := json.Marshal(exp)
	return textResult(string(out))
}

func handleToolVariantStop(raw json.RawMessage) *CallToolResult {
	var args struct {
		Key string `json:"key"`
	}
	json.Unmarshal(raw, &args)

	if args.Key == "" {
		return errorResult("key is required")
	}

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

	exp, err := c.StopExperiment(args.Key)
	if err != nil {
		return errorResult(fmt.Sprintf("variant stop: %s", err))
	}

	out, _ := json.Marshal(exp)
	return textResult(string(out))
}

func handleToolVariantPromote(raw json.RawMessage) *CallToolResult {
	var args struct {
		Key     string `json:"key"`
		Variant string `json:"variant"`
	}
	json.Unmarshal(raw, &args)

	if args.Key == "" {
		return errorResult("key is required")
	}
	if args.Variant == "" {
		return errorResult("variant is required")
	}

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

	resp, err := c.PromoteVariant(args.Key, args.Variant)
	if err != nil {
		return errorResult(fmt.Sprintf("variant promote: %s", err))
	}

	out, _ := json.Marshal(resp)
	return textResult(string(out))
}

func resolveEnvID(envs []client.Environment, name string) (string, *CallToolResult) {
	for _, e := range envs {
		if strings.EqualFold(e.Name, name) || e.ID == name {
			return e.ID, nil
		}
	}
	var names []string
	for _, e := range envs {
		names = append(names, e.Name)
	}
	return "", errorResult(fmt.Sprintf("environment %q not found. Available: %s", name, strings.Join(names, ", ")))
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
