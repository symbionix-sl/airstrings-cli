package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.airstrings.com"

// Client is the AirStrings API client.
type Client struct {
	baseURL    string
	apiKey     string
	projectID  string
	envID      string
	httpClient *http.Client
}

// New creates a new API client.
func New(apiKey, baseURL, projectID, envID string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		baseURL:   baseURL,
		apiKey:    apiKey,
		projectID: projectID,
		envID:     envID,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) ProjectID() string { return c.projectID }
func (c *Client) EnvID() string     { return c.envID }

// APIError represents a structured error from the API.
type APIError struct {
	StatusCode int
	Body       ErrorResponse
}

func (e *APIError) Error() string {
	if e.Body.Error.Message != "" {
		msg := fmt.Sprintf("API error %d: %s", e.StatusCode, e.Body.Error.Message)
		for _, d := range e.Body.Error.Details {
			msg += fmt.Sprintf("\n  - %s: %s", d.Field, d.Reason)
		}
		return msg
	}
	return fmt.Sprintf("API error %d", e.StatusCode)
}

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Details []ValidationError `json:"details,omitempty"`
}

type ValidationError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

func (c *Client) do(method, path string, query url.Values, body any, result any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, u, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("X-API-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	ct := resp.Header.Get("Content-Type")
	looksLikeJSON := len(respBody) > 0 && (respBody[0] == '{' || respBody[0] == '[')
	isJSON := strings.Contains(ct, "json") || (ct == "" && looksLikeJSON)

	if resp.StatusCode >= 400 {
		var apiErr APIError
		apiErr.StatusCode = resp.StatusCode
		if isJSON || looksLikeJSON {
			_ = json.Unmarshal(respBody, &apiErr.Body)
		}
		if apiErr.Body.Error.Message == "" && !isJSON && !looksLikeJSON {
			apiErr.Body.Error.Message = fmt.Sprintf("unexpected response from %s (got %s, expected JSON) — check your --url value", c.baseURL, ct)
		}
		return &apiErr
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			if !isJSON && !looksLikeJSON {
				return fmt.Errorf("unexpected response from %s (got %s, expected JSON) — check your --url value", c.baseURL, ct)
			}
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}

// doMultipart sends a multipart/form-data POST request with a file and optional fields.
func (c *Client) doMultipart(path string, fields map[string]string, fileName string, fileData []byte, result any) error {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	// Write form fields
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return fmt.Errorf("write field %s: %w", k, err)
		}
	}

	// Write file part
	part, err := w.CreateFormFile("file", fileName)
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(fileData); err != nil {
		return fmt.Errorf("write file data: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("close multipart writer: %w", err)
	}

	u := c.baseURL + path

	req, err := http.NewRequest("POST", u, &body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	// Use a longer timeout for multipart uploads
	uploadClient := &http.Client{
		Timeout: 5 * time.Minute,
	}

	resp, err := uploadClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	ct := resp.Header.Get("Content-Type")
	looksLikeJSON := len(respBody) > 0 && (respBody[0] == '{' || respBody[0] == '[')
	isJSON := strings.Contains(ct, "json") || (ct == "" && looksLikeJSON)

	if resp.StatusCode >= 400 {
		var apiErr APIError
		apiErr.StatusCode = resp.StatusCode
		if isJSON || looksLikeJSON {
			_ = json.Unmarshal(respBody, &apiErr.Body)
		}
		if apiErr.Body.Error.Message == "" && !isJSON && !looksLikeJSON {
			apiErr.Body.Error.Message = fmt.Sprintf("unexpected response from %s (got %s, expected JSON) — check your --url value", c.baseURL, ct)
		}
		return &apiErr
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			if !isJSON && !looksLikeJSON {
				return fmt.Errorf("unexpected response from %s (got %s, expected JSON) — check your --url value", c.baseURL, ct)
			}
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}

// envPath returns the base path for environment-scoped resources.
func (c *Client) envPath() string {
	return fmt.Sprintf("/v1/projects/%s/environments/%s", c.projectID, c.envID)
}

// projectPath returns the base path for project-scoped resources.
func (c *Client) projectPath() string {
	return fmt.Sprintf("/v1/projects/%s", c.projectID)
}
