package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const experimentBody = `{
	"experiment_id": "exp_a1b2c3d4e5f6",
	"key": "checkout.cta",
	"env_id": "env_a1b2c3d4e5f6",
	"status": "draft",
	"allocation": {"control": 50, "variant_a": 50},
	"variants": {"variant_a": {"en-US": "Continue"}},
	"created_at": "2026-07-14T00:00:00Z",
	"updated_at": "2026-07-14T00:00:00Z"
}`

func TestGetExperiment(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(experimentBody))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "proj", "env")
	exp, err := c.GetExperiment("checkout.cta")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "GET" {
		t.Errorf("expected GET, got %s", gotMethod)
	}
	if gotPath != "/v1/projects/proj/environments/env/strings/checkout.cta/experiment" {
		t.Errorf("unexpected path: %q", gotPath)
	}
	if exp.ExperimentID != "exp_a1b2c3d4e5f6" || exp.Status != "draft" {
		t.Errorf("unexpected experiment: %+v", exp)
	}
	if exp.Allocation["variant_a"] != 50 {
		t.Errorf("unexpected allocation: %+v", exp.Allocation)
	}
	if exp.Variants["variant_a"]["en-US"] != "Continue" {
		t.Errorf("unexpected variants: %+v", exp.Variants)
	}
}

func TestPutExperiment(t *testing.T) {
	var gotMethod, gotPath string
	var gotReq ExperimentUpsertRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(experimentBody))
	}))
	defer srv.Close()

	allocation := map[string]int{"control": 50, "variant_a": 50}
	variants := map[string]map[string]string{"variant_a": {"en-US": "Continue"}}

	c := New("test-key", srv.URL, "proj", "env")
	exp, err := c.PutExperiment("checkout.cta", allocation, variants)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "PUT" {
		t.Errorf("expected PUT, got %s", gotMethod)
	}
	if gotPath != "/v1/projects/proj/environments/env/strings/checkout.cta/experiment" {
		t.Errorf("unexpected path: %q", gotPath)
	}
	if gotReq.Allocation["variant_a"] != 50 || gotReq.Allocation["control"] != 50 {
		t.Errorf("unexpected allocation in body: %+v", gotReq.Allocation)
	}
	if gotReq.Variants["variant_a"]["en-US"] != "Continue" {
		t.Errorf("unexpected variants in body: %+v", gotReq.Variants)
	}
	if exp.ExperimentID != "exp_a1b2c3d4e5f6" {
		t.Errorf("unexpected experiment: %+v", exp)
	}
}

func TestDeleteExperiment(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "proj", "env")
	if err := c.DeleteExperiment("checkout.cta"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "DELETE" {
		t.Errorf("expected DELETE, got %s", gotMethod)
	}
	if gotPath != "/v1/projects/proj/environments/env/strings/checkout.cta/experiment" {
		t.Errorf("unexpected path: %q", gotPath)
	}
}

func TestStartExperiment(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(experimentBody))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "proj", "env")
	exp, err := c.StartExperiment("checkout.cta")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/v1/projects/proj/environments/env/strings/checkout.cta/experiment/start" {
		t.Errorf("unexpected path: %q", gotPath)
	}
	if exp.ExperimentID != "exp_a1b2c3d4e5f6" {
		t.Errorf("unexpected experiment: %+v", exp)
	}
}

func TestStopExperiment(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(experimentBody))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "proj", "env")
	exp, err := c.StopExperiment("checkout.cta")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/v1/projects/proj/environments/env/strings/checkout.cta/experiment/stop" {
		t.Errorf("unexpected path: %q", gotPath)
	}
	if exp.ExperimentID != "exp_a1b2c3d4e5f6" {
		t.Errorf("unexpected experiment: %+v", exp)
	}
}

func TestPromoteVariant(t *testing.T) {
	var gotMethod, gotPath string
	var gotReq ExperimentPromoteRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"experiment": ` + experimentBody + `,
			"publish_results": [
				{"locale": "en-US", "status": "ok"},
				{"locale": "it", "status": "error", "error": "sign failed"}
			]
		}`))
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "proj", "env")
	resp, err := c.PromoteVariant("checkout.cta", "variant_a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/v1/projects/proj/environments/env/strings/checkout.cta/experiment/promote" {
		t.Errorf("unexpected path: %q", gotPath)
	}
	if gotReq.Variant != "variant_a" {
		t.Errorf("unexpected request body: %+v", gotReq)
	}
	if resp.Experiment.ExperimentID != "exp_a1b2c3d4e5f6" {
		t.Errorf("unexpected experiment: %+v", resp.Experiment)
	}
	if len(resp.PublishResults) != 2 {
		t.Fatalf("expected 2 publish results, got %d", len(resp.PublishResults))
	}
	if resp.PublishResults[0].Locale != "en-US" || resp.PublishResults[0].Status != "ok" {
		t.Errorf("unexpected first publish result: %+v", resp.PublishResults[0])
	}
	if resp.PublishResults[1].Status != "error" || resp.PublishResults[1].Error != "sign failed" {
		t.Errorf("unexpected second publish result: %+v", resp.PublishResults[1])
	}
}

func TestStartExperiment_QuotaExceeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error: ErrorBody{Code: "quota_exceeded", Message: "experiment quota reached"},
		})
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "proj", "env")
	_, err := c.StartExperiment("checkout.cta")
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 403 {
		t.Errorf("expected status 403, got %d", apiErr.StatusCode)
	}
	if apiErr.Body.Error.Code != "quota_exceeded" {
		t.Errorf("expected code quota_exceeded, got %q", apiErr.Body.Error.Code)
	}
	if apiErr.ExitCode() != 3 {
		t.Errorf("expected exit code 3, got %d", apiErr.ExitCode())
	}
}

func TestGetExperiment_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error: ErrorBody{Code: "not_found", Message: "no experiment configured"},
		})
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "proj", "env")
	_, err := c.GetExperiment("checkout.cta")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", apiErr.StatusCode)
	}
	if apiErr.ExitCode() != 4 {
		t.Errorf("expected exit code 4, got %d", apiErr.ExitCode())
	}
}
