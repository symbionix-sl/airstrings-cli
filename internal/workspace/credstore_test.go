package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "airstrings-xdg")
	if err != nil {
		panic(err)
	}
	for _, e := range os.Environ() {
		if k, _, _ := strings.Cut(e, "="); strings.HasPrefix(k, "AIRSTRINGS_") {
			os.Unsetenv(k)
		}
	}
	os.Setenv("XDG_CONFIG_HOME", dir)
	os.Setenv("AIRSTRINGS_NO_BROWSER", "1")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestCredStore_PathHonorsXDG(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if got, want := CredentialsPath(), filepath.Join(dir, "airstrings", "credentials.json"); got != want {
		t.Errorf("CredentialsPath() = %q, want %q", got, want)
	}
}

func TestCredStore_FileIs0600DirIs0700(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	creds := &Creds{}
	creds.SetOrgKey(OrgKey{BaseURL: "https://api.example", APIKey: "as_org_a"})
	if err := SaveCreds(creds); err != nil {
		t.Fatalf("SaveCreds: %v", err)
	}
	fi, err := os.Stat(CredentialsPath())
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("file mode = %o, want 600", fi.Mode().Perm())
	}
	di, _ := os.Stat(filepath.Dir(CredentialsPath()))
	if di.Mode().Perm() != 0700 {
		t.Errorf("dir mode = %o, want 700", di.Mode().Perm())
	}
}

func TestCredStore_OneKeyPerBaseURL(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	creds := &Creds{}
	creds.SetOrgKey(OrgKey{BaseURL: "https://api.example", APIKey: "as_org_prod"})
	creds.SetOrgKey(OrgKey{BaseURL: "https://api-staging.example", APIKey: "as_org_stg"})
	creds.SetOrgKey(OrgKey{BaseURL: "https://api.example", APIKey: "as_org_prod2"})
	if err := SaveCreds(creds); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadCreds()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.OrgKeys) != 2 {
		t.Fatalf("got %d org keys, want 2", len(loaded.OrgKeys))
	}
	if k := loaded.OrgKey("https://api.example", ""); k == nil || k.APIKey != "as_org_prod2" {
		t.Errorf("prod key = %+v", k)
	}
	if k := loaded.OrgKey("https://api-staging.example", ""); k == nil || k.APIKey != "as_org_stg" {
		t.Errorf("staging key = %+v", k)
	}
}

func TestCredStore_DeleteOrgKey(t *testing.T) {
	creds := &Creds{}
	creds.SetOrgKey(OrgKey{BaseURL: "https://a", OrgID: "org_1", APIKey: "as_org_a1"})
	creds.SetOrgKey(OrgKey{BaseURL: "https://a", OrgID: "org_2", APIKey: "as_org_a2"})
	creds.SetOrgKey(OrgKey{BaseURL: "https://b", OrgID: "org_1", APIKey: "as_org_b"})
	creds.UseOrg("https://a", "org_1")
	if !creds.DeleteOrgKey("https://a", "org_1") || creds.OrgKey("https://a", "org_1") != nil || creds.OrgKey("https://a", "org_2") == nil || creds.OrgKey("https://b", "org_1") == nil {
		t.Errorf("unexpected keys after delete: %+v", creds.OrgKeys)
	}
	if creds.Active["https://a"] != "" {
		t.Errorf("active still points at the deleted org: %v", creds.Active)
	}
	if creds.DeleteOrgKey("https://a", "org_1") {
		t.Error("second delete reported a key")
	}
}

func TestCredStore_LoadsSingleKeyFileFromOlderCLI(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	os.MkdirAll(filepath.Dir(CredentialsPath()), 0700)
	old := `{"org_keys":[{"base_url":"https://a","org_id":"org_a","org_name":"A","key_id":"ak_a","api_key":"as_org_a","full_power":false,"created_at":"2026-10-01T00:00:00Z"}]}`
	os.WriteFile(CredentialsPath(), []byte(old), 0600)
	creds, err := LoadCreds()
	if err != nil {
		t.Fatal(err)
	}
	if k := creds.OrgKey("https://a", "org_a"); k == nil || k.APIKey != "as_org_a" {
		t.Errorf("by org = %+v", k)
	}
	if k := creds.OrgKey("https://a", ""); k == nil || k.APIKey != "as_org_a" {
		t.Errorf("without org = %+v", k)
	}
	if k := creds.OrgKey("https://a", "org_b"); k != nil {
		t.Errorf("other org = %+v, want nil", k)
	}
}

func TestCredStore_MissingFileIsEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	creds, err := LoadCreds()
	if err != nil || len(creds.OrgKeys) != 0 || creds.Pending != nil {
		t.Errorf("LoadCreds() = %+v, %v", creds, err)
	}
}

func TestCredStore_PendingRoundTripAndExpiry(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	creds := &Creds{Pending: &PendingLogin{BaseURL: "https://a", DeviceCode: "dc", UserCode: "BCDF-GHJK", ExpiresAt: time.Now().Add(time.Minute)}}
	if err := SaveCreds(creds); err != nil {
		t.Fatal(err)
	}
	loaded, _ := LoadCreds()
	if p := loaded.PendingFor("https://a"); p == nil || p.DeviceCode != "dc" {
		t.Errorf("pending = %+v", loaded.Pending)
	}
	if loaded.PendingFor("https://b") != nil {
		t.Error("pending returned for another base URL")
	}
	loaded.Pending.ExpiresAt = time.Now().Add(-time.Second)
	if loaded.PendingFor("https://a") != nil {
		t.Error("expired pending returned")
	}
}

func TestCredStore_ReadOnlyDirFailsBeforeStart(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0700) })
	t.Setenv("XDG_CONFIG_HOME", dir)

	err := SaveCreds(&Creds{})
	if !errors.Is(err, ErrCredStore) {
		t.Fatalf("SaveCreds() = %v, want ErrCredStore", err)
	}
}
