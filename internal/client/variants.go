package client

import "net/url"

type Experiment struct {
	ExperimentID string                       `json:"experiment_id"`
	Key          string                       `json:"key"`
	EnvID        string                       `json:"env_id"`
	Status       string                       `json:"status"`
	Allocation   map[string]int               `json:"allocation"`
	Variants     map[string]map[string]string `json:"variants"`
	CreatedAt    string                       `json:"created_at"`
	UpdatedAt    string                       `json:"updated_at"`
}

type ExperimentUpsertRequest struct {
	Allocation map[string]int               `json:"allocation"`
	Variants   map[string]map[string]string `json:"variants"`
}

type ExperimentPromoteRequest struct {
	Variant string `json:"variant"`
}

type ExperimentPromoteResponse struct {
	Experiment     Experiment             `json:"experiment"`
	PublishResults []PromotePublishStatus `json:"publish_results"`
}

type PromotePublishStatus struct {
	Locale string `json:"locale"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func (c *Client) GetExperiment(key string) (*Experiment, error) {
	var exp Experiment
	err := c.do("GET", c.envPath()+"/strings/"+url.PathEscape(key)+"/experiment", nil, nil, &exp)
	return &exp, err
}

func (c *Client) PutExperiment(key string, allocation map[string]int, variants map[string]map[string]string) (*Experiment, error) {
	req := ExperimentUpsertRequest{Allocation: allocation, Variants: variants}
	var exp Experiment
	err := c.do("PUT", c.envPath()+"/strings/"+url.PathEscape(key)+"/experiment", nil, req, &exp)
	return &exp, err
}

func (c *Client) DeleteExperiment(key string) error {
	return c.do("DELETE", c.envPath()+"/strings/"+url.PathEscape(key)+"/experiment", nil, nil, nil)
}

func (c *Client) DeleteVariant(key, variant string) (*Experiment, error) {
	var exp Experiment
	err := c.do("DELETE", c.envPath()+"/strings/"+url.PathEscape(key)+"/experiment/variants/"+url.PathEscape(variant), nil, nil, &exp)
	return &exp, err
}

func (c *Client) StartExperiment(key string) (*Experiment, error) {
	var exp Experiment
	err := c.do("POST", c.envPath()+"/strings/"+url.PathEscape(key)+"/experiment/start", nil, nil, &exp)
	return &exp, err
}

func (c *Client) StopExperiment(key string) (*Experiment, error) {
	var exp Experiment
	err := c.do("POST", c.envPath()+"/strings/"+url.PathEscape(key)+"/experiment/stop", nil, nil, &exp)
	return &exp, err
}

func (c *Client) PromoteVariant(key, variant string) (*ExperimentPromoteResponse, error) {
	req := ExperimentPromoteRequest{Variant: variant}
	var resp ExperimentPromoteResponse
	err := c.do("POST", c.envPath()+"/strings/"+url.PathEscape(key)+"/experiment/promote", nil, req, &resp)
	return &resp, err
}
