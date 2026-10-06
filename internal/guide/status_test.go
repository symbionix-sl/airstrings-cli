package guide

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
)

func inspectServer(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api-keys") {
			w.Write([]byte(`{"data":[{"id":"ak_r","permission":"read","prefix":"as_proj_aaaa1111"},{"id":"ak_w","permission":"write","prefix":"as_proj_aaaa2222"}]}`))
			return
		}
		w.Write([]byte(`{"data":[{"id":"env_p","name":"production","is_default":true,"is_sealed":true},{"id":"env_s","name":"staging"}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestInspect_MatchesLongPrefix(t *testing.T) {
	srv := inspectServer(t)
	key := "as_proj_aaaa2222" + strings.Repeat("f", 56)
	if d := Inspect(client.New(key, srv.URL, "proj", "env_s"), key, nil, false); d.KeyScope != "write" {
		t.Errorf("KeyScope = %q, want write", d.KeyScope)
	}
}

func TestInspect_OrgKeyIsWrite(t *testing.T) {
	srv := inspectServer(t)
	if d := Inspect(client.New("as_org_k", srv.URL, "proj", "env_s"), "as_org_k", nil, false); d.KeyScope != "write" {
		t.Errorf("KeyScope = %q, want write", d.KeyScope)
	}
}

func TestInspect_FullPowerOrgKeyHintPromotes(t *testing.T) {
	srv := inspectServer(t)
	d := Inspect(client.New("as_org_k", srv.URL, "proj", "env_s"), "as_org_k", nil, true)
	if d.Hint == nil || d.Hint.NextStep != "Publish to staging, then promote: `airstrings promote --to production`" {
		t.Errorf("hint = %+v", d.Hint)
	}
}
