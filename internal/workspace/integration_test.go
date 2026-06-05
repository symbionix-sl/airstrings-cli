package workspace_test

import (
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
	"github.com/symbionix-sl/airstrings-cli/internal/workspace"
)

// fullAPI is a more complete test API server that tracks state across
// operations, simulating the real AirStrings backend for integration tests.
type fullAPI struct {
	mu           sync.Mutex
	project      client.Project
	sections     []client.Section
	strings      map[string]*client.StringEntry // key → entry
	lastImportID string
	lastImport   *client.ImportStatus
	mux          *http.ServeMux
}

func newFullAPI() *fullAPI {
	api := &fullAPI{
		project: client.Project{
			ID:            "proj-int",
			Name:          "Integration Test",
			DefaultLocale: "en",
		},
		strings: make(map[string]*client.StringEntry),
		mux:     http.NewServeMux(),
	}
	api.mux.HandleFunc("/v1/projects", api.handleProject)
	api.mux.HandleFunc("/v1/projects/proj-int/environments", api.handleEnvironments)
	api.mux.HandleFunc("/v1/projects/proj-int/environments/env-int/sections", api.handleSections)
	api.mux.HandleFunc("/v1/projects/proj-int/environments/env-int/strings/", api.handleStringByKey)
	api.mux.HandleFunc("/v1/projects/proj-int/environments/env-int/strings", api.handleStrings)
	api.mux.HandleFunc("/v1/projects/proj-int/environments/env-int/imports", api.handleImportCreate)
	api.mux.HandleFunc("/v1/projects/proj-int/environments/env-int/imports/", api.handleImportGet)
	api.mux.HandleFunc("/v1/projects/proj-int/environments/env-int/bundles/publish", api.handlePublish)
	return api
}

func (a *fullAPI) server() *httptest.Server {
	return httptest.NewServer(a.mux)
}

func (a *fullAPI) handleProject(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(a.project)
}

func (a *fullAPI) handleEnvironments(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]any{
		"data": []client.Environment{
			{ID: "env-int", Name: "production", IsDefault: true},
		},
	})
}

func (a *fullAPI) handleSections(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch r.Method {
	case "GET":
		resp := client.SectionList{
			Data:       a.sections,
			Pagination: client.PaginationMeta{HasMore: false},
		}
		json.NewEncoder(w).Encode(resp)
	case "POST":
		var req client.CreateSectionRequest
		json.NewDecoder(r.Body).Decode(&req)
		sec := client.Section{
			ID:   "sec-" + req.Name,
			Name: req.Name,
		}
		a.sections = append(a.sections, sec)
		json.NewEncoder(w).Encode(sec)
	}
}

func (a *fullAPI) handleStrings(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch r.Method {
	case "GET":
		var entries []client.StringEntry
		for _, e := range a.strings {
			entries = append(entries, *e)
		}
		// Sort for deterministic output
		sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
		json.NewEncoder(w).Encode(client.StringList{
			Data:       entries,
			Pagination: client.PaginationMeta{HasMore: false},
		})
	}
}

func (a *fullAPI) handleStringByKey(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Check if this is a section assignment: /strings/{key}/section
	if filepath.Base(r.URL.Path) == "section" {
		key := filepath.Base(filepath.Dir(r.URL.Path))
		var req client.AssignSectionRequest
		json.NewDecoder(r.Body).Decode(&req)
		if entry, ok := a.strings[key]; ok {
			entry.SectionID = req.SectionID
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	key := filepath.Base(r.URL.Path)

	switch r.Method {
	case "PUT":
		var req client.UpsertStringRequest
		json.NewDecoder(r.Body).Decode(&req)

		entry, exists := a.strings[key]
		if !exists {
			entry = &client.StringEntry{
				Key:    key,
				Values: make(map[string]string),
			}
			a.strings[key] = entry
		}

		if req.Format != "" {
			entry.Format = req.Format
		}
		for loc, val := range req.Values {
			if val != nil {
				entry.Values[loc] = *val
			}
		}

		json.NewEncoder(w).Encode(entry)
	}
}

func (a *fullAPI) handleImportCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if err := r.ParseMultipartForm(50 << 20); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, _ := io.ReadAll(file)

	// Auto-detect gzip
	var reader io.Reader
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		gr, _ := gzip.NewReader(bytes.NewReader(data))
		defer gr.Close()
		decompressed, _ := io.ReadAll(gr)
		reader = bytes.NewReader(decompressed)
	} else {
		reader = bytes.NewReader(data)
	}

	csvReader := csv.NewReader(reader)
	records, _ := csvReader.ReadAll()

	created := 0
	updated := 0
	seenKeys := make(map[string]bool)
	if len(records) > 1 {
		for _, rec := range records[1:] { // skip header
			key := rec[0]
			locale := rec[1]
			value := rec[2]
			format := "text"
			if len(rec) > 3 && rec[3] != "" {
				format = rec[3]
			}
			section := ""
			if len(rec) > 4 {
				section = rec[4]
			}

			entry, exists := a.strings[key]
			if !exists {
				entry = &client.StringEntry{
					Key:    key,
					Format: format,
					Values: make(map[string]string),
				}
				a.strings[key] = entry
				if !seenKeys[key] {
					created++
				}
			} else if !seenKeys[key] {
				updated++
			}
			seenKeys[key] = true

			entry.Format = format
			entry.Values[locale] = value

			// Assign section if specified
			if section != "" {
				secID := ""
				for _, s := range a.sections {
					if s.Name == section {
						secID = s.ID
						break
					}
				}
				if secID == "" {
					secID = "sec-" + section
					a.sections = append(a.sections, client.Section{ID: secID, Name: section})
				}
				entry.SectionID = &secID
			}
		}
	}

	resp := client.ImportStatus{
		ID:          "imp_test_int",
		Status:      "completed",
		TotalRows:   created + updated,
		CreatedRows: created,
		UpdatedRows: updated,
	}
	a.lastImportID = resp.ID
	a.lastImport = &resp

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(resp)
}

func (a *fullAPI) handleImportGet(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.lastImport != nil {
		json.NewEncoder(w).Encode(a.lastImport)
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
}

func (a *fullAPI) handlePublish(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(client.PublishResponse{
		Results: []client.LocalePublishStatus{
			{Locale: "en", Status: "ok", Bundle: &client.BundleStatus{Revision: 1, StringCount: len(a.strings)}},
		},
	})
}

// --- Integration Tests ---

// TestIntegration_InitPushPullCycle tests the full workspace lifecycle:
// init → local set → push → verify server state → pull into fresh workspace → verify local state
func TestIntegration_InitPushPullCycle(t *testing.T) {
	api := newFullAPI()
	srv := api.server()
	defer srv.Close()

	c := client.New("test-key", srv.URL, "proj-int", "env-int")

	// Step 1: Init workspace
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")

	cfg := workspace.WorkspaceConfig{
		ProjectID: "proj-int",
		ActiveEnv: "env-int",
		Credentials: []workspace.Credential{
			{APIKey: "test-key", BaseURL: srv.URL, EnvID: "env-int", EnvName: "production"},
		},
	}
	if err := workspace.Init(dir, cfg); err != nil {
		t.Fatalf("init: %v", err)
	}

	// Verify workspace was created
	if _, err := os.Stat(filepath.Join(wsDir, "config.json")); err != nil {
		t.Fatalf("config.json not created: %v", err)
	}

	// Step 2: Add strings locally
	err := workspace.SetRows(workspace.CSVPath(wsDir, ""), "app.name", map[string]string{
		"en": "My App",
		"it": "La Mia App",
		"fr": "Mon App",
	}, "text")
	if err != nil {
		t.Fatalf("set rows: %v", err)
	}

	err = workspace.SetRows(workspace.CSVPath(wsDir, ""), "app.tagline", map[string]string{
		"en": "The best app ever",
		"it": "La migliore app di sempre",
	}, "text")
	if err != nil {
		t.Fatalf("set rows: %v", err)
	}

	// Step 3: Push to API
	pushResult, err := workspace.Push(c, wsDir, "", nil)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if pushResult.Upserted != 2 {
		t.Errorf("expected 2 upserted, got %d", pushResult.Upserted)
	}
	if pushResult.Errors != 0 {
		t.Errorf("expected 0 errors, got %d", pushResult.Errors)
	}

	// Step 4: Verify server state
	if len(api.strings) != 2 {
		t.Fatalf("expected 2 strings on server, got %d", len(api.strings))
	}
	appName := api.strings["app.name"]
	if appName == nil {
		t.Fatal("app.name not found on server")
	}
	if appName.Values["en"] != "My App" {
		t.Errorf("expected 'My App', got %q", appName.Values["en"])
	}
	if appName.Values["it"] != "La Mia App" {
		t.Errorf("expected 'La Mia App', got %q", appName.Values["it"])
	}
	if appName.Values["fr"] != "Mon App" {
		t.Errorf("expected 'Mon App', got %q", appName.Values["fr"])
	}

	// Step 5: Pull into fresh workspace
	dir2 := t.TempDir()
	wsDir2 := filepath.Join(dir2, ".airstrings")
	workspace.Init(dir2, cfg)

	pullResult, err := workspace.Pull(c, wsDir2, "")
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if pullResult.StringCount != 2 {
		t.Errorf("expected 2 strings pulled, got %d", pullResult.StringCount)
	}
	if pullResult.FileCount != 1 {
		t.Errorf("expected 1 file, got %d", pullResult.FileCount)
	}

	// Step 6: Verify pulled local state
	rows, err := workspace.ReadCSV(workspace.CSVPath(wsDir2, ""))
	if err != nil {
		t.Fatalf("read pulled CSV: %v", err)
	}
	// 2 keys: app.name (3 locales) + app.tagline (2 locales) = 5 rows
	if len(rows) != 5 {
		t.Errorf("expected 5 rows in pulled CSV, got %d", len(rows))
	}

	// Verify specific values
	found := make(map[string]string)
	for _, r := range rows {
		found[r.Key+"/"+r.Locale] = r.Value
	}
	if found["app.name/en"] != "My App" {
		t.Errorf("expected 'My App', got %q", found["app.name/en"])
	}
	if found["app.tagline/it"] != "La migliore app di sempre" {
		t.Errorf("expected Italian tagline, got %q", found["app.tagline/it"])
	}
}

// TestIntegration_SectionWorkflow tests init → create sections → set strings → push → pull
// with strings organized by section.
func TestIntegration_SectionWorkflow(t *testing.T) {
	api := newFullAPI()
	srv := api.server()
	defer srv.Close()

	c := client.New("test-key", srv.URL, "proj-int", "env-int")

	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	cfg := workspace.WorkspaceConfig{
		ProjectID: "proj-int",
		ActiveEnv: "env-int",
		Credentials: []workspace.Credential{
			{APIKey: "test-key", BaseURL: srv.URL, EnvID: "env-int", EnvName: "production"},
		},
	}
	workspace.Init(dir, cfg)

	// Create sections and add strings
	workspace.CreateSectionDir(wsDir, "home")
	workspace.CreateSectionDir(wsDir, "settings")

	workspace.SetRows(workspace.CSVPath(wsDir, "home"), "home.welcome", map[string]string{
		"en": "Welcome home!",
		"de": "Willkommen zu Hause!",
	}, "text")

	workspace.SetRows(workspace.CSVPath(wsDir, "home"), "home.greeting", map[string]string{
		"en": "Hello {name}!",
		"de": "Hallo {name}!",
	}, "icu")

	workspace.SetRows(workspace.CSVPath(wsDir, "settings"), "settings.title", map[string]string{
		"en": "Settings",
		"de": "Einstellungen",
	}, "text")

	// Push all
	result, err := workspace.Push(c, wsDir, "", nil)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if result.Upserted != 3 {
		t.Errorf("expected 3 upserted, got %d", result.Upserted)
	}
	if len(result.Sections) != 2 {
		t.Errorf("expected 2 sections, got %d", len(result.Sections))
	}

	// Verify sections were created on server
	if len(api.sections) != 2 {
		t.Errorf("expected 2 sections on server, got %d", len(api.sections))
	}

	// Verify strings have section IDs
	greeting := api.strings["home.greeting"]
	if greeting == nil {
		t.Fatal("home.greeting not found on server")
	}
	if greeting.SectionID == nil || *greeting.SectionID != "sec-home" {
		t.Errorf("expected section ID 'sec-home', got %v", greeting.SectionID)
	}
	if greeting.Format != "icu" {
		t.Errorf("expected format 'icu', got %q", greeting.Format)
	}

	// Pull into fresh workspace
	dir2 := t.TempDir()
	wsDir2 := filepath.Join(dir2, ".airstrings")
	workspace.Init(dir2, cfg)

	pullResult, err := workspace.Pull(c, wsDir2, "")
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if pullResult.FileCount != 2 {
		t.Errorf("expected 2 files (home + settings), got %d", pullResult.FileCount)
	}

	// Verify home section CSV
	homeRows, _ := workspace.ReadCSV(workspace.CSVPath(wsDir2, "home"))
	if len(homeRows) != 4 { // 2 keys × 2 locales
		t.Errorf("expected 4 rows in home, got %d", len(homeRows))
	}

	// Verify ICU format preserved
	for _, r := range homeRows {
		if r.Key == "home.greeting" && r.Format != "icu" {
			t.Errorf("expected icu format for home.greeting, got %q", r.Format)
		}
	}

	// Verify settings section CSV
	settingsRows, _ := workspace.ReadCSV(workspace.CSVPath(wsDir2, "settings"))
	if len(settingsRows) != 2 { // 1 key × 2 locales
		t.Errorf("expected 2 rows in settings, got %d", len(settingsRows))
	}
}

// TestIntegration_PushSingleSection tests pushing only one section
// while other sections exist.
func TestIntegration_PushSingleSection(t *testing.T) {
	api := newFullAPI()
	srv := api.server()
	defer srv.Close()

	c := client.New("test-key", srv.URL, "proj-int", "env-int")

	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	workspace.Init(dir, workspace.WorkspaceConfig{
		ProjectID: "proj-int", ActiveEnv: "env-int", Credentials: []workspace.Credential{{APIKey: "test-key", BaseURL: srv.URL, EnvID: "env-int", EnvName: "production"}},
	})

	// Add strings to two sections
	workspace.SetRows(workspace.CSVPath(wsDir, "home"), "home.title", map[string]string{"en": "Home"}, "text")
	workspace.SetRows(workspace.CSVPath(wsDir, "login"), "login.title", map[string]string{"en": "Login"}, "text")

	// Push only home
	result, err := workspace.Push(c, wsDir, "home", nil)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if result.Upserted != 1 {
		t.Errorf("expected 1 upserted, got %d", result.Upserted)
	}

	// Only home.title should be on server
	if len(api.strings) != 1 {
		t.Errorf("expected 1 string on server, got %d", len(api.strings))
	}
	if _, ok := api.strings["home.title"]; !ok {
		t.Error("expected home.title on server")
	}
	if _, ok := api.strings["login.title"]; ok {
		t.Error("login.title should NOT be on server (only home was pushed)")
	}
}

// TestIntegration_LocalEditCycle tests the local set → edit → remove → ls cycle
// without any API calls.
func TestIntegration_LocalEditCycle(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)

	// Add initial strings
	path := workspace.CSVPath(wsDir, "")
	workspace.SetRows(path, "greeting", map[string]string{
		"en": "Hello",
		"it": "Ciao",
		"fr": "Bonjour",
	}, "text")
	workspace.SetRows(path, "farewell", map[string]string{
		"en": "Goodbye",
		"it": "Arrivederci",
	}, "text")

	// Verify initial state
	rows, _ := workspace.ReadCSV(path)
	if len(rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(rows))
	}

	// Edit: update greeting value
	workspace.SetRows(path, "greeting", map[string]string{"en": "Hi there!"}, "text")

	rows, _ = workspace.ReadCSV(path)
	found := make(map[string]string)
	for _, r := range rows {
		found[r.Key+"/"+r.Locale] = r.Value
	}
	if found["greeting/en"] != "Hi there!" {
		t.Errorf("expected 'Hi there!', got %q", found["greeting/en"])
	}
	// Other locales should be preserved
	if found["greeting/it"] != "Ciao" {
		t.Errorf("expected 'Ciao' preserved, got %q", found["greeting/it"])
	}

	// Edit: change format from text to icu
	workspace.SetRows(path, "greeting", map[string]string{
		"en": "Hello {name}!",
	}, "icu")

	rows, _ = workspace.ReadCSV(path)
	for _, r := range rows {
		if r.Key == "greeting" && r.Locale == "en" {
			if r.Format != "icu" {
				t.Errorf("expected icu format, got %q", r.Format)
			}
			if r.Value != "Hello {name}!" {
				t.Errorf("expected 'Hello {name}!', got %q", r.Value)
			}
		}
	}

	// Remove single locale
	workspace.RemoveRows(path, "greeting", "fr")
	rows, _ = workspace.ReadCSV(path)
	for _, r := range rows {
		if r.Key == "greeting" && r.Locale == "fr" {
			t.Error("French greeting should have been removed")
		}
	}

	// Remove entire key
	workspace.RemoveRows(path, "farewell", "")
	rows, _ = workspace.ReadCSV(path)
	for _, r := range rows {
		if r.Key == "farewell" {
			t.Error("farewell should have been completely removed")
		}
	}

	// Should only have greeting/en and greeting/it left
	if len(rows) != 2 {
		t.Errorf("expected 2 rows remaining, got %d", len(rows))
	}
}

// TestIntegration_UpdateAndRepush tests updating local strings and pushing again
// (the server should have updated values).
func TestIntegration_UpdateAndRepush(t *testing.T) {
	api := newFullAPI()
	srv := api.server()
	defer srv.Close()

	c := client.New("test-key", srv.URL, "proj-int", "env-int")

	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	workspace.Init(dir, workspace.WorkspaceConfig{
		ProjectID: "proj-int", ActiveEnv: "env-int", Credentials: []workspace.Credential{{APIKey: "test-key", BaseURL: srv.URL, EnvID: "env-int", EnvName: "production"}},
	})

	// First push
	workspace.SetRows(workspace.CSVPath(wsDir, ""), "title", map[string]string{"en": "Version 1"}, "text")
	workspace.Push(c, wsDir, "", nil)

	if api.strings["title"].Values["en"] != "Version 1" {
		t.Fatalf("expected 'Version 1' on server")
	}

	// Update locally and push again
	workspace.SetRows(workspace.CSVPath(wsDir, ""), "title", map[string]string{"en": "Version 2"}, "text")
	result, err := workspace.Push(c, wsDir, "", nil)
	if err != nil {
		t.Fatalf("re-push: %v", err)
	}
	if result.Upserted != 1 {
		t.Errorf("expected 1 upserted, got %d", result.Upserted)
	}

	if api.strings["title"].Values["en"] != "Version 2" {
		t.Errorf("expected 'Version 2' on server, got %q", api.strings["title"].Values["en"])
	}
}

// TestIntegration_MixedSectionsAndFlat tests a workspace with both
// sectioned and unsectioned strings.
func TestIntegration_MixedSectionsAndFlat(t *testing.T) {
	api := newFullAPI()
	srv := api.server()
	defer srv.Close()

	c := client.New("test-key", srv.URL, "proj-int", "env-int")

	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	workspace.Init(dir, workspace.WorkspaceConfig{
		ProjectID: "proj-int", ActiveEnv: "env-int", Credentials: []workspace.Credential{{APIKey: "test-key", BaseURL: srv.URL, EnvID: "env-int", EnvName: "production"}},
	})

	// Flat strings (no section)
	workspace.SetRows(workspace.CSVPath(wsDir, ""), "app.name", map[string]string{"en": "My App"}, "text")

	// Section strings
	workspace.SetRows(workspace.CSVPath(wsDir, "onboarding"), "onboarding.step1", map[string]string{"en": "Welcome!"}, "text")

	result, err := workspace.Push(c, wsDir, "", nil)
	if err != nil {
		t.Fatalf("push: %v", err)
	}

	if result.Upserted != 2 {
		t.Errorf("expected 2 upserted, got %d", result.Upserted)
	}

	// Flat string should have no section
	appName := api.strings["app.name"]
	if appName.SectionID != nil {
		t.Errorf("expected nil section ID for flat string, got %v", appName.SectionID)
	}

	// Section string should have section ID
	step1 := api.strings["onboarding.step1"]
	if step1.SectionID == nil || *step1.SectionID != "sec-onboarding" {
		t.Errorf("expected section ID 'sec-onboarding', got %v", step1.SectionID)
	}
}

// TestIntegration_PullSingleSection tests pulling only one section.
func TestIntegration_PullSingleSection(t *testing.T) {
	homeID := "sec-home"
	loginID := "sec-login"
	api := newFullAPI()
	api.sections = []client.Section{
		{ID: homeID, Name: "home"},
		{ID: loginID, Name: "login"},
	}
	api.strings["home.title"] = &client.StringEntry{
		Key: "home.title", Format: "text", SectionID: &homeID,
		Values: map[string]string{"en": "Home", "es": "Inicio"},
	}
	api.strings["login.title"] = &client.StringEntry{
		Key: "login.title", Format: "text", SectionID: &loginID,
		Values: map[string]string{"en": "Login", "es": "Iniciar sesión"},
	}
	srv := api.server()
	defer srv.Close()

	c := client.New("test-key", srv.URL, "proj-int", "env-int")

	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	workspace.Init(dir, workspace.WorkspaceConfig{
		ProjectID: "proj-int", ActiveEnv: "env-int", Credentials: []workspace.Credential{{APIKey: "test-key", BaseURL: srv.URL, EnvID: "env-int", EnvName: "production"}},
	})

	// Pull only home section
	result, err := workspace.Pull(c, wsDir, "home")
	if err != nil {
		t.Fatalf("pull: %v", err)
	}

	if result.FileCount != 1 {
		t.Errorf("expected 1 file, got %d", result.FileCount)
	}

	// Home CSV should exist
	homeRows, _ := workspace.ReadCSV(workspace.CSVPath(wsDir, "home"))
	if len(homeRows) != 2 {
		t.Errorf("expected 2 rows in home, got %d", len(homeRows))
	}

	// Login CSV should NOT exist
	loginRows, _ := workspace.ReadCSV(workspace.CSVPath(wsDir, "login"))
	if len(loginRows) != 0 {
		t.Errorf("expected 0 rows in login (not pulled), got %d", len(loginRows))
	}
}

// TestIntegration_CSVSpecialCharacters tests that values with commas,
// quotes, newlines, and ICU syntax survive the round-trip.
func TestIntegration_CSVSpecialCharacters(t *testing.T) {
	api := newFullAPI()
	srv := api.server()
	defer srv.Close()

	c := client.New("test-key", srv.URL, "proj-int", "env-int")

	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	workspace.Init(dir, workspace.WorkspaceConfig{
		ProjectID: "proj-int", ActiveEnv: "env-int", Credentials: []workspace.Credential{{APIKey: "test-key", BaseURL: srv.URL, EnvID: "env-int", EnvName: "production"}},
	})

	path := workspace.CSVPath(wsDir, "")

	// String with commas
	workspace.SetRows(path, "comma", map[string]string{
		"en": "Hello, world, how are you?",
	}, "text")

	// String with quotes
	workspace.SetRows(path, "quotes", map[string]string{
		"en": `She said "hello" to me`,
	}, "text")

	// ICU MessageFormat string
	workspace.SetRows(path, "plural", map[string]string{
		"en": "{count, plural, one {# item} other {# items}}",
	}, "icu")

	// Verify local round-trip
	rows, err := workspace.ReadCSV(path)
	if err != nil {
		t.Fatalf("read CSV: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	vals := make(map[string]string)
	for _, r := range rows {
		vals[r.Key] = r.Value
	}
	if vals["comma"] != "Hello, world, how are you?" {
		t.Errorf("comma value mangled: %q", vals["comma"])
	}
	if vals["quotes"] != `She said "hello" to me` {
		t.Errorf("quotes value mangled: %q", vals["quotes"])
	}
	if vals["plural"] != "{count, plural, one {# item} other {# items}}" {
		t.Errorf("ICU value mangled: %q", vals["plural"])
	}

	// Push and verify server received correct values
	workspace.Push(c, wsDir, "", nil)
	if api.strings["comma"].Values["en"] != "Hello, world, how are you?" {
		t.Errorf("server comma value: %q", api.strings["comma"].Values["en"])
	}
	if api.strings["quotes"].Values["en"] != `She said "hello" to me` {
		t.Errorf("server quotes value: %q", api.strings["quotes"].Values["en"])
	}
	if api.strings["plural"].Values["en"] != "{count, plural, one {# item} other {# items}}" {
		t.Errorf("server ICU value: %q", api.strings["plural"].Values["en"])
	}
}

// TestIntegration_FindFromSubdirectory tests that workspace.Find() works
// when called from a nested subdirectory.
func TestIntegration_FindFromSubdirectory(t *testing.T) {
	dir := t.TempDir()
	workspace.Init(dir, workspace.WorkspaceConfig{
		ProjectID: "p", ActiveEnv: "e",
	})

	// Create nested subdirectories
	nested := filepath.Join(dir, "src", "components", "ui")
	os.MkdirAll(nested, 0700)

	// Find from deep subdirectory
	wsDir, err := workspace.FindFrom(nested)
	if err != nil {
		t.Fatalf("FindFrom nested: %v", err)
	}

	expected := filepath.Join(dir, ".airstrings")
	if wsDir != expected {
		t.Errorf("expected %q, got %q", expected, wsDir)
	}

	// Load config to verify it works
	cfg, err := workspace.LoadConfig(wsDir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.ProjectID != "p" {
		t.Errorf("expected project ID 'p', got %q", cfg.ProjectID)
	}
}

// TestIntegration_DetectModeTransitions tests the workspace mode detection
// as the workspace evolves.
func TestIntegration_DetectModeTransitions(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)

	// Empty workspace
	mode := workspace.DetectMode(wsDir)
	if mode != "empty" {
		t.Errorf("expected 'empty', got %q", mode)
	}

	// Add flat strings → flat mode
	workspace.SetRows(workspace.CSVPath(wsDir, ""), "key", map[string]string{"en": "val"}, "text")
	mode = workspace.DetectMode(wsDir)
	if mode != "flat" {
		t.Errorf("expected 'flat', got %q", mode)
	}

	// Add a section → sections mode
	workspace.CreateSectionDir(wsDir, "home")
	workspace.SetRows(workspace.CSVPath(wsDir, "home"), "key", map[string]string{"en": "val"}, "text")
	mode = workspace.DetectMode(wsDir)
	if mode != "sections" {
		t.Errorf("expected 'sections', got %q", mode)
	}
}
