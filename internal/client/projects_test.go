package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListProjects_OrgKeyList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/projects" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"data":[{"id":"proj_a","name":"A"},{"id":"proj_b","name":"B"}]}`))
	}))
	defer srv.Close()

	projects, err := New("as_org_x", srv.URL, "", "").ListProjects()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 2 || projects[1].ID != "proj_b" {
		t.Errorf("unexpected projects: %+v", projects)
	}
}

func TestListProjects_SingleProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"proj_a","name":"A"}`))
	}))
	defer srv.Close()

	projects, err := New("as_proj_x", srv.URL, "", "").ListProjects()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 1 || projects[0].ID != "proj_a" {
		t.Errorf("unexpected projects: %+v", projects)
	}
}

func TestCreateProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/projects" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var req CreateProjectRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Name != "my-app" {
			t.Errorf("unexpected name: %q", req.Name)
		}
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"proj_new","name":"my-app"}`))
	}))
	defer srv.Close()

	p, err := New("as_org_x", srv.URL, "", "").CreateProject(CreateProjectRequest{Name: "my-app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ID != "proj_new" {
		t.Errorf("unexpected project: %+v", p)
	}
}

func TestGetProject_UsesProjectPathWhenKnown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/projects/proj_a" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte(`{"id":"proj_a","name":"A"}`))
	}))
	defer srv.Close()

	p, err := New("as_org_x", srv.URL, "proj_a", "").GetProject()
	if err != nil || p.ID != "proj_a" {
		t.Fatalf("GetProject() = %+v, %v", p, err)
	}
}
