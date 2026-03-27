package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListAllStrings_SinglePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := StringList{
			Data: []StringEntry{
				{Key: "greeting", Format: "text", Values: map[string]string{"en": "Hello"}},
			},
			Pagination: PaginationMeta{HasMore: false},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "proj", "env")
	all, err := c.ListAllStrings(ListStringsOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(all))
	}
	if all[0].Key != "greeting" {
		t.Errorf("expected key 'greeting', got %q", all[0].Key)
	}
}

func TestListAllStrings_MultiplePages(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		cursor := r.URL.Query().Get("cursor")

		var resp StringList
		switch cursor {
		case "":
			resp = StringList{
				Data: []StringEntry{
					{Key: "a", Format: "text", Values: map[string]string{"en": "A"}},
					{Key: "b", Format: "text", Values: map[string]string{"en": "B"}},
				},
				Pagination: PaginationMeta{HasMore: true, NextCursor: "page2"},
			}
		case "page2":
			resp = StringList{
				Data: []StringEntry{
					{Key: "c", Format: "text", Values: map[string]string{"en": "C"}},
				},
				Pagination: PaginationMeta{HasMore: false},
			}
		default:
			t.Errorf("unexpected cursor: %q", cursor)
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "proj", "env")
	all, err := c.ListAllStrings(ListStringsOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(all))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls, got %d", callCount)
	}
}

func TestListAllStrings_EmptyResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := StringList{
			Data:       []StringEntry{},
			Pagination: PaginationMeta{HasMore: false},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New("test-key", srv.URL, "proj", "env")
	all, err := c.ListAllStrings(ListStringsOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected 0 entries, got %d", len(all))
	}
}

func TestListAllStrings_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error: ErrorBody{Code: "unauthorized", Message: "Invalid API key"},
		})
	}))
	defer srv.Close()

	c := New("bad-key", srv.URL, "proj", "env")
	_, err := c.ListAllStrings(ListStringsOpts{})
	if err == nil {
		t.Fatal("expected error for unauthorized request")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("expected status 401, got %d", apiErr.StatusCode)
	}
}
