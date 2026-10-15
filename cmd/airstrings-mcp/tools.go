package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
	"github.com/symbionix-sl/airstrings-cli/internal/guide"
	"github.com/symbionix-sl/airstrings-cli/internal/workspace"
)

var (
	readOnlyLocal  = &ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true}
	readOnlyRemote = &ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: true}
)

var toolDefs = []ToolDef{
	{
		Name:        "airstrings_init",
		Description: "Bind a directory to an AirStrings project; call this first in a repo with no .airstrings workspace, before any other airstrings_* tool. Writes .airstrings/config.json and stores the key in the workspace. Credentials: api_key, else AIRSTRINGS_ORG_API_KEY, AIRSTRINGS_API_KEY, else the stored login. With none it returns status \"pending\" and verification_uri_complete: show the user that URL, and once they approve call init again with the same arguments. With an org key it creates a project named after the folder (or name) unless project names an existing one. Returns the project, whether it was created, the active environment, all environments and sections. Use airstrings_login instead only to store an org login without binding a directory.",
		Annotations: &ToolAnnotations{IdempotentHint: true, OpenWorldHint: true},
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"api_key":  {Type: "string", Description: "Project key (as_proj_…), org key (as_org_…) or legacy environment key. Optional: overrides the env vars and stored login."},
				"base_url": {Type: "string", Description: "API base URL. Defaults to https://api.airstrings.com; only set it for self-hosted or test servers."},
				"dir":      {Type: "string", Description: "Directory to initialize (absolute or relative). Defaults to the current working directory."},
				"name":     {Type: "string", Description: "Name for a project created with an org key. Defaults to the folder or repository name. Ignored when project is set."},
				"project":  {Type: "string", Description: "Existing project ID (proj_…) or name to bind instead of creating one. Needs an org key or org login."},
				"org":      {Type: "string", Description: "Organization ID (org_…) to bind to when the login has several. Uses the stored login for that org, or starts one."},
			},
		},
	},
	{
		Name:        "airstrings_login",
		Description: "Log in to an AirStrings organization and store an org key on this machine, without binding a directory. The first call returns status \"pending\" and verification_uri_complete: an organization owner approves that URL in the browser, then call login again to finish; if it is denied or expires, start over. Returns the organization once stored. Use it before airstrings_init when several directories will share one login; to set up a single repo, call airstrings_init directly (it logs in when needed).",
		Annotations: &ToolAnnotations{OpenWorldHint: true},
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"base_url": {Type: "string", Description: "API base URL. Defaults to https://api.airstrings.com; only set it for self-hosted or test servers."},
				"org":      {Type: "string", Description: "Organization ID (org_…) the approval must be for; the approval is rejected for any other org. Optional."},
			},
		},
	},
	{
		Name:        "airstrings_strings_set",
		Description: "Add or update one string key in the local workspace CSV (.airstrings/<section>/<section>.csv, or .airstrings/strings.csv without a section). Overwrites the given locales of that key and keeps its other locales. Local only unless push=true, which also upserts the key in the active environment; on a protected environment that push is refused and the local change is rolled back. Returns the key, section and whether it was pushed. Use this for a few keys; after editing many keys, run airstrings_push once instead. Apps see the change only after airstrings_publish.",
		Annotations: &ToolAnnotations{DestructiveHint: true, IdempotentHint: true, OpenWorldHint: true},
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"key":     {Type: "string", Description: "Dot-separated string key, e.g. 'onboarding.welcome'. Created if it does not exist."},
				"values":  {Type: "string", Description: "JSON object string of locale code to text, e.g. {\"en\": \"Hello\", \"it\": \"Ciao\"}."},
				"format":  {Type: "string", Description: "'text' for plain strings or 'icu' for ICU MessageFormat (plurals, select, {placeholders}). Applies to every locale of the key."},
				"section": {Type: "string", Description: "Section (group of keys) to write to; created on push if missing. Omit for unsectioned strings."},
				"push":    {Type: "boolean", Description: "Default false. When true, also upsert this key in the active environment right after the local write."},
			},
			Required: []string{"key", "values", "format"},
		},
	},
	{
		Name:        "airstrings_strings_rm",
		Description: "Remove a string key, or one locale of it, from the local workspace CSV. Local only unless push=true, which also deletes it in the active environment (refused on a protected environment). Deletion cannot be undone except by setting the string again. Returns what was removed and whether it was pushed. Apps keep the old value until the next airstrings_publish.",
		Annotations: &ToolAnnotations{DestructiveHint: true, IdempotentHint: true, OpenWorldHint: true},
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"key":     {Type: "string", Description: "The string key to remove, e.g. 'onboarding.welcome'."},
				"locale":  {Type: "string", Description: "Remove only this locale's value (e.g. 'it'). Omit to remove the whole key in every locale."},
				"section": {Type: "string", Description: "Section the key lives in. Omit for unsectioned strings."},
				"push":    {Type: "boolean", Description: "Default false. When true, also delete it in the active environment."},
			},
			Required: []string{"key"},
		},
	},
	{
		Name:        "airstrings_strings_ls",
		Description: "List the strings in the local workspace CSVs: key, section, format and the value per locale. Reads files only, never the API, so it shows unpushed edits; run airstrings_pull first to see what the server currently holds.",
		Annotations: readOnlyLocal,
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"section": {Type: "string", Description: "Only list this section. Omit to list every section and the unsectioned strings."},
			},
		},
	},
	{
		Name:        "airstrings_push",
		Description: "Upload the local workspace CSVs to the active environment: upserts every key and locale value, overwriting the server's values for those keys, and creates missing sections. Keys that exist only on the server are left alone (nothing is deleted). Refused on a protected environment: push to staging, then a human promotes. Returns upserted and error counts, the sections and any failed keys. Use after many airstrings_strings_set edits; for one key, strings_set with push=true is enough. Apps see the change only after airstrings_publish.",
		Annotations: &ToolAnnotations{DestructiveHint: true, IdempotentHint: true, OpenWorldHint: true},
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"section": {Type: "string", Description: "Only push this section's CSV. Omit to push every section and the unsectioned strings."},
			},
		},
	},
	{
		Name:        "airstrings_pull",
		Description: "Download the active environment's strings into the local workspace, overwriting the local CSVs with the server state; unpushed local edits in those files are lost, so run airstrings_push first if you want to keep them. Read-only on the server. Returns the string, section and file counts. Use it to start from the server's state; to inspect local files without touching them, use airstrings_strings_ls.",
		Annotations: &ToolAnnotations{DestructiveHint: true, IdempotentHint: true, OpenWorldHint: true},
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"section": {Type: "string", Description: "Only overwrite this section's CSV. Omit to pull every section."},
			},
		},
	},
	{
		Name:        "airstrings_publish",
		Description: "Make the active environment's current server strings live: builds one bundle per locale, signs it (Ed25519) and uploads it to the CDN, where the SDKs pick it up on their next fetch. Publishes what is on the server, so run airstrings_push first for local edits. Each call creates a new bundle revision that replaces the live one; to undo, set the old values and publish again. Refused on a protected environment (production): publish to staging, then use airstrings_promote_preview and hand the apply_url to a human. Returns per-locale status and the published_at time.",
		Annotations: &ToolAnnotations{DestructiveHint: true, OpenWorldHint: true},
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"locales": {Type: "string", Description: "Comma-separated locale codes to publish, e.g. 'en,it'. Omit to publish every locale of the project."},
			},
		},
	},
	{
		Name:        "airstrings_sdk_config",
		Description: "Public, non-secret SDK setup values for one environment — safe to show the user, commit and embed in app code; no API key or secret is returned. Returns organization, project and environment IDs, Ed25519 public key(s) (verification keys, not credentials), protection (protected/open) and ready-to-paste initializers for web, react_native, ios, android and go. Use it when wiring the AirStrings SDK into an app. Works with any API key of the project — a staging key returns production's config, so no production key is needed for SDK setup.",
		Annotations: readOnlyRemote,
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"env": {Type: "string", Description: "Environment NAME (e.g. 'staging') or ID (env_…). Defaults to the default (production) environment, which is what shipped apps use."},
			},
		},
	},
	{
		Name:        "airstrings_status",
		Description: "Workspace status, same as `airstrings status`: project, active environment, key scope (read/write), protection of every environment (protected/open), the keys stored in this workspace (masked; `airstrings apikey revoke` removes them) and the one next step for this state. Call it first when unsure what to do next or why a write was refused. For just the environment list, use airstrings_env_list.",
		Annotations: readOnlyRemote,
		InputSchema: InputSchema{Type: "object", Properties: map[string]Property{}},
	},
	{
		Name:        "airstrings_env_list",
		Description: "List every environment of the project, same as `airstrings env`: name, ID, whether it is the default, protection (protected/open), whether this workspace holds a key for it, and which one is active. Use it to pick environment names for airstrings_promote_preview or airstrings_sdk_config; for the overall workspace state and next step, use airstrings_status.",
		Annotations: readOnlyRemote,
		InputSchema: InputSchema{Type: "object", Properties: map[string]Property{}},
	},
	{
		Name:        "airstrings_promote_preview",
		Description: "Read-only diff of what promoting strings from one environment to another would change (added, updated, and extra keys per locale). Applies nothing — a human reviews the diff and applies the promotion at apply_url in the dashboard. Use it after publishing to staging to hand a production release to a human; agents cannot promote to a protected environment themselves.",
		Annotations: readOnlyRemote,
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"source_env": {Type: "string", Description: "Environment NAME to promote from, e.g. 'staging'. Defaults to the active environment."},
				"target_env": {Type: "string", Description: "Environment NAME to promote to, e.g. 'production'. Defaults to the default environment. Must differ from source_env."},
			},
		},
	},
	{
		Name:        "airstrings_variant_set",
		Description: "Create or replace the A/B experiment for one text-format string key that already exists on the server in the active environment. Replaces the whole experiment as a new draft, so pass every variant each time; the key's base string is the implicit 'control' variant. Refused while the experiment is running (stop it first) and on a protected environment. Does not start it: call airstrings_variant_start, then airstrings_publish. Returns the stored experiment. To read the current one first, use airstrings_variant_status.",
		Annotations: &ToolAnnotations{DestructiveHint: true, IdempotentHint: true, OpenWorldHint: true},
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"key":        {Type: "string", Description: "Server-side string key (format text) the experiment belongs to, e.g. 'onboarding.welcome'."},
				"allocation": {Type: "object", Description: "Variant name to integer traffic weight, including 'control', e.g. {\"control\": 50, \"treatment\": 50}. Weights are relative; give every variant in variants a weight."},
				"variants":   {Type: "object", Description: "Each non-control variant name to an object of locale=value pairs, e.g. {\"treatment\": {\"en\": \"Hi\"}}. Never include 'control' — it is the base string. Give a value for every locale the base string has, or airstrings_variant_start will refuse."},
			},
			Required: []string{"key", "allocation", "variants"},
		},
	},
	{
		Name:        "airstrings_variant_status",
		Description: "Read the A/B experiment for one string key in the active environment: status (draft, running or stopped), allocation and each variant's values. Returns an error if the key has no experiment. Use it before airstrings_variant_set to keep existing variants, or to check state before start, stop or promote.",
		Annotations: readOnlyRemote,
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"key": {Type: "string", Description: "The string key whose experiment to read, e.g. 'onboarding.welcome'."},
			},
			Required: []string{"key"},
		},
	},
	{
		Name:        "airstrings_variant_start",
		Description: "Start a draft A/B experiment for one string key so its variants are served, split by allocation; takes effect in apps after the next airstrings_publish. Only a draft can start, and every variant must have a value for every locale of the base string. Refused on a protected environment (production): start it in a writable environment instead. Returns the updated experiment. Create the draft first with airstrings_variant_set.",
		Annotations: &ToolAnnotations{OpenWorldHint: true},
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
		Description: "Stop a running A/B experiment for one string key: every user gets the base string again after the next airstrings_publish. A stopped experiment cannot restart; airstrings_variant_set creates a new draft, and a stopped experiment can still be promoted. Refused on a protected environment. Returns the updated experiment. To keep a winning variant instead, use airstrings_variant_promote.",
		Annotations: &ToolAnnotations{OpenWorldHint: true},
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
		Description: "End a running or stopped A/B experiment for one string key by making a variant's values the key's base string, overwriting the old base values, then republish that environment's bundles automatically. Cannot be undone except by setting the old values again. Refused on a protected environment (production): promote in a writable environment instead. Returns the experiment and per-locale publish results. Use it once a winner is clear; to end the test without changing the base string, use airstrings_variant_stop.",
		Annotations: &ToolAnnotations{DestructiveHint: true, OpenWorldHint: true},
		InputSchema: InputSchema{
			Type: "object",
			Properties: map[string]Property{
				"key":     {Type: "string", Description: "The string key whose experiment to promote."},
				"variant": {Type: "string", Description: "Name of the winning variant, as in airstrings_variant_status (not 'control')."},
			},
			Required: []string{"key", "variant"},
		},
	},
}

type toolHandler func(args json.RawMessage) *CallToolResult

var toolHandlers = map[string]toolHandler{
	"airstrings_init":            handleToolInit,
	"airstrings_login":           handleToolLogin,
	"airstrings_strings_set":     handleToolStringsSet,
	"airstrings_strings_rm":      handleToolStringsRm,
	"airstrings_strings_ls":      handleToolStringsLs,
	"airstrings_push":            handleToolPush,
	"airstrings_pull":            handleToolPull,
	"airstrings_publish":         handleToolPublish,
	"airstrings_promote_preview": handleToolPromotePreview,
	"airstrings_sdk_config":      handleToolSDKConfig,
	"airstrings_status":          handleToolStatus,
	"airstrings_env_list":        handleToolEnvList,
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
		Name    string `json:"name"`
		Project string `json:"project"`
		Org     string `json:"org"`
	}
	json.Unmarshal(raw, &args)

	dir := args.Dir
	if dir == "" {
		dir, _ = os.Getwd()
	}

	wsDir := filepath.Join(dir, ".airstrings")
	if _, err := os.Stat(filepath.Join(wsDir, "config.json")); err == nil {
		return errorResult(fmt.Sprintf("workspace already exists at %s", wsDir))
	}

	if args.BaseURL == "" {
		args.BaseURL = os.Getenv("AIRSTRINGS_BASE_URL")
	}
	opts := workspace.SetupOptions{APIKey: args.APIKey, BaseURL: args.BaseURL, Name: args.Name, Project: args.Project, Org: args.Org}
	res, err := workspace.Setup(dir, opts)
	if errors.Is(err, workspace.ErrNoKey) {
		if _, pending := login(args.BaseURL, args.Org); pending != nil {
			return pending
		}
		res, err = workspace.Setup(dir, opts)
	}
	if err != nil {
		return setupError(err)
	}

	result, _ := json.Marshal(map[string]any{
		"project_id":   res.ProjectID,
		"project":      res.ProjectName,
		"created":      res.Created,
		"environment":  res.ActiveEnvName,
		"environments": res.Environments,
		"sections":     res.Sections,
	})
	return textResult(string(result))
}

func setupError(err error) *CallToolResult {
	var usage *workspace.UsageError
	if errors.As(err, &usage) {
		return errorResult(usage.Message + "\nNext step: " + usage.NextStep)
	}
	return errorResult(err.Error())
}

func handleToolLogin(raw json.RawMessage) *CallToolResult {
	var args struct {
		BaseURL string `json:"base_url"`
		Org     string `json:"org"`
	}
	json.Unmarshal(raw, &args)
	if args.BaseURL == "" {
		args.BaseURL = os.Getenv("AIRSTRINGS_BASE_URL")
	}
	k, pending := login(args.BaseURL, args.Org)
	if pending != nil {
		return pending
	}
	out, _ := json.Marshal(map[string]any{"status": "logged_in", "org_id": k.OrgID, "org_name": k.OrgName, "full_power": k.FullPower, "base_url": k.BaseURL})
	return textResult(string(out))
}

func login(baseURL, org string) (*workspace.OrgKey, *CallToolResult) {
	if baseURL == "" {
		baseURL = client.DefaultBaseURL
	}
	p, fresh, _, err := workspace.StartLogin(baseURL, client.ClientName(version), false)
	if err != nil {
		return nil, errorResult(fmt.Sprintf("start login: %s", err))
	}
	p.Org = org
	if !fresh {
		k, err := workspace.PollLogin(p, 0, nil)
		if err == nil && org != "" && k.OrgID != org {
			return nil, errorResult(workspace.WrongOrgMessage(k, org))
		}
		if err == nil {
			return k, nil
		}
		if !errors.Is(err, workspace.ErrLoginPending) {
			return nil, errorResult(fmt.Sprintf("login: %s", err))
		}
	} else {
		workspace.OpenBrowser(p.VerificationURIComplete)
	}
	out, _ := json.Marshal(map[string]any{
		"status":                    "pending",
		"verification_uri_complete": p.VerificationURIComplete,
		"user_code":                 p.UserCode,
		"expires_in":                int(time.Until(p.ExpiresAt).Seconds()),
		"next_step":                 workspace.PendingNextStep(p.VerificationURIComplete),
	})
	return nil, textResult(string(out))
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
	restore, err := workspace.Snapshot(path)
	if err != nil {
		return errorResult(fmt.Sprintf("read %s: %s", path, err))
	}
	if err := workspace.SetRows(path, args.Key, values, format); err != nil {
		return errorResult(fmt.Sprintf("set rows: %s", err))
	}

	if args.Push {
		c, errRes := resolvePushClient(wsDir)
		if errRes != nil {
			return errRes
		}
		if err := workspace.PushKey(c, args.Key, values, format, args.Section); err != nil {
			return errorResult(fmt.Sprintf("push %s%s: %s", args.Key, rollback(restore), err))
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
	restore, err := workspace.Snapshot(path)
	if err != nil {
		return errorResult(fmt.Sprintf("read %s: %s", path, err))
	}
	if err := workspace.RemoveRows(path, args.Key, args.Locale); err != nil {
		return errorResult(fmt.Sprintf("remove rows: %s", err))
	}

	if args.Push {
		c, errRes := resolvePushClient(wsDir)
		if errRes != nil {
			return errRes
		}
		if err := workspace.PushKeyRemoval(c, args.Key, args.Locale); err != nil {
			return errorResult(fmt.Sprintf("push removal %s%s: %s", args.Key, rollback(restore), err))
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
	resp.ApplyURL = guide.PromoteURL(c.DashboardURL(""), c.ProjectID(), targetID)

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

func rollback(restore func() error) string {
	if err := restore(); err != nil {
		return fmt.Sprintf(" (could not restore local CSV: %s — it still has the unpushed change)", err)
	}
	return " (local CSV restored — nothing changed)"
}

func handleToolSDKConfig(raw json.RawMessage) *CallToolResult {
	var args struct {
		Env string `json:"env"`
	}
	json.Unmarshal(raw, &args)

	wsDir, err := workspace.Find()
	if err != nil {
		return errorResult(err.Error())
	}
	c, errRes := resolvePushClient(wsDir)
	if errRes != nil {
		return errRes
	}
	cfg, err := c.GetSDKConfig()
	if err != nil {
		return errorResult(fmt.Sprintf("get sdk config: %s", err))
	}

	var names []string
	for _, e := range cfg.Environments {
		names = append(names, e.Name)
		if (args.Env == "" && e.IsDefault) || (args.Env != "" && (strings.EqualFold(e.Name, args.Env) || e.ID == args.Env)) {
			out, _ := json.Marshal(guide.SDKConfig(cfg, e, c.BaseURL()))
			return textResult(string(out))
		}
	}
	return errorResult(fmt.Sprintf("environment %q does not exist in this project. Available: %s", args.Env, strings.Join(names, ", ")))
}

func loadWorkspace() (*workspace.WorkspaceConfig, *client.Client, *CallToolResult) {
	wsDir, err := workspace.Find()
	if err != nil {
		return nil, nil, errorResult(err.Error())
	}
	wsCfg, err := workspace.LoadConfig(wsDir)
	if err != nil {
		return nil, nil, errorResult(err.Error())
	}
	c, err := workspace.ResolveClient(wsCfg)
	if err != nil {
		return nil, nil, errorResult(err.Error())
	}
	return wsCfg, c, nil
}

func handleToolStatus(json.RawMessage) *CallToolResult {
	wsCfg, c, errRes := loadWorkspace()
	if errRes != nil {
		return errRes
	}
	cred, err := wsCfg.ActiveCredential()
	if err != nil {
		return errorResult(err.Error())
	}
	auth, _ := workspace.ResolveAuth(wsCfg)
	d := guide.Inspect(c, auth.Key, auth.HasKey(wsCfg), auth.FullPower() == true)
	out, _ := json.Marshal(map[string]any{
		"project_id":        wsCfg.ProjectID,
		"project_name":      wsCfg.ProjectName,
		"env_id":            cred.EnvID,
		"env_name":          cred.EnvName,
		"protection":        d.Protection,
		"protection_by_env": d.ProtectionByEnv,
		"key_scope":         d.KeyScope,
		"key_type":          auth.Type(),
		"key_source":        auth.Source,
		"full_power":        auth.FullPower(),
		"hint":              d.Hint,
		"stored_keys":       wsCfg.MaskedStoredKeys(),
	})
	return textResult(string(out))
}

func handleToolEnvList(json.RawMessage) *CallToolResult {
	wsCfg, c, errRes := loadWorkspace()
	if errRes != nil {
		return errRes
	}
	envs, err := c.ListEnvironments()
	if err != nil {
		return errorResult(fmt.Sprintf("list environments: %s", err))
	}
	auth, _ := workspace.ResolveAuth(wsCfg)
	out, _ := json.Marshal(guide.EnvRows(envs, auth.HasKey(wsCfg), wsCfg.ActiveEnv))
	return textResult(string(out))
}
