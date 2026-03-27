package client

type ImportStatus struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	TotalRows      int    `json:"total_rows"`
	ImportedCount  int    `json:"imported_count"`
	SkippedCount   int    `json:"skipped_count"`
	ErrorCount     int    `json:"error_count"`
	Errors         []any  `json:"errors,omitempty"`
	CreatedAt      string `json:"created_at"`
	CompletedAt    string `json:"completed_at,omitempty"`
}

type CreateImportRequest struct {
	Format string `json:"format"`
	Data   string `json:"data"`
}

func (c *Client) CreateImport(req CreateImportRequest) (*ImportStatus, error) {
	var s ImportStatus
	err := c.do("POST", c.envPath()+"/imports", nil, req, &s)
	return &s, err
}

func (c *Client) GetImport(id string) (*ImportStatus, error) {
	var s ImportStatus
	err := c.do("GET", c.envPath()+"/imports/"+id, nil, nil, &s)
	return &s, err
}
