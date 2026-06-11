package workspace

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
	"testing"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
)

// testServer creates an httptest server with handlers for the API endpoints
// used by push and pull.
type testAPI struct {
	sections []client.Section
	strings  []client.StringEntry
	// imported tracks keys received via the import endpoint
	imported       map[string]map[string]string // key -> locale -> value
	upserts        map[string]client.UpsertStringRequest
	deleted        []string
	sectionAssigns map[string]string // key -> section ID
	mux            *http.ServeMux
}

func newTestAPI() *testAPI {
	api := &testAPI{
		imported:       make(map[string]map[string]string),
		upserts:        make(map[string]client.UpsertStringRequest),
		sectionAssigns: make(map[string]string),
		mux:            http.NewServeMux(),
	}
	api.mux.HandleFunc("/v1/projects/proj/environments/env/sections", api.handleSections)
	api.mux.HandleFunc("/v1/projects/proj/environments/env/strings/", api.handleStringByKey)
	api.mux.HandleFunc("/v1/projects/proj/environments/env/strings", api.handleStrings)
	api.mux.HandleFunc("/v1/projects/proj/environments/env/imports", api.handleImportCreate)
	api.mux.HandleFunc("/v1/projects/proj/environments/env/imports/", api.handleImportStatus)
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
	}
}

func (a *testAPI) handleStringByKey(w http.ResponseWriter, r *http.Request) {
	if filepath.Base(r.URL.Path) == "section" {
		key := filepath.Base(filepath.Dir(r.URL.Path))
		var req client.AssignSectionRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.SectionID != nil {
			a.sectionAssigns[key] = *req.SectionID
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	key := filepath.Base(r.URL.Path)
	switch r.Method {
	case "PUT":
		var req client.UpsertStringRequest
		json.NewDecoder(r.Body).Decode(&req)
		a.upserts[key] = req
		entry := client.StringEntry{Key: key, Format: req.Format, Values: make(map[string]string)}
		for loc, v := range req.Values {
			if v != nil {
				entry.Values[loc] = *v
			}
		}
		json.NewEncoder(w).Encode(entry)
	case "DELETE":
		a.deleted = append(a.deleted, key)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (a *testAPI) handleImportCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "missing file"})
		return
	}
	defer file.Close()

	data, _ := io.ReadAll(file)

	// Auto-detect gzip
	var reader io.Reader
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		gr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "gzip: " + err.Error()})
			return
		}
		defer gr.Close()
		decompressed, _ := io.ReadAll(gr)
		reader = bytes.NewReader(decompressed)
	} else {
		reader = bytes.NewReader(data)
	}

	// Parse CSV
	csvReader := csv.NewReader(reader)
	records, _ := csvReader.ReadAll()

	totalRows := 0
	if len(records) > 1 { // skip header
		for _, rec := range records[1:] {
			key := rec[0]
			locale := rec[1]
			value := rec[2]
			if a.imported[key] == nil {
				a.imported[key] = make(map[string]string)
			}
			a.imported[key][locale] = value
			totalRows++
		}
	}

	// Return completed import immediately (test simplification)
	resp := client.ImportStatus{
		ID:          "imp_test123",
		Status:      "completed",
		TotalRows:   totalRows,
		CreatedRows: totalRows,
	}
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(resp)
}

func (a *testAPI) handleImportStatus(w http.ResponseWriter, r *http.Request) {
	resp := client.ImportStatus{
		ID:          "imp_test123",
		Status:      "completed",
		TotalRows:   len(a.imported),
		CreatedRows: len(a.imported),
	}
	json.NewEncoder(w).Encode(resp)
}

func (a *testAPI) server() *httptest.Server {
	return httptest.NewServer(a.mux)
}

func TestPush_FlatStrings(t *testing.T) {
	api := newTestAPI()
	srv := api.server()
	defer srv.Close()

	c := client.New("key", srv.URL, "proj", "env")

	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)

	SetRows(CSVPath(wsDir, ""), "greeting", map[string]string{"en": "Hello", "it": "Ciao"}, "text")
	SetRows(CSVPath(wsDir, ""), "farewell", map[string]string{"en": "Bye"}, "text")

	result, err := Push(c, wsDir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Upserted != 2 { // 2 unique keys: greeting + farewell
		t.Errorf("expected 2 upserted keys, got %d", result.Upserted)
	}
}

func TestPush_WithSections(t *testing.T) {
	api := newTestAPI()
	srv := api.server()
	defer srv.Close()

	c := client.New("key", srv.URL, "proj", "env")

	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")

	SetRows(CSVPath(wsDir, "home"), "welcome", map[string]string{"en": "Welcome"}, "text")
	SetRows(CSVPath(wsDir, "login"), "email", map[string]string{"en": "Email"}, "text")

	result, err := Push(c, wsDir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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

	if result.Upserted != 1 {
		t.Errorf("expected 1 upserted, got %d", result.Upserted)
	}
	if len(result.Sections) != 1 {
		t.Errorf("expected 1 section, got %d", len(result.Sections))
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

	homeRows, _ := ReadCSV(CSVPath(wsDir, "home"))
	if len(homeRows) != 1 {
		t.Errorf("expected 1 row in home, got %d", len(homeRows))
	}
	if homeRows[0].Key != "welcome" {
		t.Errorf("expected 'welcome' key, got %q", homeRows[0].Key)
	}

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

func TestPushKey_UpsertsValuesAndFormat(t *testing.T) {
	api := newTestAPI()
	srv := api.server()
	defer srv.Close()

	c := client.New("key", srv.URL, "proj", "env")

	err := PushKey(c, "greeting", map[string]string{"en": "Hello", "it": "Ciao"}, "icu", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req, ok := api.upserts["greeting"]
	if !ok {
		t.Fatal("expected PUT upsert for 'greeting'")
	}
	if req.Format != "icu" {
		t.Errorf("expected format 'icu', got %q", req.Format)
	}
	if len(req.Values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(req.Values))
	}
	if req.Values["en"] == nil || *req.Values["en"] != "Hello" {
		t.Errorf("expected en=Hello, got %v", req.Values["en"])
	}
	if req.Values["it"] == nil || *req.Values["it"] != "Ciao" {
		t.Errorf("expected it=Ciao, got %v", req.Values["it"])
	}
	if len(api.sectionAssigns) != 0 {
		t.Errorf("expected no section assignment, got %v", api.sectionAssigns)
	}
}

func TestPushKey_CreatesSectionWhenMissing(t *testing.T) {
	api := newTestAPI()
	srv := api.server()
	defer srv.Close()

	c := client.New("key", srv.URL, "proj", "env")

	err := PushKey(c, "welcome", map[string]string{"en": "Welcome"}, "text", "home")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(api.sections) != 1 || api.sections[0].Name != "home" {
		t.Fatalf("expected section 'home' created, got %v", api.sections)
	}
	if api.sectionAssigns["welcome"] != "sec-home" {
		t.Errorf("expected 'welcome' assigned to 'sec-home', got %q", api.sectionAssigns["welcome"])
	}
}

func TestPushKey_ReusesExistingSection(t *testing.T) {
	api := newTestAPI()
	api.sections = []client.Section{
		{ID: "sec-existing", Name: "home"},
	}
	srv := api.server()
	defer srv.Close()

	c := client.New("key", srv.URL, "proj", "env")

	err := PushKey(c, "welcome", map[string]string{"en": "Welcome"}, "text", "home")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(api.sections) != 1 {
		t.Fatalf("expected no new section, got %v", api.sections)
	}
	if api.sectionAssigns["welcome"] != "sec-existing" {
		t.Errorf("expected 'welcome' assigned to 'sec-existing', got %q", api.sectionAssigns["welcome"])
	}
}

func TestPushKeyRemoval_FullKey(t *testing.T) {
	api := newTestAPI()
	srv := api.server()
	defer srv.Close()

	c := client.New("key", srv.URL, "proj", "env")

	if err := PushKeyRemoval(c, "old.key", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(api.deleted) != 1 || api.deleted[0] != "old.key" {
		t.Errorf("expected DELETE for 'old.key', got %v", api.deleted)
	}
	if len(api.upserts) != 0 {
		t.Errorf("expected no upserts, got %v", api.upserts)
	}
}

func TestPushKeyRemoval_SingleLocale(t *testing.T) {
	api := newTestAPI()
	srv := api.server()
	defer srv.Close()

	c := client.New("key", srv.URL, "proj", "env")

	if err := PushKeyRemoval(c, "greeting", "it"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(api.deleted) != 0 {
		t.Errorf("expected no deletes, got %v", api.deleted)
	}
	req, ok := api.upserts["greeting"]
	if !ok {
		t.Fatal("expected PUT upsert for 'greeting'")
	}
	v, present := req.Values["it"]
	if !present {
		t.Fatal("expected 'it' locale in upsert values")
	}
	if v != nil {
		t.Errorf("expected nil value for 'it', got %q", *v)
	}
}

func TestSetThenPushKey_WritesCSVAndPushes(t *testing.T) {
	api := newTestAPI()
	srv := api.server()
	defer srv.Close()

	c := client.New("key", srv.URL, "proj", "env")

	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")

	values := map[string]string{"en": "Welcome", "it": "Benvenuto"}
	path := CSVPath(wsDir, "home")
	if err := SetRows(path, "welcome", values, "text"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := PushKey(c, "welcome", values, "text", "home"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rows, err := ReadCSV(path)
	if err != nil {
		t.Fatalf("read CSV error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	for _, r := range rows {
		if r.Key != "welcome" || r.Format != "text" {
			t.Errorf("unexpected row: %+v", r)
		}
	}

	if _, ok := api.upserts["welcome"]; !ok {
		t.Error("expected PUT upsert for 'welcome'")
	}
	if api.sectionAssigns["welcome"] != "sec-home" {
		t.Errorf("expected 'welcome' assigned to 'sec-home', got %q", api.sectionAssigns["welcome"])
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
