package client

import (
	"fmt"
	"net/url"
)

type StringEntry struct {
	Key       string            `json:"key"`
	Format    string            `json:"format"`
	SectionID *string           `json:"section_id"`
	Values    map[string]string `json:"values"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
}

type PaginationMeta struct {
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type StringList struct {
	Data       []StringEntry  `json:"data"`
	Pagination PaginationMeta `json:"pagination"`
}

type ListStringsOpts struct {
	Locale  string
	Section string
	Cursor  string
	Limit   int
}

func (c *Client) ListStrings(opts ListStringsOpts) (*StringList, error) {
	q := url.Values{}
	if opts.Locale != "" {
		q.Set("locale", opts.Locale)
	}
	if opts.Section != "" {
		q.Set("section", opts.Section)
	}
	if opts.Cursor != "" {
		q.Set("cursor", opts.Cursor)
	}
	if opts.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", opts.Limit))
	}
	var list StringList
	err := c.do("GET", c.envPath()+"/strings", q, nil, &list)
	return &list, err
}

func (c *Client) GetString(key string) (*StringEntry, error) {
	var s StringEntry
	err := c.do("GET", c.envPath()+"/strings/"+key, nil, nil, &s)
	return &s, err
}

type CreateStringRequest struct {
	Key       string            `json:"key"`
	Format    string            `json:"format,omitempty"`
	SectionID *string           `json:"section_id,omitempty"`
	Values    map[string]string `json:"values"`
}

func (c *Client) CreateString(req CreateStringRequest) (*StringEntry, error) {
	var s StringEntry
	err := c.do("POST", c.envPath()+"/strings", nil, req, &s)
	return &s, err
}

type UpsertStringRequest struct {
	Format string             `json:"format,omitempty"`
	Values map[string]*string `json:"values"`
}

func (c *Client) UpsertString(key string, req UpsertStringRequest) (*StringEntry, error) {
	var s StringEntry
	err := c.do("PUT", c.envPath()+"/strings/"+key, nil, req, &s)
	return &s, err
}

func (c *Client) DeleteString(key string) error {
	return c.do("DELETE", c.envPath()+"/strings/"+key, nil, nil, nil)
}

// Sections

type Section struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	StringCount int    `json:"string_count"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type SectionList struct {
	Data       []Section      `json:"data"`
	Pagination PaginationMeta `json:"pagination"`
}

func (c *Client) ListSections() (*SectionList, error) {
	var list SectionList
	err := c.do("GET", c.envPath()+"/sections", nil, nil, &list)
	return &list, err
}

type CreateSectionRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func (c *Client) CreateSection(req CreateSectionRequest) (*Section, error) {
	var s Section
	err := c.do("POST", c.envPath()+"/sections", nil, req, &s)
	return &s, err
}

func (c *Client) DeleteSection(id string) error {
	return c.do("DELETE", c.envPath()+"/sections/"+id, nil, nil, nil)
}
