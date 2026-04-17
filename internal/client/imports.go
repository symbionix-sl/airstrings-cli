package client

import (
	"fmt"
	"time"
)

// ImportStatus represents the status of a CSV import job.
type ImportStatus struct {
	ID            string        `json:"id"`
	Status        string        `json:"status"`
	TotalRows     int           `json:"total_rows"`
	ProcessedRows int           `json:"processed_rows"`
	CreatedRows   int           `json:"created_rows"`
	UpdatedRows   int           `json:"updated_rows"`
	SkippedRows   int           `json:"skipped_rows"`
	Errors        []ImportError `json:"errors"`
	CreatedAt     string        `json:"created_at"`
	UpdatedAt     string        `json:"updated_at"`
}

// ImportError represents a single row-level error from an import.
type ImportError struct {
	Row    int    `json:"row"`
	Key    string `json:"key,omitempty"`
	Locale string `json:"locale,omitempty"`
	Reason string `json:"reason"`
}

// CreateImport uploads a CSV file (possibly gzipped) via multipart form and starts an import job.
// If sectionID is non-nil, it is sent as a form field to scope all rows to that section.
func (c *Client) CreateImport(csvData []byte, sectionID *string) (*ImportStatus, error) {
	fields := make(map[string]string)
	if sectionID != nil {
		fields["section_id"] = *sectionID
	}

	var s ImportStatus
	err := c.doMultipart(c.envPath()+"/imports", fields, "strings.csv", csvData, &s)
	return &s, err
}

// GetImport retrieves the current status of an import job.
func (c *Client) GetImport(id string) (*ImportStatus, error) {
	var s ImportStatus
	err := c.do("GET", c.envPath()+"/imports/"+id, nil, nil, &s)
	return &s, err
}

// WaitForImport polls an import job until it reaches a terminal state.
// onPoll is called on each poll with the current status (may be nil).
// Returns an error if the import fails or the 5 minute timeout is exceeded.
func (c *Client) WaitForImport(id string, onPoll func(*ImportStatus)) (*ImportStatus, error) {
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("import %s timed out after 5 minutes", id)
		case <-ticker.C:
			status, err := c.GetImport(id)
			if err != nil {
				return nil, fmt.Errorf("poll import %s: %w", id, err)
			}
			if onPoll != nil {
				onPoll(status)
			}
			switch status.Status {
			case "completed":
				return status, nil
			case "failed":
				return status, fmt.Errorf("import %s failed", id)
			}
		}
	}
}
