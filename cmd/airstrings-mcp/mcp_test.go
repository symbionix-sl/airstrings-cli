package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/symbionix/airstrings-cli/internal/workspace"
)

// mcpExchange sends a JSON-RPC request to the MCP server and returns the response.
func mcpExchange(t *testing.T, server *MCPServer, method string, id int, params any) *JSONRPCResponse {
	t.Helper()

	reqBody := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		raw, _ := json.Marshal(params)
		reqBody["params"] = json.RawMessage(raw)
	}

	line, _ := json.Marshal(reqBody)
	line = append(line, '\n')

	var out bytes.Buffer
	in := bytes.NewReader(line)

	scanner := bufio.NewScanner(in)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, len(buf))
	encoder := json.NewEncoder(&out)

	for scanner.Scan() {
		b := scanner.Bytes()
		if len(b) == 0 {
			continue
		}
		var req JSONRPCRequest
		json.Unmarshal(b, &req)
		resp := server.handle(req)
		if resp != nil {
			encoder.Encode(resp)
		}
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v\nraw: %s", err, out.String())
	}
	return &resp
}

func TestMCP_Initialize(t *testing.T) {
	server := &MCPServer{}
	resp := mcpExchange(t, server, "initialize", 1, map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "1.0"},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	result, _ := json.Marshal(resp.Result)
	var initResult InitializeResult
	json.Unmarshal(result, &initResult)

	if initResult.ProtocolVersion != "2024-11-05" {
		t.Errorf("expected protocol version 2024-11-05, got %q", initResult.ProtocolVersion)
	}
	if initResult.ServerInfo.Name != "airstrings-mcp" {
		t.Errorf("expected server name 'airstrings-mcp', got %q", initResult.ServerInfo.Name)
	}
	if initResult.Capabilities.Tools == nil {
		t.Error("expected tools capability to be present")
	}
}

func TestMCP_ToolsList(t *testing.T) {
	server := &MCPServer{}
	resp := mcpExchange(t, server, "tools/list", 2, map[string]any{})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	result, _ := json.Marshal(resp.Result)
	var toolsList ToolsListResult
	json.Unmarshal(result, &toolsList)

	expectedTools := map[string]bool{
		"airstrings_init":      false,
		"airstrings_local_set": false,
		"airstrings_local_rm":  false,
		"airstrings_local_ls":  false,
		"airstrings_push":      false,
		"airstrings_pull":      false,
		"airstrings_publish":   false,
	}

	for _, tool := range toolsList.Tools {
		if _, ok := expectedTools[tool.Name]; ok {
			expectedTools[tool.Name] = true
		}
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
		if tool.InputSchema.Type != "object" {
			t.Errorf("tool %q has wrong schema type: %q", tool.Name, tool.InputSchema.Type)
		}
	}

	for name, found := range expectedTools {
		if !found {
			t.Errorf("expected tool %q not found in tools/list", name)
		}
	}
}

func TestMCP_ToolCall_LocalSet(t *testing.T) {
	// Set up a workspace so local_set has somewhere to write
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	workspace.Init(dir, workspace.WorkspaceConfig{
		ProjectID: "p", EnvID: "e",
	})

	// Change to workspace dir so Find() works
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	server := &MCPServer{}
	resp := mcpExchange(t, server, "tools/call", 3, map[string]any{
		"name": "airstrings_local_set",
		"arguments": map[string]any{
			"key":    "greeting",
			"values": `{"en": "Hello", "it": "Ciao"}`,
			"format": "text",
		},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	// Parse the tool result
	resultJSON, _ := json.Marshal(resp.Result)
	var toolResult CallToolResult
	json.Unmarshal(resultJSON, &toolResult)

	if toolResult.IsError {
		t.Fatalf("tool returned error: %s", toolResult.Content[0].Text)
	}

	// Verify the CSV was written
	rows, err := workspace.ReadCSV(workspace.CSVPath(wsDir, ""))
	if err != nil {
		t.Fatalf("read CSV: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}

	vals := make(map[string]string)
	for _, r := range rows {
		vals[r.Locale] = r.Value
	}
	if vals["en"] != "Hello" {
		t.Errorf("expected 'Hello', got %q", vals["en"])
	}
	if vals["it"] != "Ciao" {
		t.Errorf("expected 'Ciao', got %q", vals["it"])
	}
}

func TestMCP_ToolCall_LocalSetWithSection(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	workspace.Init(dir, workspace.WorkspaceConfig{
		ProjectID: "p", EnvID: "e",
	})

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	server := &MCPServer{}
	resp := mcpExchange(t, server, "tools/call", 4, map[string]any{
		"name": "airstrings_local_set",
		"arguments": map[string]any{
			"key":     "home.title",
			"values":  `{"en": "Home", "de": "Startseite"}`,
			"format":  "text",
			"section": "home",
		},
	})

	resultJSON, _ := json.Marshal(resp.Result)
	var toolResult CallToolResult
	json.Unmarshal(resultJSON, &toolResult)

	if toolResult.IsError {
		t.Fatalf("tool returned error: %s", toolResult.Content[0].Text)
	}

	// Verify section CSV was created
	rows, err := workspace.ReadCSV(workspace.CSVPath(wsDir, "home"))
	if err != nil {
		t.Fatalf("read CSV: %v", err)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}
}

func TestMCP_ToolCall_LocalRm(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	workspace.Init(dir, workspace.WorkspaceConfig{
		ProjectID: "p", EnvID: "e",
	})

	// Pre-populate
	workspace.SetRows(workspace.CSVPath(wsDir, ""), "greeting", map[string]string{
		"en": "Hello", "it": "Ciao",
	}, "text")

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	server := &MCPServer{}

	// Remove single locale
	resp := mcpExchange(t, server, "tools/call", 5, map[string]any{
		"name": "airstrings_local_rm",
		"arguments": map[string]any{
			"key":    "greeting",
			"locale": "it",
		},
	})

	resultJSON, _ := json.Marshal(resp.Result)
	var toolResult CallToolResult
	json.Unmarshal(resultJSON, &toolResult)

	if toolResult.IsError {
		t.Fatalf("tool returned error: %s", toolResult.Content[0].Text)
	}

	rows, _ := workspace.ReadCSV(workspace.CSVPath(wsDir, ""))
	if len(rows) != 1 {
		t.Errorf("expected 1 row after rm, got %d", len(rows))
	}
	if rows[0].Locale != "en" {
		t.Errorf("expected 'en' remaining, got %q", rows[0].Locale)
	}
}

func TestMCP_ToolCall_LocalLs(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	workspace.Init(dir, workspace.WorkspaceConfig{
		ProjectID: "p", EnvID: "e",
	})

	// Add some strings
	workspace.SetRows(workspace.CSVPath(wsDir, ""), "greeting", map[string]string{"en": "Hello"}, "text")
	workspace.SetRows(workspace.CSVPath(wsDir, "home"), "title", map[string]string{"en": "Home"}, "text")

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	server := &MCPServer{}
	resp := mcpExchange(t, server, "tools/call", 6, map[string]any{
		"name":      "airstrings_local_ls",
		"arguments": map[string]any{},
	})

	resultJSON, _ := json.Marshal(resp.Result)
	var toolResult CallToolResult
	json.Unmarshal(resultJSON, &toolResult)

	if toolResult.IsError {
		t.Fatalf("tool returned error: %s", toolResult.Content[0].Text)
	}

	// Parse the JSON array from the result text
	var entries []struct {
		Section string `json:"section"`
		Key     string `json:"key"`
		Locale  string `json:"locale"`
		Value   string `json:"value"`
		Format  string `json:"format"`
	}
	if err := json.Unmarshal([]byte(toolResult.Content[0].Text), &entries); err != nil {
		t.Fatalf("parse ls result: %v\nraw: %s", err, toolResult.Content[0].Text)
	}

	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestMCP_ToolCall_UnknownTool(t *testing.T) {
	server := &MCPServer{}
	resp := mcpExchange(t, server, "tools/call", 7, map[string]any{
		"name":      "nonexistent_tool",
		"arguments": map[string]any{},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %+v", resp.Error)
	}

	// Should return a tool result with isError=true
	resultJSON, _ := json.Marshal(resp.Result)
	var toolResult CallToolResult
	json.Unmarshal(resultJSON, &toolResult)

	if !toolResult.IsError {
		t.Error("expected isError=true for unknown tool")
	}
}

func TestMCP_UnknownMethod(t *testing.T) {
	server := &MCPServer{}
	resp := mcpExchange(t, server, "nonexistent/method", 8, nil)

	if resp.Error == nil {
		t.Fatal("expected JSON-RPC error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("expected error code -32601, got %d", resp.Error.Code)
	}
}

func TestMCP_ToolCall_LocalSetMissingKey(t *testing.T) {
	dir := t.TempDir()
	workspace.Init(dir, workspace.WorkspaceConfig{
		ProjectID: "p", EnvID: "e",
	})

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	server := &MCPServer{}
	resp := mcpExchange(t, server, "tools/call", 9, map[string]any{
		"name": "airstrings_local_set",
		"arguments": map[string]any{
			"values": `{"en": "Hello"}`,
		},
	})

	resultJSON, _ := json.Marshal(resp.Result)
	var toolResult CallToolResult
	json.Unmarshal(resultJSON, &toolResult)

	if !toolResult.IsError {
		t.Error("expected isError=true when key is missing")
	}
}

func TestMCP_ToolCall_LocalSetInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	workspace.Init(dir, workspace.WorkspaceConfig{
		ProjectID: "p", EnvID: "e",
	})

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	server := &MCPServer{}
	resp := mcpExchange(t, server, "tools/call", 10, map[string]any{
		"name": "airstrings_local_set",
		"arguments": map[string]any{
			"key":    "test",
			"values": "not valid json",
		},
	})

	resultJSON, _ := json.Marshal(resp.Result)
	var toolResult CallToolResult
	json.Unmarshal(resultJSON, &toolResult)

	if !toolResult.IsError {
		t.Error("expected isError=true for invalid values JSON")
	}
}

func TestMCP_FullWorkflow(t *testing.T) {
	// This test simulates how an AI assistant would use the MCP server:
	// 1. initialize
	// 2. local_set multiple strings
	// 3. local_ls to verify
	// 4. local_rm one locale
	// 5. local_ls again

	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	workspace.Init(dir, workspace.WorkspaceConfig{
		ProjectID: "p", EnvID: "e",
	})

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	server := &MCPServer{}

	// Step 1: Initialize
	resp := mcpExchange(t, server, "initialize", 1, map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "claude", "version": "1.0"},
	})
	if resp.Error != nil {
		t.Fatalf("initialize error: %+v", resp.Error)
	}

	// Step 2: Set strings in different sections
	for _, tc := range []struct {
		key     string
		values  string
		section string
		format  string
	}{
		{"home.welcome", `{"en": "Welcome", "it": "Benvenuto", "de": "Willkommen"}`, "home", "text"},
		{"home.subtitle", `{"en": "Your dashboard", "it": "La tua dashboard"}`, "home", "text"},
		{"login.email", `{"en": "Email address", "it": "Indirizzo email"}`, "login", "text"},
		{"login.password", `{"en": "Password", "it": "Password"}`, "login", "text"},
		{"greeting", `{"en": "Hello {name}!", "it": "Ciao {name}!"}`, "", "icu"},
	} {
		args := map[string]any{
			"key":    tc.key,
			"values": tc.values,
			"format": tc.format,
		}
		if tc.section != "" {
			args["section"] = tc.section
		}
		resp := mcpExchange(t, server, "tools/call", 2, map[string]any{
			"name":      "airstrings_local_set",
			"arguments": args,
		})
		resultJSON, _ := json.Marshal(resp.Result)
		var result CallToolResult
		json.Unmarshal(resultJSON, &result)
		if result.IsError {
			t.Fatalf("set %s error: %s", tc.key, result.Content[0].Text)
		}
	}

	// Step 3: List all strings
	resp = mcpExchange(t, server, "tools/call", 3, map[string]any{
		"name":      "airstrings_local_ls",
		"arguments": map[string]any{},
	})
	resultJSON, _ := json.Marshal(resp.Result)
	var lsResult CallToolResult
	json.Unmarshal(resultJSON, &lsResult)

	var entries []struct {
		Key    string `json:"key"`
		Locale string `json:"locale"`
	}
	json.Unmarshal([]byte(lsResult.Content[0].Text), &entries)

	// home: 2 keys × (3+2) = 5 locale rows, login: 2 keys × 2 = 4, flat: 1 key × 2 = 2 → total 11
	if len(entries) != 11 {
		t.Errorf("expected 11 entries, got %d", len(entries))
	}

	// Step 4: Remove Italian from home.welcome
	resp = mcpExchange(t, server, "tools/call", 4, map[string]any{
		"name": "airstrings_local_rm",
		"arguments": map[string]any{
			"key":     "home.welcome",
			"locale":  "it",
			"section": "home",
		},
	})
	resultJSON, _ = json.Marshal(resp.Result)
	var rmResult CallToolResult
	json.Unmarshal(resultJSON, &rmResult)
	if rmResult.IsError {
		t.Fatalf("rm error: %s", rmResult.Content[0].Text)
	}

	// Step 5: Verify home section now has 4 rows (was 5, removed 1)
	resp = mcpExchange(t, server, "tools/call", 5, map[string]any{
		"name": "airstrings_local_ls",
		"arguments": map[string]any{
			"section": "home",
		},
	})
	resultJSON, _ = json.Marshal(resp.Result)
	var lsResult2 CallToolResult
	json.Unmarshal(resultJSON, &lsResult2)

	var homeEntries []struct{ Key string }
	json.Unmarshal([]byte(lsResult2.Content[0].Text), &homeEntries)
	if len(homeEntries) != 4 {
		t.Errorf("expected 4 home entries after rm, got %d", len(homeEntries))
	}

	// Verify files on disk
	homeRows, _ := workspace.ReadCSV(workspace.CSVPath(wsDir, "home"))
	loginRows, _ := workspace.ReadCSV(workspace.CSVPath(wsDir, "login"))
	flatRows, _ := workspace.ReadCSV(workspace.CSVPath(wsDir, ""))

	if len(homeRows) != 4 {
		t.Errorf("expected 4 home CSV rows, got %d", len(homeRows))
	}
	if len(loginRows) != 4 {
		t.Errorf("expected 4 login CSV rows, got %d", len(loginRows))
	}
	if len(flatRows) != 2 {
		t.Errorf("expected 2 flat CSV rows, got %d", len(flatRows))
	}

	// Verify ICU format preserved for the flat greeting
	for _, r := range flatRows {
		if r.Key == "greeting" && r.Format != "icu" {
			t.Errorf("expected icu format for greeting, got %q", r.Format)
		}
	}
}
