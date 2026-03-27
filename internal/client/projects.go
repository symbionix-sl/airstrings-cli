package client

import "time"

type Project struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	DefaultLocale string `json:"default_locale"`
	StringCount   int    `json:"string_count"`
	LocaleCount   int    `json:"locale_count"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// GetProject returns the project bound to this API key.
func (c *Client) GetProject() (*Project, error) {
	var p Project
	err := c.do("GET", "/v1/projects", nil, nil, &p)
	return &p, err
}

type Environment struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
	IsSealed  bool   `json:"is_sealed"`
	PublicKey string `json:"public_key,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type EnvironmentList struct {
	Data []Environment `json:"data"`
}

func (c *Client) ListEnvironments() ([]Environment, error) {
	var list EnvironmentList
	err := c.do("GET", c.projectPath()+"/environments", nil, nil, &list)
	return list.Data, err
}

func (c *Client) GetEnvironment(envID string) (*Environment, error) {
	var env Environment
	err := c.do("GET", c.projectPath()+"/environments/"+envID, nil, nil, &env)
	return &env, err
}

type CreateEnvRequest struct {
	Name        string  `json:"name"`
	SourceEnvID *string `json:"source_env_id,omitempty"`
}

func (c *Client) CreateEnvironment(req CreateEnvRequest) (*Environment, error) {
	var env Environment
	err := c.do("POST", c.projectPath()+"/environments", nil, req, &env)
	return &env, err
}

// Locale represents a locale with string count.
type Locale struct {
	Locale      string `json:"locale"`
	StringCount int    `json:"string_count"`
}

type LocaleList struct {
	Data []Locale `json:"data"`
}

func (c *Client) ListLocales() ([]Locale, error) {
	var list LocaleList
	err := c.do("GET", c.envPath()+"/locales", nil, nil, &list)
	return list.Data, err
}

// Bundle status types.

type BundleStatus struct {
	ProjectID   string `json:"project_id"`
	Locale      string `json:"locale"`
	Revision    int    `json:"revision"`
	KeyID       string `json:"key_id"`
	CreatedAt   string `json:"created_at"`
	CDNURL      string `json:"cdn_url"`
	SizeBytes   int    `json:"size_bytes"`
	StringCount int    `json:"string_count"`
}

type BundleList struct {
	Data []BundleStatus `json:"data"`
}

func (c *Client) ListBundles() ([]BundleStatus, error) {
	var list BundleList
	err := c.do("GET", c.envPath()+"/bundles", nil, nil, &list)
	return list.Data, err
}

type PublishRequest struct {
	Locales []string `json:"locales,omitempty"`
}

type LocalePublishStatus struct {
	Locale string        `json:"locale"`
	Status string        `json:"status"`
	Bundle *BundleStatus `json:"bundle,omitempty"`
	Error  string        `json:"error,omitempty"`
}

type PublishResponse struct {
	Results     []LocalePublishStatus `json:"results"`
	PublishedAt time.Time             `json:"published_at"`
}

func (c *Client) PublishBundles(locales []string) (*PublishResponse, error) {
	var resp PublishResponse
	err := c.do("POST", c.envPath()+"/bundles/publish", nil, PublishRequest{Locales: locales}, &resp)
	return &resp, err
}
