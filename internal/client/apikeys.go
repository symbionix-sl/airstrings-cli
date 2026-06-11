package client

import "time"

type APIKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Permission string     `json:"permission"`
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
	Key        string `json:"key"`
	Prefix     string `json:"prefix"`
	CreatedAt  string `json:"created_at"`
}

// ListAPIKeys returns metadata for all API keys scoped to the environment.
func (c *Client) ListAPIKeys() (*APIKeyList, error) {
	var list APIKeyList
	err := c.do("GET", c.envPath()+"/api-keys", nil, nil, &list)
	return &list, err
}

// CreateAPIKey creates a new API key; the raw key is only returned here.
func (c *Client) CreateAPIKey(req CreateAPIKeyRequest) (*APIKeyCreated, error) {
	var k APIKeyCreated
	err := c.do("POST", c.envPath()+"/api-keys", nil, req, &k)
	return &k, err
}

// RevokeAPIKey immediately revokes an API key by ID.
func (c *Client) RevokeAPIKey(id string) error {
	return c.do("DELETE", c.envPath()+"/api-keys/"+id, nil, nil, nil)
}
