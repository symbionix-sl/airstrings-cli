package workspace

import (
	"encoding/csv"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// Row represents one row in a workspace CSV file.
type Row struct {
	Key    string
	Locale string
	Value  string
	Format string // "text" (default) or "icu"
}

// CSVPath returns the file path for a section's CSV.
// If section is empty, returns the flat strings.csv path.
func CSVPath(wsDir, section string) string {
	if section == "" {
		return filepath.Join(wsDir, "strings.csv")
	}
	return filepath.Join(wsDir, section, section+".csv")
}

// AllCSVPaths scans the workspace directory and returns a map of
// section name to CSV file path. The empty string key ("") represents
// the flat strings.csv file (no section).
func AllCSVPaths(wsDir string) (map[string]string, error) {
	paths := make(map[string]string)

	// Check for flat strings.csv
	flat := filepath.Join(wsDir, "strings.csv")
	if _, err := os.Stat(flat); err == nil {
		paths[""] = flat
	}

	// Scan for section directories
	entries, err := os.ReadDir(wsDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		csvFile := filepath.Join(wsDir, name, name+".csv")
		if _, err := os.Stat(csvFile); err == nil {
			paths[name] = csvFile
		}
	}

	return paths, nil
}

// ReadCSV parses a CSV file and returns its rows.
// Returns an empty slice if the file does not exist.
// Missing format column defaults to "text".
func ReadCSV(path string) ([]Row, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // allow variable field count

	var rows []Row
	header := true
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if header {
			header = false
			continue
		}
		if len(record) < 3 {
			continue // skip malformed rows
		}
		format := "text"
		if len(record) >= 4 && record[3] != "" {
			format = record[3]
		}
		rows = append(rows, Row{
			Key:    record[0],
			Locale: record[1],
			Value:  record[2],
			Format: format,
		})
	}
	return rows, nil
}

// WriteCSV writes rows to a CSV file with the header key,locale,value,format.
// Creates parent directories (0700) and sets file permissions to 0600.
// Empty format defaults to "text".
func WriteCSV(path string, rows []Row) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write([]string{"key", "locale", "value", "format"}); err != nil {
		return err
	}

	for _, row := range rows {
		format := row.Format
		if format == "" {
			format = "text"
		}
		if err := w.Write([]string{row.Key, row.Locale, row.Value, format}); err != nil {
			return err
		}
	}

	return nil
}

// SetRows upserts rows for a key in a CSV file.
// For each locale in values, if a row with that key+locale exists, it is updated.
// Otherwise, a new row is appended. Creates the file if it doesn't exist.
func SetRows(path, key string, values map[string]string, format string) error {
	if format == "" {
		format = "text"
	}

	existing, err := ReadCSV(path)
	if err != nil {
		return err
	}

	// Track which locales we've updated
	updated := make(map[string]bool)

	// Update existing rows
	for i, row := range existing {
		if row.Key != key {
			continue
		}
		if val, ok := values[row.Locale]; ok {
			existing[i].Value = val
			existing[i].Format = format
			updated[row.Locale] = true
		}
	}

	// Append new locales (sorted for deterministic output)
	locales := make([]string, 0, len(values))
	for loc := range values {
		if !updated[loc] {
			locales = append(locales, loc)
		}
	}
	sort.Strings(locales)

	for _, loc := range locales {
		existing = append(existing, Row{
			Key:    key,
			Locale: loc,
			Value:  values[loc],
			Format: format,
		})
	}

	return WriteCSV(path, existing)
}

// RemoveRows removes rows from a CSV file.
// If locale is empty, all rows for the key are removed.
// If locale is specified, only the row for that key+locale is removed.
// No error if the key doesn't exist or the file doesn't exist.
func RemoveRows(path, key, locale string) error {
	existing, err := ReadCSV(path)
	if err != nil {
		return err
	}
	if len(existing) == 0 {
		return nil
	}

	var kept []Row
	for _, row := range existing {
		if row.Key == key {
			if locale == "" || row.Locale == locale {
				continue // remove this row
			}
		}
		kept = append(kept, row)
	}

	return WriteCSV(path, kept)
}
