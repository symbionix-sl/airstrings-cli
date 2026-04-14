package workspace

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/symbionix/airstrings-cli/internal/client"
)

// testServer creates an httptest server with handlers for the API endpoints
// used by push and pull.
type testAPI struct {
	sections []client.Section
	strings  []client.StringEntry
	upserted map[string]client.UpsertStringRequest
	created  map[string]client.CreateStringRequest
	mux      *http.ServeMux
}

func newTestAPI() *testAPI {
	api := &testAPI{
		upserted: make(map[string]client.UpsertStringRequest),
		created:  make(map[string]client.CreateStringRequest),
		mux:      http.NewServeMux(),
	}
	api.mux.HandleFunc("/v1/projects/proj/environments/env/sections", api.handleSections)
	api.mux.HandleFunc("/v1/projects/proj/environments/env/strings/", api.handleStringByKey)
	api.mux.HandleFunc("/v1/projects/proj/environments/env/strings", api.handleStrings)
	return api
}

func (a *testAPI) handleSections(w http.ResponseWriter, r *http.Request) {
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

func (a *testAPI) handleStrings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		resp := client.StringList{
			Data:       a.strings,
			Pagination: client.PaginationMeta{HasMore: false},
		}
		json.NewEncoder(w).Encode(resp)
	case "POST":
		var req client.CreateStringRequest
		json.NewDecoder(r.Body).Decode(&req)
		a.created[req.Key] = req
		entry := client.StringEntry{Key: req.Key, Format: req.Format, Values: req.Values, SectionID: req.SectionID}
		json.NewEncoder(w).Encode(entry)
	}
}

func (a *testAPI) handleStringByKey(w http.ResponseWriter, r *http.Request) {
	// Handle section assignment: /strings/{key}/section
	if filepath.Base(r.URL.Path) == "section" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Extract key from path: /v1/projects/proj/environments/env/strings/<key>
	key := filepath.Base(r.URL.Path)

	switch r.Method {
	case "PUT":
		var req client.UpsertStringRequest
		json.NewDecoder(r.Body).Decode(&req)
		a.upserted[key] = req
		// Convert *string values to string values for the response
		vals := make(map[string]string)
		for k, v := range req.Values {
			if v != nil {
				vals[k] = *v
			}
		}
		entry := client.StringEntry{Key: key, Format: req.Format, Values: vals}
		json.NewEncoder(w).Encode(entry)
	}
}

func (a *testAPI) server() *httptest.Server {
	return httptest.NewServer(a.mux)
}

func TestPush_FlatStrings(t *testing.T) {
	api := newTestAPI()
	srv := api.server()
	defer srv.Close()

	c := client.New("key", srv.URL, "proj", "env")

	// Set up workspace with flat strings
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)

	SetRows(CSVPath(wsDir, ""), "greeting", map[string]string{"en": "Hello", "it": "Ciao"}, "text")
	SetRows(CSVPath(wsDir, ""), "farewell", map[string]string{"en": "Bye"}, "text")

	result, err := Push(c, wsDir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(api.upserted) != 2 {
		t.Errorf("expected 2 upserted keys, got %d", len(api.upserted))
	}
	if _, ok := api.upserted["greeting"]; !ok {
		t.Error("expected 'greeting' to be upserted")
	}
	if _, ok := api.upserted["farewell"]; !ok {
		t.Error("expected 'farewell' to be upserted")
	}
	if result.Upserted != 2 {
		t.Errorf("expected 2 upserted, got %d", result.Upserted)
	}
}

func TestPush_WithSections(t *testing.T) {
	api := newTestAPI()
	srv := api.server()
	defer srv.Close()

	c := client.New("key", srv.URL, "proj", "env")

	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")

	// Create section CSVs
	SetRows(CSVPath(wsDir, "home"), "welcome", map[string]string{"en": "Welcome"}, "text")
	SetRows(CSVPath(wsDir, "login"), "email", map[string]string{"en": "Email"}, "text")

	result, err := Push(c, wsDir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have created sections
	if len(api.sections) != 2 {
		t.Errorf("expected 2 sections created, got %d", len(api.sections))
	}

	if len(result.Sections) != 2 {
		t.Errorf("expected 2 sections in result, got %d", len(result.Sections))
	}
}

func TestPush_SingleSection(t *testing.T) {
	api := newTestAPI()
	srv := api.server()
	defer srv.Close()

	c := client.New("key", srv.URL, "proj", "env")

	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")

	SetRows(CSVPath(wsDir, "home"), "welcome", map[string]string{"en": "Welcome"}, "text")
	SetRows(CSVPath(wsDir, "login"), "email", map[string]string{"en": "Email"}, "text")

	// Push only "home" section
	result, err := Push(c, wsDir, "home", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(api.upserted) != 1 {
		t.Errorf("expected 1 upserted, got %d", len(api.upserted))
	}
	if _, ok := api.upserted["welcome"]; !ok {
		t.Error("expected 'welcome' to be upserted")
	}
	if result.Upserted != 1 {
		t.Errorf("expected 1 upserted, got %d", result.Upserted)
	}
}

func TestPush_EmptyWorkspace(t *testing.T) {
	api := newTestAPI()
	srv := api.server()
	defer srv.Close()

	c := client.New("key", srv.URL, "proj", "env")

	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)

	result, err := Push(c, wsDir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Upserted != 0 {
		t.Errorf("expected 0 upserted, got %d", result.Upserted)
	}
}

func TestPush_ExistingSectionReuse(t *testing.T) {
	api := newTestAPI()
	// Pre-populate with existing section
	api.sections = []client.Section{
		{ID: "sec-existing", Name: "home"},
	}
	srv := api.server()
	defer srv.Close()

	c := client.New("key", srv.URL, "proj", "env")

	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	SetRows(CSVPath(wsDir, "home"), "welcome", map[string]string{"en": "Welcome"}, "text")

	_, err := Push(c, wsDir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should NOT have created a new section (only 1 should exist still)
	if len(api.sections) != 1 {
		t.Errorf("expected 1 section (reused), got %d", len(api.sections))
	}
}

func TestPull_FlatStrings(t *testing.T) {
	api := newTestAPI()
	api.strings = []client.StringEntry{
		{Key: "greeting", Format: "text", Values: map[string]string{"en": "Hello", "it": "Ciao"}},
		{Key: "farewell", Format: "text", Values: map[string]string{"en": "Bye"}},
	}
	srv := api.server()
	defer srv.Close()

	c := client.New("key", srv.URL, "proj", "env")

	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)

	result, err := Pull(c, wsDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.StringCount != 2 {
		t.Errorf("expected 2 strings, got %d", result.StringCount)
	}

	// Verify flat CSV was written
	rows, err := ReadCSV(CSVPath(wsDir, ""))
	if err != nil {
		t.Fatalf("read CSV error: %v", err)
	}
	if len(rows) != 3 { // greeting(en,it) + farewell(en)
		t.Errorf("expected 3 rows, got %d", len(rows))
	}
}

func TestPull_WithSections(t *testing.T) {
	homeID := "sec-home"
	api := newTestAPI()
	api.sections = []client.Section{
		{ID: homeID, Name: "home"},
	}
	api.strings = []client.StringEntry{
		{Key: "welcome", Format: "text", SectionID: &homeID, Values: map[string]string{"en": "Welcome"}},
		{Key: "farewell", Format: "text", Values: map[string]string{"en": "Bye"}},
	}
	srv := api.server()
	defer srv.Close()

	c := client.New("key", srv.URL, "proj", "env")

	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)

	result, err := Pull(c, wsDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.FileCount != 2 {
		t.Errorf("expected 2 files, got %d", result.FileCount)
	}

	// Verify section CSV
	homeRows, _ := ReadCSV(CSVPath(wsDir, "home"))
	if len(homeRows) != 1 {
		t.Errorf("expected 1 row in home, got %d", len(homeRows))
	}
	if homeRows[0].Key != "welcome" {
		t.Errorf("expected 'welcome' key, got %q", homeRows[0].Key)
	}

	// Verify flat CSV (unsectioned string)
	flatRows, _ := ReadCSV(CSVPath(wsDir, ""))
	if len(flatRows) != 1 {
		t.Errorf("expected 1 row in flat, got %d", len(flatRows))
	}
}

func TestPull_EmptyProject(t *testing.T) {
	api := newTestAPI()
	srv := api.server()
	defer srv.Close()

	c := client.New("key", srv.URL, "proj", "env")

	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)

	result, err := Pull(c, wsDir, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StringCount != 0 {
		t.Errorf("expected 0 strings, got %d", result.StringCount)
	}
	if result.FileCount != 0 {
		t.Errorf("expected 0 files, got %d", result.FileCount)
	}
}

func TestEnsureSection_ExistingSection(t *testing.T) {
	api := newTestAPI()
	api.sections = []client.Section{
		{ID: "sec-abc", Name: "home"},
	}
	srv := api.server()
	defer srv.Close()

	c := client.New("key", srv.URL, "proj", "env")

	id, err := EnsureSection(c, "home")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "sec-abc" {
		t.Errorf("expected 'sec-abc', got %q", id)
	}
}

func TestEnsureSection_CreatesNew(t *testing.T) {
	api := newTestAPI()
	srv := api.server()
	defer srv.Close()

	c := client.New("key", srv.URL, "proj", "env")

	id, err := EnsureSection(c, "newone")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "sec-newone" {
		t.Errorf("expected 'sec-newone', got %q", id)
	}
	if len(api.sections) != 1 {
		t.Errorf("expected 1 section created, got %d", len(api.sections))
	}
}
