package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPromotionPreview(t *testing.T) {
	var gotPath, gotSource, gotTarget string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotSource = r.URL.Query().Get("source_env_id")
		gotTarget = r.URL.Query().Get("target_env_id")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"source_env_id": "env_src",
			"target_env_id": "env_tgt",
			"summary": {"added": 1, "updated": 1, "extra": 0},
			"entries": [
				{"key": "greeting", "locales": [
					{"locale": "en", "change_type": "added", "source_value": "Hello", "target_value": null}
				]}
			]
		}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "proj", "env")
	resp, err := c.PromotionPreview("env_src", "env_tgt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/v1/projects/proj/promotions/preview" {
		t.Errorf("unexpected path: %q", gotPath)
	}
	if gotSource != "env_src" {
		t.Errorf("expected source_env_id 'env_src', got %q", gotSource)
	}
	if gotTarget != "env_tgt" {
		t.Errorf("expected target_env_id 'env_tgt', got %q", gotTarget)
	}

	if resp.Summary.Added != 1 || resp.Summary.Updated != 1 || resp.Summary.Extra != 0 {
		t.Errorf("unexpected summary: %+v", resp.Summary)
	}
	if len(resp.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(resp.Entries))
	}
	if len(resp.Entries[0].Locales) != 1 {
		t.Fatalf("expected 1 locale change, got %d", len(resp.Entries[0].Locales))
	}

	loc := resp.Entries[0].Locales[0]
	if loc.SourceValue == nil || *loc.SourceValue != "Hello" {
		t.Errorf("expected source_value 'Hello', got %v", loc.SourceValue)
	}
	if loc.TargetValue != nil {
		t.Errorf("expected nil target_value for null, got %q", *loc.TargetValue)
	}
}

func TestPromotionPreview_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"error":{"code":"unprocessable_entity","message":"source and target environments must differ"}}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "proj", "env")
	_, err := c.PromotionPreview("env_same", "env_same")
	if err == nil {
		t.Fatal("expected error for 422 response")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 422 {
		t.Errorf("expected status 422, got %d", apiErr.StatusCode)
	}
}
