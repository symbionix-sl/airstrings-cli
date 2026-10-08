package workspace

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func tokenServer(t *testing.T, replies ...string) (*httptest.Server, *int32) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/cli/auth/token":
			i := int(atomic.AddInt32(&n, 1)) - 1
			if i >= len(replies) {
				i = len(replies) - 1
			}
			w.Header().Set("Content-Type", "application/json")
			if strings.HasPrefix(replies[i], "{\"api_key\"") {
				w.Write([]byte(replies[i]))
				return
			}
			w.WriteHeader(400)
			w.Write([]byte(replies[i]))
		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/v1/org/api-keys/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

const (
	pendingReply  = `{"error":{"code":"authorization_pending","message":"pending"}}`
	slowDownReply = `{"error":{"code":"slow_down","message":"slow down"}}`
	approvedReply = `{"api_key":"as_org_new","key_id":"ak_new","org_id":"org_1","org_name":"Acme","full_power":true}`
)

func pendingFor(baseURL string) *PendingLogin {
	return &PendingLogin{BaseURL: baseURL, DeviceCode: "dc", UserCode: "BCDF-GHJK", Interval: 5, ExpiresAt: time.Now().Add(10 * time.Minute)}
}

func recorder(slept *[]time.Duration) func(time.Duration) string {
	return func(d time.Duration) string { *slept = append(*slept, d); return "" }
}

func TestPollLogin_PendingThenApproved(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv, _ := tokenServer(t, pendingReply, pendingReply, approvedReply)
	var slept []time.Duration
	key, err := PollLogin(pendingFor(srv.URL), time.Minute, recorder(&slept))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key.APIKey != "as_org_new" || len(slept) != 2 || slept[0] != 5*time.Second {
		t.Errorf("key %+v, slept %v", key, slept)
	}
}

func TestPollLogin_SlowDownAddsFiveSeconds(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv, _ := tokenServer(t, slowDownReply, approvedReply)
	var slept []time.Duration
	if _, err := PollLogin(pendingFor(srv.URL), time.Minute, recorder(&slept)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(slept) != 1 || slept[0] != 10*time.Second {
		t.Errorf("slept %v, want [10s]", slept)
	}
}

func TestLoginRerunPollsUpTo90sThenExits9(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv, n := tokenServer(t, pendingReply)
	var slept []time.Duration
	_, err := PollLogin(pendingFor(srv.URL), PollBudget, recorder(&slept))
	if !errors.Is(err, ErrLoginPending) {
		t.Fatalf("err = %v, want ErrLoginPending", err)
	}
	var total time.Duration
	for _, d := range slept {
		total += d
	}
	if total > 90*time.Second || total < 85*time.Second || atomic.LoadInt32(n) != int32(len(slept))+1 {
		t.Errorf("slept %v (total %v) over %d polls", slept, total, atomic.LoadInt32(n))
	}
}

func TestLoginRerunSlowDownStillExits9(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv, _ := tokenServer(t, slowDownReply)
	var slept []time.Duration
	if _, err := PollLogin(pendingFor(srv.URL), PollBudget, recorder(&slept)); !errors.Is(err, ErrLoginPending) {
		t.Fatalf("err = %v, want ErrLoginPending", err)
	}
	var total time.Duration
	for _, d := range slept {
		total += d
	}
	if total > 90*time.Second {
		t.Errorf("slept %v past the 90s budget", total)
	}
}

func TestPollLogin_ExpiredStops(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv, n := tokenServer(t, pendingReply)
	p := pendingFor(srv.URL)
	p.ExpiresAt = time.Now().Add(-time.Second)
	_, err := PollLogin(p, time.Until(p.ExpiresAt), func(time.Duration) string { return "" })
	if !errors.Is(err, ErrLoginPending) || atomic.LoadInt32(n) != 1 {
		t.Errorf("err = %v after %d polls", err, atomic.LoadInt32(n))
	}
}

func TestPollLogin_ExpiredTokenReturnsErrLoginExpired(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv, _ := tokenServer(t, `{"error":{"code":"expired_token","message":"expired","next_step":"Run airstrings login again"}}`)
	SaveCreds(&Creds{Pending: pendingFor(srv.URL)})
	_, err := PollLogin(pendingFor(srv.URL), time.Minute, func(time.Duration) string { return "" })
	if !errors.Is(err, ErrLoginExpired) {
		t.Fatalf("err = %v, want ErrLoginExpired", err)
	}
	if creds, _ := LoadCreds(); creds.Pending != nil {
		t.Error("expired pending login kept")
	}
}

func TestPendingMessageSaysOwnerMustApprove(t *testing.T) {
	if s := PendingNextStep("https://u"); s != "Approve in the browser at https://u, then run the same command again" {
		t.Errorf("PendingNextStep = %q", s)
	}
}

func TestLoginSavesKeyBeforeSetup(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv, _ := tokenServer(t, approvedReply)
	SaveCreds(&Creds{Pending: pendingFor(srv.URL)})
	if _, err := PollOnce(pendingFor(srv.URL), ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	creds, _ := LoadCreds()
	k := creds.OrgKey(srv.URL, "")
	if k == nil || k.APIKey != "as_org_new" || k.KeyID != "ak_new" || k.OrgID != "org_1" || !k.FullPower || creds.Pending != nil {
		t.Errorf("stored creds = %+v", creds)
	}
}

func TestReloginRevokesPreviousStoredKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var revoked, revokedWith string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			revoked, revokedWith = r.URL.Path, r.Header.Get("X-API-Key")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Write([]byte(approvedReply))
	}))
	defer srv.Close()
	creds := &Creds{}
	creds.SetOrgKey(OrgKey{BaseURL: srv.URL, OrgID: "org_1", KeyID: "ak_old", APIKey: "as_org_old"})
	SaveCreds(creds)

	if _, err := PollOnce(pendingFor(srv.URL), ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if revoked != "/v1/org/api-keys/ak_old" || revokedWith != "as_org_old" {
		t.Errorf("revoked %q with %q", revoked, revokedWith)
	}
}

func TestLoginOtherOrgKeepsBothKeysAndRevokesNothing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv, _ := tokenServer(t, approvedReply)
	revoked := false
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			revoked = true
		}
		w.Write([]byte(approvedReply))
	})
	creds := &Creds{}
	creds.SetOrgKey(OrgKey{BaseURL: srv.URL, OrgID: "org_a", KeyID: "ak_a", APIKey: "as_org_a", CreatedAt: time.Now().Add(-time.Hour)})
	SaveCreds(creds)

	if _, err := PollOnce(pendingFor(srv.URL), ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if revoked {
		t.Error("logging in to another org revoked the stored key")
	}
	loaded, _ := LoadCreds()
	if a, b := loaded.OrgKey(srv.URL, "org_a"), loaded.OrgKey(srv.URL, "org_1"); a == nil || a.APIKey != "as_org_a" || b == nil || b.APIKey != "as_org_new" {
		t.Errorf("keys = %+v", loaded.OrgKeys)
	}
	if k := loaded.OrgKey(srv.URL, ""); k == nil || k.OrgID != "org_1" {
		t.Errorf("no-org lookup = %+v, want the most recent (org_1)", k)
	}
	if loaded.Active[srv.URL] != "org_1" {
		t.Errorf("active = %v, want the newly approved org", loaded.Active)
	}
}

func TestStartLogin_ReadOnlyStoreFailsBeforeStart(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()
	dir := t.TempDir()
	os.Chmod(dir, 0500)
	t.Cleanup(func() { os.Chmod(dir, 0700) })
	t.Setenv("XDG_CONFIG_HOME", dir)

	if _, _, _, err := StartLogin(srv.URL, "test", false); !errors.Is(err, ErrCredStore) {
		t.Fatalf("err = %v, want ErrCredStore", err)
	}
	if hits != 0 {
		t.Errorf("start called %d times before the store was known writable", hits)
	}
}

func TestStartLogin_ReusesUnexpiredPending(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write([]byte(`{"device_code":"dc","user_code":"BCDF-GHJK","verification_uri_complete":"https://app/cli/approve?code=BCDF-GHJK","interval":5,"expires_in":600}`))
	}))
	defer srv.Close()

	p1, fresh1, _, err := StartLogin(srv.URL, "test", false)
	if err != nil || !fresh1 {
		t.Fatalf("first start: %+v %v %v", p1, fresh1, err)
	}
	p2, fresh2, _, _ := StartLogin(srv.URL, "test", false)
	if fresh2 || p2.DeviceCode != "dc" || hits != 1 {
		t.Errorf("second start fresh=%v hits=%d", fresh2, hits)
	}
}
