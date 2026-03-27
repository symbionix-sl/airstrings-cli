package workspace

import (
	"fmt"
	"sort"

	"github.com/symbionix/airstrings-cli/internal/client"
)

// PushResult summarizes the outcome of a push operation.
type PushResult struct {
	Upserted int
	Errors   int
	Sections []string
}

// PullResult summarizes the outcome of a pull operation.
type PullResult struct {
	StringCount int
	Sections    []string
	FileCount   int
}

// Push reads local CSVs and upserts each key to the API.
// If section is non-empty, only that section is pushed.
// Sections are created remotely if they don't exist.
func Push(c *client.Client, wsDir, section string) (*PushResult, error) {
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

	// Cache section name → ID lookups
	sectionIDs := make(map[string]string)

	for secName, csvPath := range csvMap {
		rows, err := ReadCSV(csvPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", csvPath, err)
		}
		if len(rows) == 0 {
			continue
		}

		// Resolve section ID if this is a section CSV
		var sectionID *string
		if secName != "" {
			id, ok := sectionIDs[secName]
			if !ok {
				id, err = EnsureSection(c, secName)
				if err != nil {
					return nil, fmt.Errorf("ensure section %q: %w", secName, err)
				}
				sectionIDs[secName] = id
			}
			sectionID = &id
			result.Sections = append(result.Sections, secName)
		}

		// Group rows by key
		grouped := groupByKey(rows)

		// Upsert each key
		for _, key := range sortedKeys(grouped) {
			keyRows := grouped[key]
			format := keyRows[0].Format

			values := make(map[string]*string, len(keyRows))
			for _, row := range keyRows {
				v := row.Value
				values[row.Locale] = &v
			}

			req := client.UpsertStringRequest{
				Format:    format,
				Values:    values,
				SectionID: sectionID,
			}

			_, err := c.UpsertString(key, req)
			if err != nil {
				result.Errors++
				continue
			}
			result.Upserted++
		}
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

// groupByKey groups rows by their Key field.
func groupByKey(rows []Row) map[string][]Row {
	m := make(map[string][]Row)
	for _, r := range rows {
		m[r.Key] = append(m[r.Key], r)
	}
	return m
}

// sortedKeys returns the keys of a map in sorted order.
func sortedKeys(m map[string][]Row) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
