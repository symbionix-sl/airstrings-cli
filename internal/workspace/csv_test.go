package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCSVPath(t *testing.T) {
	tests := []struct {
		name    string
		wsDir   string
		section string
		want    string
	}{
		{"flat mode", "/ws", "", "/ws/strings.csv"},
		{"section mode", "/ws", "home", "/ws/home/home.csv"},
		{"section with spaces", "/ws", "my section", "/ws/my section/my section.csv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CSVPath(tt.wsDir, tt.section)
			if got != tt.want {
				t.Errorf("CSVPath(%q, %q) = %q, want %q", tt.wsDir, tt.section, got, tt.want)
			}
		})
	}
}

func TestReadCSV_NonExistentFile(t *testing.T) {
	rows, err := ReadCSV("/nonexistent/path.csv")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected empty slice, got %d rows", len(rows))
	}
}

func TestReadCSV_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")
	os.WriteFile(path, []byte(""), 0600)

	rows, err := ReadCSV(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected empty slice, got %d rows", len(rows))
	}
}

func TestReadCSV_HeaderOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")
	os.WriteFile(path, []byte("key,locale,value,format\n"), 0600)

	rows, err := ReadCSV(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected empty slice, got %d rows", len(rows))
	}
}

func TestReadCSV_ThreeColumns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")
	os.WriteFile(path, []byte("key,locale,value\ngreeting,en,Hello\n"), 0600)

	rows, err := ReadCSV(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Key != "greeting" || rows[0].Locale != "en" || rows[0].Value != "Hello" || rows[0].Format != "text" {
		t.Errorf("unexpected row: %+v", rows[0])
	}
}

func TestReadCSV_FourColumns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")
	content := "key,locale,value,format\ngreeting,en,Hello,text\nmsg,en,\"{name} has {count, plural, one {# item} other {# items}}\",icu\n"
	os.WriteFile(path, []byte(content), 0600)

	rows, err := ReadCSV(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[1].Format != "icu" {
		t.Errorf("expected icu format, got %q", rows[1].Format)
	}
}

func TestReadCSV_QuotedValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")
	// Value with comma and newline
	content := "key,locale,value,format\ngreeting,en,\"Hello, world\",text\n"
	os.WriteFile(path, []byte(content), 0600)

	rows, err := ReadCSV(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Value != "Hello, world" {
		t.Errorf("expected 'Hello, world', got %q", rows[0].Value)
	}
}

func TestWriteCSV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "test.csv")

	rows := []Row{
		{Key: "greeting", Locale: "en", Value: "Hello", Format: "text"},
		{Key: "greeting", Locale: "it", Value: "Ciao", Format: "text"},
	}
	if err := WriteCSV(path, rows); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file exists and has correct permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected 0600 permissions, got %o", perm)
	}

	// Verify parent dir permissions
	dirInfo, _ := os.Stat(filepath.Dir(path))
	if perm := dirInfo.Mode().Perm(); perm != 0700 {
		t.Errorf("expected 0700 dir permissions, got %o", perm)
	}

	// Read back and verify
	readBack, err := ReadCSV(path)
	if err != nil {
		t.Fatalf("read back error: %v", err)
	}
	if len(readBack) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(readBack))
	}
	if readBack[0].Key != "greeting" || readBack[0].Locale != "en" || readBack[0].Value != "Hello" {
		t.Errorf("unexpected first row: %+v", readBack[0])
	}
}

func TestWriteCSV_EmptyRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")

	if err := WriteCSV(path, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have header only
	readBack, err := ReadCSV(path)
	if err != nil {
		t.Fatalf("read back error: %v", err)
	}
	if len(readBack) != 0 {
		t.Errorf("expected 0 rows, got %d", len(readBack))
	}
}

func TestWriteCSV_DefaultFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")

	rows := []Row{{Key: "k", Locale: "en", Value: "v", Format: ""}}
	WriteCSV(path, rows)

	readBack, _ := ReadCSV(path)
	if readBack[0].Format != "text" {
		t.Errorf("expected default format 'text', got %q", readBack[0].Format)
	}
}

func TestSetRows_InsertNew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")

	err := SetRows(path, "greeting", map[string]string{"en": "Hello", "it": "Ciao"}, "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rows, _ := ReadCSV(path)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	byLocale := make(map[string]Row)
	for _, r := range rows {
		byLocale[r.Locale] = r
	}
	if byLocale["en"].Value != "Hello" {
		t.Errorf("expected Hello, got %q", byLocale["en"].Value)
	}
	if byLocale["it"].Value != "Ciao" {
		t.Errorf("expected Ciao, got %q", byLocale["it"].Value)
	}
}

func TestSetRows_UpdateExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")

	// Create initial row
	SetRows(path, "greeting", map[string]string{"en": "Hello"}, "text")

	// Update it
	SetRows(path, "greeting", map[string]string{"en": "Hi there"}, "text")

	rows, _ := ReadCSV(path)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (updated), got %d", len(rows))
	}
	if rows[0].Value != "Hi there" {
		t.Errorf("expected 'Hi there', got %q", rows[0].Value)
	}
}

func TestSetRows_UpdateFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")

	SetRows(path, "msg", map[string]string{"en": "{count} items"}, "text")
	SetRows(path, "msg", map[string]string{"en": "{count, plural, one {# item} other {# items}}"}, "icu")

	rows, _ := ReadCSV(path)
	if rows[0].Format != "icu" {
		t.Errorf("expected icu, got %q", rows[0].Format)
	}
}

func TestSetRows_AddLocaleToExistingKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")

	SetRows(path, "greeting", map[string]string{"en": "Hello"}, "text")
	SetRows(path, "greeting", map[string]string{"it": "Ciao"}, "text")

	rows, _ := ReadCSV(path)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestSetRows_MultipleLocalesAtOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")

	err := SetRows(path, "greeting", map[string]string{
		"en": "Hello",
		"it": "Ciao",
		"fr": "Bonjour",
		"de": "Hallo",
		"es": "Hola",
	}, "text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rows, _ := ReadCSV(path)
	if len(rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(rows))
	}
}

func TestSetRows_PreservesOtherKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")

	SetRows(path, "greeting", map[string]string{"en": "Hello"}, "text")
	SetRows(path, "farewell", map[string]string{"en": "Bye"}, "text")

	rows, _ := ReadCSV(path)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestRemoveRows_AllLocales(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")

	SetRows(path, "greeting", map[string]string{"en": "Hello", "it": "Ciao"}, "text")
	SetRows(path, "farewell", map[string]string{"en": "Bye"}, "text")

	err := RemoveRows(path, "greeting", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rows, _ := ReadCSV(path)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row remaining, got %d", len(rows))
	}
	if rows[0].Key != "farewell" {
		t.Errorf("expected farewell, got %q", rows[0].Key)
	}
}

func TestRemoveRows_SingleLocale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")

	SetRows(path, "greeting", map[string]string{"en": "Hello", "it": "Ciao"}, "text")

	err := RemoveRows(path, "greeting", "it")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rows, _ := ReadCSV(path)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row remaining, got %d", len(rows))
	}
	if rows[0].Locale != "en" {
		t.Errorf("expected en locale remaining, got %q", rows[0].Locale)
	}
}

func TestRemoveRows_NonExistentKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.csv")

	SetRows(path, "greeting", map[string]string{"en": "Hello"}, "text")

	// Should not error
	err := RemoveRows(path, "nonexistent", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rows, _ := ReadCSV(path)
	if len(rows) != 1 {
		t.Errorf("expected 1 row unchanged, got %d", len(rows))
	}
}

func TestRemoveRows_NonExistentFile(t *testing.T) {
	err := RemoveRows("/nonexistent/path.csv", "key", "")
	if err != nil {
		t.Fatalf("expected no error for non-existent file, got: %v", err)
	}
}

func TestAllCSVPaths_Empty(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)

	paths, err := AllCSVPaths(wsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected empty map, got %d entries", len(paths))
	}
}

func TestAllCSVPaths_FlatOnly(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)
	os.WriteFile(filepath.Join(wsDir, "strings.csv"), []byte("key,locale,value,format\n"), 0600)

	paths, err := AllCSVPaths(wsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(paths))
	}
	if _, ok := paths[""]; !ok {
		t.Error("expected empty string key for flat mode")
	}
}

func TestAllCSVPaths_Sections(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")

	// Create section dirs with CSVs
	for _, sec := range []string{"home", "login", "settings"} {
		secDir := filepath.Join(wsDir, sec)
		os.MkdirAll(secDir, 0700)
		os.WriteFile(filepath.Join(secDir, sec+".csv"), []byte("key,locale,value,format\n"), 0600)
	}

	paths, err := AllCSVPaths(wsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(paths))
	}
	for _, sec := range []string{"home", "login", "settings"} {
		if _, ok := paths[sec]; !ok {
			t.Errorf("expected section %q in paths", sec)
		}
	}
}

func TestAllCSVPaths_Mixed(t *testing.T) {
	dir := t.TempDir()
	wsDir := filepath.Join(dir, ".airstrings")
	os.MkdirAll(wsDir, 0700)

	// Flat strings.csv
	os.WriteFile(filepath.Join(wsDir, "strings.csv"), []byte("key,locale,value,format\n"), 0600)

	// Section dir
	secDir := filepath.Join(wsDir, "home")
	os.MkdirAll(secDir, 0700)
	os.WriteFile(filepath.Join(secDir, "home.csv"), []byte("key,locale,value,format\n"), 0600)

	paths, err := AllCSVPaths(wsDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(paths))
	}
	if _, ok := paths[""]; !ok {
		t.Error("expected flat entry")
	}
	if _, ok := paths["home"]; !ok {
		t.Error("expected home section entry")
	}
}
