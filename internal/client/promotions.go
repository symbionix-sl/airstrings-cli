package client

import "net/url"

type PromotionPreview struct {
	SourceEnvID string           `json:"source_env_id"`
	TargetEnvID string           `json:"target_env_id"`
	Summary     PromotionSummary `json:"summary"`
	Entries     []PromotionEntry `json:"entries"`
}

type PromotionSummary struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Extra   int `json:"extra"`
}

type PromotionEntry struct {
	Key     string                  `json:"key"`
	Locales []PromotionLocaleChange `json:"locales"`
}

type PromotionLocaleChange struct {
	Locale      string  `json:"locale"`
	ChangeType  string  `json:"change_type"`
	SourceValue *string `json:"source_value"`
	TargetValue *string `json:"target_value"`
}

func (c *Client) PromotionPreview(sourceEnvID, targetEnvID string) (*PromotionPreview, error) {
	q := url.Values{}
	q.Set("source_env_id", sourceEnvID)
	q.Set("target_env_id", targetEnvID)

	var resp PromotionPreview
	err := c.do("GET", c.projectPath()+"/promotions/preview", q, nil, &resp)
	return &resp, err
}
