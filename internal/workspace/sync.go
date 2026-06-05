package workspace

import (
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"sort"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
)

// PushError records a single key that failed to import.
type PushError struct {
	Key     string `json:"key"`
	Message string `json:"message"`
}

// PushResult summarizes the outcome of a push operation.
type PushResult struct {
	Upserted   int         `json:"upserted"`
	Errors     int         `json:"errors"`
	Sections   []string    `json:"sections"`
	FailedKeys []PushError `json:"failed_keys,omitempty"`
}

// PullResult summarizes the outcome of a pull operation.
type PullResult struct {
	StringCount int
	Sections    []string
	FileCount   int
}

// ProgressFunc is called during push with the current phase and progress.
// Phase is "uploading" or "processing".
type ProgressFunc func(phase string, done, total int)

// Push reads local CSVs, builds a single CSV with a section column,
// gzips it, and uploads via the import endpoint.
// If section is non-empty, only that section is pushed.
func Push(c *client.Client, wsDir, section string, onProgress ProgressFunc) (*PushResult, error) {
	// Determine which CSVs to push
	var csvMap map[string]string
	if section != "" {
		path := CSVPath(wsDir, section)
		csvMap = map[string]string{section: path}
	} else {
		var err error
		csvMap, err = AllCSVPaths(wsDir)
		if err != nil {
			return nil, fmt.Errorf("scan workspace: %w", err)
		}
	}

	result := &PushResult{}

	// Build a single CSV buffer with section column from all CSVs
	var allBuf bytes.Buffer
	w := csv.NewWriter(&allBuf)

	// Write header
	if err := w.Write([]string{"key", "locale", "value", "format", "section"}); err != nil {
		return nil, fmt.Errorf("write csv header: %w", err)
	}

	totalRows := 0
	for secName, csvPath := range csvMap {
		rows, err := ReadCSV(csvPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", csvPath, err)
		}
		if len(rows) == 0 {
			continue
		}

		if secName != "" {
			result.Sections = append(result.Sections, secName)
		}

		for _, row := range rows {
			format := row.Format
			if format == "" {
				format = "text"
			}
			if err := w.Write([]string{row.Key, row.Locale, row.Value, format, secName}); err != nil {
				return nil, fmt.Errorf("write csv row: %w", err)
			}
			totalRows++
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("flush csv: %w", err)
	}

	if totalRows == 0 {
		sort.Strings(result.Sections)
		return result, nil
	}

	// Gzip the CSV
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write(allBuf.Bytes()); err != nil {
		return nil, fmt.Errorf("gzip write: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("gzip close: %w", err)
	}

	// Upload
	if onProgress != nil {
		onProgress("uploading", 0, 1)
	}

	status, err := c.CreateImport(compressed.Bytes(), nil)
	if err != nil {
		return nil, fmt.Errorf("create import: %w", err)
	}

	if onProgress != nil {
		onProgress("uploading", 1, 1)
	}

	// Poll until complete
	var pollCallback func(*client.ImportStatus)
	if onProgress != nil {
		pollCallback = func(s *client.ImportStatus) {
			onProgress("processing", s.ProcessedRows, s.TotalRows)
		}
	}

	final, err := c.WaitForImport(status.ID, pollCallback)
	if err != nil {
		// Even on failure, try to populate result from whatever we have
		if final != nil {
			result.Upserted = final.CreatedRows + final.UpdatedRows
			result.Errors = len(final.Errors)
			for _, ie := range final.Errors {
				result.FailedKeys = append(result.FailedKeys, PushError{
					Key:     ie.Key,
					Message: ie.Reason,
				})
			}
		}
		sort.Strings(result.Sections)
		return result, fmt.Errorf("wait for import: %w", err)
	}

	// Build result from final status
	result.Upserted = final.CreatedRows + final.UpdatedRows
	result.Errors = len(final.Errors)
	for _, ie := range final.Errors {
		result.FailedKeys = append(result.FailedKeys, PushError{
			Key:     ie.Key,
			Message: ie.Reason,
		})
	}

	sort.Strings(result.Sections)
	return result, nil
}

// Pull downloads strings from the API and writes them to local CSVs.
// If section is non-empty, only that section is pulled.
// Existing local CSVs are overwritten.
func Pull(c *client.Client, wsDir, section string) (*PullResult, error) {
	// Fetch all strings
	strings, err := c.ListAllStrings(client.ListStringsOpts{})
	if err != nil {
		return nil, fmt.Errorf("list strings: %w", err)
	}

	// Fetch sections for ID → name mapping
	sectionList, err := c.ListSections()
	if err != nil {
		return nil, fmt.Errorf("list sections: %w", err)
	}
	sectionNames := make(map[string]string) // ID → name
	for _, sec := range sectionList.Data {
		sectionNames[sec.ID] = sec.Name
	}

	// Group strings by section name ("" for unsectioned)
	bySection := make(map[string][]Row)
	for _, entry := range strings {
		secName := ""
		if entry.SectionID != nil {
			if name, ok := sectionNames[*entry.SectionID]; ok {
				secName = name
			}
		}

		// Filter by section if specified
		if section != "" && secName != section {
			continue
		}

		for locale, value := range entry.Values {
			bySection[secName] = append(bySection[secName], Row{
				Key:    entry.Key,
				Locale: locale,
				Value:  value,
				Format: entry.Format,
			})
		}
	}

	result := &PullResult{
		StringCount: len(strings),
	}

	if section != "" {
		// Count only strings in the requested section
		count := 0
		for _, entry := range strings {
			secName := ""
			if entry.SectionID != nil {
				if name, ok := sectionNames[*entry.SectionID]; ok {
					secName = name
				}
			}
			if secName == section {
				count++
			}
		}
		result.StringCount = count
	}

	// Write CSVs
	for secName, rows := range bySection {
		// Sort rows for deterministic output
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].Key != rows[j].Key {
				return rows[i].Key < rows[j].Key
			}
			return rows[i].Locale < rows[j].Locale
		})

		path := CSVPath(wsDir, secName)
		if err := WriteCSV(path, rows); err != nil {
			return nil, fmt.Errorf("write %s: %w", path, err)
		}
		result.FileCount++
		if secName != "" {
			result.Sections = append(result.Sections, secName)
		}
	}

	sort.Strings(result.Sections)
	return result, nil
}

// EnsureSection finds a section by name or creates it. Returns the section ID.
func EnsureSection(c *client.Client, name string) (string, error) {
	sections, err := c.ListSections()
	if err != nil {
		return "", err
	}
	for _, sec := range sections.Data {
		if sec.Name == name {
			return sec.ID, nil
		}
	}

	// Create new section
	sec, err := c.CreateSection(client.CreateSectionRequest{Name: name})
	if err != nil {
		return "", fmt.Errorf("create section %q: %w", name, err)
	}
	return sec.ID, nil
}
