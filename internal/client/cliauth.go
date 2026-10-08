package client

import "os"

type CLIAuthStart struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	Interval                int    `json:"interval"`
	ExpiresIn               int    `json:"expires_in"`
}

type CLIAuthToken struct {
	APIKey    string `json:"api_key"`
	KeyID     string `json:"key_id"`
	OrgID     string `json:"org_id"`
	OrgName   string `json:"org_name"`
	FullPower bool   `json:"full_power"`
}

// ClientName identifies this CLI on the approve page, capped at the API's 90 characters.
func ClientName(version string) string {
	host, _ := os.Hostname()
	name := []rune(StripControl("airstrings-cli " + version + " on " + host))
	if len(name) > 90 {
		name = name[:90]
	}
	return string(name)
}

func (c *Client) StartCLIAuth(clientName string) (*CLIAuthStart, error) {
	var s CLIAuthStart
	err := c.do("POST", "/v1/cli/auth/start", nil, map[string]string{"client_name": clientName}, &s)
	return &s, err
}

// PollCLIAuth redeems an approved device code; pending states come back as *APIError codes.
func (c *Client) PollCLIAuth(deviceCode string) (*CLIAuthToken, error) {
	var t CLIAuthToken
	err := c.do("POST", "/v1/cli/auth/token", nil, map[string]string{"device_code": deviceCode}, &t)
	return &t, err
}

func (c *Client) RevokeOrgKey(keyID string) error {
	return c.do("DELETE", "/v1/org/api-keys/"+keyID, nil, nil, nil)
}

type Org struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *Client) GetOrg() (*Org, error) {
	var o Org
	return &o, c.do("GET", "/v1/org", nil, nil, &o)
}
