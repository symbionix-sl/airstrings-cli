package client

import "time"

type APIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Permission string     `json:"permission"`
	Scope      string     `json:"scope"`
	EnvID      *string    `json:"env_id"`
	FullPower  bool       `json:"full_power"`
	Prefix     string     `json:"prefix"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  string     `json:"created_at"`
}

type APIKeyList struct {
	Data []APIKey `json:"data"`
}

type CreateAPIKeyRequest struct {
	Name       string `json:"name"`
	Permission string `json:"permission"`
}

type APIKeyCreated struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Permission string `json:"permission"`
	Scope      string `json:"scope"`
	FullPower  bool   `json:"full_power"`
	Key        string `json:"key"`
	Prefix     string `json:"prefix"`
	CreatedAt  string `json:"created_at"`
}

func (c *Client) keysPath() string {
	if KeyType(c.apiKey) == "environment" {
		return c.envPath() + "/api-keys"
	}
	return c.projectPath() + "/api-keys"
}

// ListAPIKeys returns metadata for the API keys visible to this key.
func (c *Client) ListAPIKeys() (*APIKeyList, error) {
	var list APIKeyList
	err := c.do("GET", c.keysPath(), nil, nil, &list)
	return &list, err
}

// CreateAPIKey creates a new API key; the raw key is only returned here.
func (c *Client) CreateAPIKey(req CreateAPIKeyRequest) (*APIKeyCreated, error) {
	var k APIKeyCreated
	err := c.do("POST", c.keysPath(), nil, req, &k)
	return &k, err
}

// RevokeAPIKey immediately revokes an API key by ID.
func (c *Client) RevokeAPIKey(id string) error {
	return c.do("DELETE", c.envPath()+"/api-keys/"+id, nil, nil, nil)
}

// RotateProjectKey replaces the calling project key; the old key stops working.
func (c *Client) RotateProjectKey() (*APIKeyCreated, error) {
	var k APIKeyCreated
	err := c.do("POST", c.projectPath()+"/api-keys/rotate", nil, nil, &k)
	return &k, err
}
