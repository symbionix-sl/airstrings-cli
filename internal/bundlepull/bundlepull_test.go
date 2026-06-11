package bundlepull

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
	"github.com/symbionix-sl/airstrings-cli/internal/workspace"
)

const (
	testProjectID = "proj_a1b2c3d4e5f6"
	testEnvID     = "env_x1y2z3w4v5u6"
	testOrgID     = "org_a1b2c3d4e5f6"
)

type keypair struct {
	pub  ed25519.PublicKey
	priv ed25519.PrivateKey
}

func newKeypair(t *testing.T) keypair {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return keypair{pub: pub, priv: priv}
}

func canonicalFixture(projectID, locale string, revision int, createdAt string, strs map[string][2]string) string {
	keys := make([]string, 0, len(strs))
	for k := range strs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf(`%q:{"format":%q,"value":%q}`, k, strs[k][1], strs[k][0]))
	}
	return fmt.Sprintf(`{"format_version":1,"project_id":%q,"locale":%q,"revision":%d,"created_at":%q,"strings":{%s}}`,
		projectID, locale, revision, createdAt, strings.Join(parts, ","))
}

func signedBundle(t *testing.T, kp keypair, projectID, locale string, revision int, createdAt string, strs map[string][2]string) []byte {
	t.Helper()
	canonical := canonicalFixture(projectID, locale, revision, createdAt, strs)
	sig := ed25519.Sign(kp.priv, []byte(canonical))
	stringsObj := make(map[string]map[string]string, len(strs))
	for k, v := range strs {
		stringsObj[k] = map[string]string{"value": v[0], "format": v[1]}
	}
	data, err := json.Marshal(map[string]any{
		"format_version": 1,
		"project_id":     projectID,
		"locale":         locale,
		"revision":       revision,
		"created_at":     createdAt,
		"key_id":         base64.StdEncoding.EncodeToString(kp.pub),
		"signature":      base64.RawURLEncoding.EncodeToString(sig),
		"strings":        stringsObj,
	})
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	return data
}

func mutateBundle(t *testing.T, data []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal bundle: %v", err)
	}
	mutate(m)
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	return out
}

func sampleStrings() map[string][2]string {
	return map[string][2]string{
		"onboarding.welcome_title": {"Welcome to Acme", "text"},
		"items.count":              {"{count, plural, one {# item} other {# items}}", "icu"},
	}
}

type cdnRequest struct {
	path   string
	query  string
	apiKey string
}

type cdnServer struct {
	mu       sync.Mutex
	requests []cdnRequest
	srv      *httptest.Server
}

func newCDN(t *testing.T, respond func(path string, hit int) (int, []byte)) *cdnServer {
	t.Helper()
	c := &cdnServer{}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		hit := 0
		for _, req := range c.requests {
			if req.path == r.URL.Path {
				hit++
			}
		}
		c.requests = append(c.requests, cdnRequest{path: r.URL.Path, query: r.URL.RawQuery, apiKey: r.Header.Get("X-API-Key")})
		c.mu.Unlock()
		status, body := respond(r.URL.Path, hit)
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write(body)
	}))
	t.Cleanup(c.srv.Close)
	return c
}

func (c *cdnServer) hits(path string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, req := range c.requests {
		if req.path == path {
			n++
		}
	}
	return n
}

func (c *cdnServer) all() []cdnRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]cdnRequest(nil), c.requests...)
}

func newAPI(t *testing.T, metas []client.BundleStatus) *client.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/v1/projects/" + testProjectID + "/environments/" + testEnvID + "/bundles"
		if r.URL.Path != want {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": metas})
	}))
	t.Cleanup(srv.Close)
	return client.New("test-key", srv.URL, testProjectID, testEnvID)
}

func bundleMeta(locale string, revision int, createdAt, cdnURL string) client.BundleStatus {
	return client.BundleStatus{
		ProjectID: testProjectID,
		Locale:    locale,
		Revision:  revision,
		CreatedAt: createdAt,
		CDNURL:    cdnURL,
	}
}

func cdnPath(locale string) string {
	return "/" + testOrgID + "/" + testProjectID + "/" + testEnvID + "/" + locale + "/bundle.json"
}

func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	snap := make(map[string]string)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		snap[e.Name()] = string(data)
	}
	return snap
}

func writePrevManifest(t *testing.T, dir string, m Manifest) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

func readManifestFile(t *testing.T, dir string) Manifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	return m
}

func TestPull_HappyPath(t *testing.T) {
	kp := newKeypair(t)
	enBytes := signedBundle(t, kp, testProjectID, "en-US", 42, "2026-06-09T18:00:00Z", sampleStrings())
	jaBytes := signedBundle(t, kp, testProjectID, "ja", 7, "2026-06-08T10:00:00Z", map[string][2]string{"greeting": {"こんにちは", "text"}})

	cdn := newCDN(t, func(path string, hit int) (int, []byte) {
		switch path {
		case cdnPath("en-US"):
			return 200, enBytes
		case cdnPath("ja"):
			return 200, jaBytes
		}
		return 404, nil
	})
	c := newAPI(t, []client.BundleStatus{
		bundleMeta("ja", 7, "2026-06-08T10:00:00Z", cdn.srv.URL+cdnPath("ja")),
		bundleMeta("en-US", 42, "2026-06-09T18:00:00Z", cdn.srv.URL+cdnPath("en-US")),
	})
	dir := filepath.Join(t.TempDir(), "airstrings", "bundles")

	before := time.Now().UTC().Add(-2 * time.Second)
	res, err := Pull(c, Options{Dir: dir, EnvName: "production", CLIVersion: "1.5.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.FirstPull {
		t.Error("expected FirstPull on fresh directory")
	}
	if res.Dir != dir || res.EnvID != testEnvID {
		t.Errorf("unexpected result: %+v", res)
	}
	wantPulled := []PulledBundle{
		{Locale: "en-US", Revision: 42, File: "en-US.json"},
		{Locale: "ja", Revision: 7, File: "ja.json"},
	}
	if !reflect.DeepEqual(res.Pulled, wantPulled) {
		t.Errorf("pulled = %+v, want %+v", res.Pulled, wantPulled)
	}

	gotEn, err := os.ReadFile(filepath.Join(dir, "en-US.json"))
	if err != nil {
		t.Fatalf("read en-US.json: %v", err)
	}
	if !bytes.Equal(gotEn, enBytes) {
		t.Error("en-US.json is not byte-identical to served bundle")
	}
	gotJa, err := os.ReadFile(filepath.Join(dir, "ja.json"))
	if err != nil {
		t.Fatalf("read ja.json: %v", err)
	}
	if !bytes.Equal(gotJa, jaBytes) {
		t.Error("ja.json is not byte-identical to served bundle")
	}

	m := readManifestFile(t, dir)
	if m.ManifestVersion != 1 {
		t.Errorf("manifest_version = %d, want 1", m.ManifestVersion)
	}
	ts, err := time.Parse(time.RFC3339, m.GeneratedAt)
	if err != nil {
		t.Errorf("generated_at not RFC3339: %v", err)
	}
	if !strings.HasSuffix(m.GeneratedAt, "Z") {
		t.Errorf("generated_at not UTC: %q", m.GeneratedAt)
	}
	if ts.Before(before) {
		t.Errorf("generated_at too old: %q", m.GeneratedAt)
	}
	if m.CLIVersion != "1.5.0" {
		t.Errorf("cli_version = %q", m.CLIVersion)
	}
	if m.OrgID != testOrgID {
		t.Errorf("org_id = %q, want %q", m.OrgID, testOrgID)
	}
	if m.ProjectID != testProjectID || m.EnvID != testEnvID || m.EnvName != "production" {
		t.Errorf("unexpected provenance: %+v", m)
	}
	wantEntries := []ManifestEntry{
		{Locale: "en-US", File: "en-US.json", Revision: 42, CreatedAt: "2026-06-09T18:00:00Z"},
		{Locale: "ja", File: "ja.json", Revision: 7, CreatedAt: "2026-06-08T10:00:00Z"},
	}
	if !reflect.DeepEqual(m.Bundles, wantEntries) {
		t.Errorf("manifest bundles = %+v, want %+v", m.Bundles, wantEntries)
	}

	for _, req := range cdn.all() {
		if req.apiKey != "" {
			t.Errorf("CDN request %s carried an API key", req.path)
		}
	}
}

func TestPull_ManifestOmitsOrgIDWhenUnderivable(t *testing.T) {
	kp := newKeypair(t)
	enBytes := signedBundle(t, kp, testProjectID, "en-US", 1, "2026-06-09T18:00:00Z", sampleStrings())

	cdn := newCDN(t, func(path string, hit int) (int, []byte) {
		if path == "/files/en-US.json" {
			return 200, enBytes
		}
		return 404, nil
	})
	c := newAPI(t, []client.BundleStatus{
		bundleMeta("en-US", 1, "2026-06-09T18:00:00Z", cdn.srv.URL+"/files/en-US.json"),
	})
	dir := filepath.Join(t.TempDir(), "bundles")

	if _, err := Pull(c, Options{Dir: dir, EnvName: "production", CLIVersion: "dev"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if _, ok := m["org_id"]; ok {
		t.Error("org_id present despite underivable CDN path")
	}
}

func TestPull_TamperedBundleAborts(t *testing.T) {
	kp := newKeypair(t)
	enBytes := signedBundle(t, kp, testProjectID, "en-US", 2, "2026-06-09T18:00:00Z", sampleStrings())
	jaBytes := signedBundle(t, kp, testProjectID, "ja", 5, "2026-06-09T18:00:00Z", map[string][2]string{"greeting": {"こんにちは", "text"}})
	tamperedJa := mutateBundle(t, jaBytes, func(m map[string]any) {
		m["strings"].(map[string]any)["greeting"].(map[string]any)["value"] = "tampered"
	})

	cdn := newCDN(t, func(path string, hit int) (int, []byte) {
		switch path {
		case cdnPath("en-US"):
			return 200, enBytes
		case cdnPath("ja"):
			return 200, tamperedJa
		}
		return 404, nil
	})
	c := newAPI(t, []client.BundleStatus{
		bundleMeta("en-US", 2, "2026-06-09T18:00:00Z", cdn.srv.URL+cdnPath("en-US")),
		bundleMeta("ja", 5, "2026-06-09T18:00:00Z", cdn.srv.URL+cdnPath("ja")),
	})

	dir := filepath.Join(t.TempDir(), "bundles")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "en-US.json"), []byte("old-en"), 0644); err != nil {
		t.Fatal(err)
	}
	writePrevManifest(t, dir, Manifest{
		ManifestVersion: 1,
		ProjectID:       testProjectID,
		EnvID:           testEnvID,
		EnvName:         "production",
		Bundles:         []ManifestEntry{{Locale: "en-US", File: "en-US.json", Revision: 1, CreatedAt: "2026-06-01T00:00:00Z"}},
	})
	before := snapshotDir(t, dir)

	_, err := Pull(c, Options{Dir: dir, EnvName: "production", CLIVersion: "dev"})
	if err == nil {
		t.Fatal("expected error for tampered bundle")
	}
	if !strings.Contains(err.Error(), "signature") {
		t.Errorf("error %q does not mention signature", err)
	}

	after := snapshotDir(t, dir)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("directory changed after failed pull:\nbefore: %v\nafter: %v", before, after)
	}
}

func TestPull_RevisionMismatchRetriesWithCacheBust(t *testing.T) {
	kp := newKeypair(t)
	stale := signedBundle(t, kp, testProjectID, "en-US", 41, "2026-06-01T00:00:00Z", sampleStrings())
	fresh := signedBundle(t, kp, testProjectID, "en-US", 42, "2026-06-09T18:00:00Z", sampleStrings())

	cdn := newCDN(t, func(path string, hit int) (int, []byte) {
		if path != cdnPath("en-US") {
			return 404, nil
		}
		if hit == 0 {
			return 200, stale
		}
		return 200, fresh
	})
	c := newAPI(t, []client.BundleStatus{
		bundleMeta("en-US", 42, "2026-06-09T18:00:00Z", cdn.srv.URL+cdnPath("en-US")),
	})
	dir := filepath.Join(t.TempDir(), "bundles")

	res, err := Pull(c, Options{Dir: dir, EnvName: "production", CLIVersion: "dev"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := cdn.hits(cdnPath("en-US")); got != 2 {
		t.Errorf("CDN hits = %d, want 2", got)
	}
	reqs := cdn.all()
	if reqs[0].query != "" {
		t.Errorf("first request had query %q, want none", reqs[0].query)
	}
	if reqs[1].query == "" {
		t.Error("retry request missing cache-busting query parameter")
	}
	got, err := os.ReadFile(filepath.Join(dir, "en-US.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, fresh) {
		t.Error("written bundle is not the retried fresh artifact")
	}
	if res.Pulled[0].Revision != 42 {
		t.Errorf("revision = %d, want 42", res.Pulled[0].Revision)
	}
}

func TestPull_RevisionMismatchPersistentAborts(t *testing.T) {
	kp := newKeypair(t)
	stale := signedBundle(t, kp, testProjectID, "en-US", 41, "2026-06-01T00:00:00Z", sampleStrings())

	cdn := newCDN(t, func(path string, hit int) (int, []byte) {
		if path == cdnPath("en-US") {
			return 200, stale
		}
		return 404, nil
	})
	c := newAPI(t, []client.BundleStatus{
		bundleMeta("en-US", 42, "2026-06-09T18:00:00Z", cdn.srv.URL+cdnPath("en-US")),
	})
	dir := filepath.Join(t.TempDir(), "bundles")

	_, err := Pull(c, Options{Dir: dir, EnvName: "production", CLIVersion: "dev"})
	if err == nil {
		t.Fatal("expected error for persistent revision mismatch")
	}
	if !strings.Contains(err.Error(), "revision") {
		t.Errorf("error %q does not mention revision", err)
	}
	if got := cdn.hits(cdnPath("en-US")); got != 2 {
		t.Errorf("CDN hits = %d, want exactly 2 (one retry)", got)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("output directory created despite failed pull")
	}
}

func TestPull_MirrorSemantics(t *testing.T) {
	kp := newKeypair(t)
	enBytes := signedBundle(t, kp, testProjectID, "en-US", 3, "2026-06-09T18:00:00Z", sampleStrings())

	cdn := newCDN(t, func(path string, hit int) (int, []byte) {
		if path == cdnPath("en-US") {
			return 200, enBytes
		}
		return 404, nil
	})
	c := newAPI(t, []client.BundleStatus{
		bundleMeta("en-US", 3, "2026-06-09T18:00:00Z", cdn.srv.URL+cdnPath("en-US")),
	})

	dir := filepath.Join(t.TempDir(), "bundles")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"en-US.json": "old-en",
		"de.json":    "old-de",
		"random.txt": "keep-me",
		"other.json": "keep-me-too",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writePrevManifest(t, dir, Manifest{
		ManifestVersion: 1,
		ProjectID:       testProjectID,
		EnvID:           testEnvID,
		EnvName:         "production",
		Bundles: []ManifestEntry{
			{Locale: "de", File: "de.json", Revision: 1, CreatedAt: "2026-06-01T00:00:00Z"},
			{Locale: "en-US", File: "en-US.json", Revision: 2, CreatedAt: "2026-06-01T00:00:00Z"},
		},
	})

	res, err := Pull(c, Options{Dir: dir, EnvName: "production", CLIVersion: "dev"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.FirstPull {
		t.Error("FirstPull true despite previous manifest")
	}

	if _, err := os.Stat(filepath.Join(dir, "de.json")); !os.IsNotExist(err) {
		t.Error("de.json not deleted despite no longer being published")
	}
	for name, content := range map[string]string{"random.txt": "keep-me", "other.json": "keep-me-too"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != content {
			t.Errorf("unmanaged file %s touched: %q, %v", name, got, err)
		}
	}
	gotEn, err := os.ReadFile(filepath.Join(dir, "en-US.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotEn, enBytes) {
		t.Error("en-US.json not updated")
	}

	m := readManifestFile(t, dir)
	wantEntries := []ManifestEntry{{Locale: "en-US", File: "en-US.json", Revision: 3, CreatedAt: "2026-06-09T18:00:00Z"}}
	if !reflect.DeepEqual(m.Bundles, wantEntries) {
		t.Errorf("manifest bundles = %+v, want %+v", m.Bundles, wantEntries)
	}
}

func TestPull_NoChangeLeavesManifestUntouched(t *testing.T) {
	kp := newKeypair(t)
	enBytes := signedBundle(t, kp, testProjectID, "en-US", 3, "2026-06-09T18:00:00Z", sampleStrings())

	cdn := newCDN(t, func(path string, hit int) (int, []byte) {
		if path == cdnPath("en-US") {
			return 200, enBytes
		}
		return 404, nil
	})
	c := newAPI(t, []client.BundleStatus{
		bundleMeta("en-US", 3, "2026-06-09T18:00:00Z", cdn.srv.URL+cdnPath("en-US")),
	})
	dir := filepath.Join(t.TempDir(), "bundles")

	if _, err := Pull(c, Options{Dir: dir, EnvName: "production", CLIVersion: "1.5.0"}); err != nil {
		t.Fatalf("first pull: %v", err)
	}
	first, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	firstManifest := readManifestFile(t, dir)

	res, err := Pull(c, Options{Dir: dir, EnvName: "production", CLIVersion: "1.6.0"})
	if err != nil {
		t.Fatalf("second pull: %v", err)
	}
	if res.FirstPull {
		t.Error("FirstPull true on second pull")
	}
	second, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Errorf("manifest rewritten despite no upstream changes:\nfirst:  %s\nsecond: %s", first, second)
	}
	if got := readManifestFile(t, dir).GeneratedAt; got != firstManifest.GeneratedAt {
		t.Errorf("generated_at changed: %q -> %q", firstManifest.GeneratedAt, got)
	}
}

func TestPull_ChangedContentRewritesManifest(t *testing.T) {
	kp := newKeypair(t)
	rev3 := signedBundle(t, kp, testProjectID, "en-US", 3, "2026-06-09T18:00:00Z", sampleStrings())
	rev4 := signedBundle(t, kp, testProjectID, "en-US", 4, "2026-06-10T09:00:00Z", sampleStrings())

	cdn := newCDN(t, func(path string, hit int) (int, []byte) {
		if path != cdnPath("en-US") {
			return 404, nil
		}
		if hit == 0 {
			return 200, rev3
		}
		return 200, rev4
	})
	c3 := newAPI(t, []client.BundleStatus{
		bundleMeta("en-US", 3, "2026-06-09T18:00:00Z", cdn.srv.URL+cdnPath("en-US")),
	})
	c4 := newAPI(t, []client.BundleStatus{
		bundleMeta("en-US", 4, "2026-06-10T09:00:00Z", cdn.srv.URL+cdnPath("en-US")),
	})
	dir := filepath.Join(t.TempDir(), "bundles")

	if _, err := Pull(c3, Options{Dir: dir, EnvName: "production", CLIVersion: "dev"}); err != nil {
		t.Fatalf("first pull: %v", err)
	}
	first, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}

	before := time.Now().UTC().Add(-2 * time.Second)
	if _, err := Pull(c4, Options{Dir: dir, EnvName: "production", CLIVersion: "dev"}); err != nil {
		t.Fatalf("second pull: %v", err)
	}
	second, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Error("manifest not rewritten despite changed upstream revision")
	}

	m := readManifestFile(t, dir)
	wantEntries := []ManifestEntry{{Locale: "en-US", File: "en-US.json", Revision: 4, CreatedAt: "2026-06-10T09:00:00Z"}}
	if !reflect.DeepEqual(m.Bundles, wantEntries) {
		t.Errorf("manifest bundles = %+v, want %+v", m.Bundles, wantEntries)
	}
	ts, err := time.Parse(time.RFC3339, m.GeneratedAt)
	if err != nil {
		t.Errorf("generated_at not RFC3339: %v", err)
	}
	if !strings.HasSuffix(m.GeneratedAt, "Z") {
		t.Errorf("generated_at not UTC: %q", m.GeneratedAt)
	}
	if ts.Before(before) {
		t.Errorf("generated_at too old: %q", m.GeneratedAt)
	}
}

func TestPull_MalformedManifestRewritten(t *testing.T) {
	kp := newKeypair(t)
	enBytes := signedBundle(t, kp, testProjectID, "en-US", 3, "2026-06-09T18:00:00Z", sampleStrings())

	cdn := newCDN(t, func(path string, hit int) (int, []byte) {
		if path == cdnPath("en-US") {
			return 200, enBytes
		}
		return 404, nil
	})
	c := newAPI(t, []client.BundleStatus{
		bundleMeta("en-US", 3, "2026-06-09T18:00:00Z", cdn.srv.URL+cdnPath("en-US")),
	})
	dir := filepath.Join(t.TempDir(), "bundles")

	if _, err := Pull(c, Options{Dir: dir, EnvName: "production", CLIVersion: "dev"}); err != nil {
		t.Fatalf("first pull: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}

	res, err := Pull(c, Options{Dir: dir, EnvName: "production", CLIVersion: "dev"})
	if err != nil {
		t.Fatalf("second pull: %v", err)
	}
	if res.FirstPull {
		t.Error("FirstPull true despite existing manifest file")
	}

	m := readManifestFile(t, dir)
	wantEntries := []ManifestEntry{{Locale: "en-US", File: "en-US.json", Revision: 3, CreatedAt: "2026-06-09T18:00:00Z"}}
	if !reflect.DeepEqual(m.Bundles, wantEntries) {
		t.Errorf("manifest bundles = %+v, want %+v", m.Bundles, wantEntries)
	}
	if m.ManifestVersion != 1 {
		t.Errorf("manifest_version = %d, want 1", m.ManifestVersion)
	}
	if _, err := time.Parse(time.RFC3339, m.GeneratedAt); err != nil {
		t.Errorf("generated_at not RFC3339: %v", err)
	}
}

func TestPull_AtomicOnMidPullFailure(t *testing.T) {
	kp := newKeypair(t)
	deBytes := signedBundle(t, kp, testProjectID, "de", 4, "2026-06-09T18:00:00Z", sampleStrings())
	jaBytes := signedBundle(t, kp, testProjectID, "ja", 6, "2026-06-09T18:00:00Z", sampleStrings())

	cdn := newCDN(t, func(path string, hit int) (int, []byte) {
		switch path {
		case cdnPath("de"):
			return 200, deBytes
		case cdnPath("en-US"):
			return 500, nil
		case cdnPath("ja"):
			return 200, jaBytes
		}
		return 404, nil
	})
	c := newAPI(t, []client.BundleStatus{
		bundleMeta("de", 4, "2026-06-09T18:00:00Z", cdn.srv.URL+cdnPath("de")),
		bundleMeta("en-US", 9, "2026-06-09T18:00:00Z", cdn.srv.URL+cdnPath("en-US")),
		bundleMeta("ja", 6, "2026-06-09T18:00:00Z", cdn.srv.URL+cdnPath("ja")),
	})

	dir := filepath.Join(t.TempDir(), "bundles")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ja.json"), []byte("old-ja"), 0644); err != nil {
		t.Fatal(err)
	}
	writePrevManifest(t, dir, Manifest{
		ManifestVersion: 1,
		ProjectID:       testProjectID,
		EnvID:           testEnvID,
		EnvName:         "production",
		Bundles:         []ManifestEntry{{Locale: "ja", File: "ja.json", Revision: 5, CreatedAt: "2026-06-01T00:00:00Z"}},
	})
	before := snapshotDir(t, dir)

	_, err := Pull(c, Options{Dir: dir, EnvName: "production", CLIVersion: "dev"})
	if err == nil {
		t.Fatal("expected error when a locale download fails")
	}

	after := snapshotDir(t, dir)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("directory changed after failed pull:\nbefore: %v\nafter: %v", before, after)
	}
	parentEntries, err := os.ReadDir(filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range parentEntries {
		if strings.HasPrefix(e.Name(), ".airstrings-pull-") {
			t.Errorf("staging leftover: %s", e.Name())
		}
	}
}

func TestPull_LocaleFilter(t *testing.T) {
	kp := newKeypair(t)
	enBytes := signedBundle(t, kp, testProjectID, "en-US", 42, "2026-06-09T18:00:00Z", sampleStrings())
	oldJa := signedBundle(t, kp, testProjectID, "ja", 6, "2026-06-01T00:00:00Z", sampleStrings())

	cdn := newCDN(t, func(path string, hit int) (int, []byte) {
		if path == cdnPath("en-US") {
			return 200, enBytes
		}
		return 404, nil
	})
	c := newAPI(t, []client.BundleStatus{
		bundleMeta("en-US", 42, "2026-06-09T18:00:00Z", cdn.srv.URL+cdnPath("en-US")),
		bundleMeta("ja", 7, "2026-06-09T18:00:00Z", cdn.srv.URL+cdnPath("ja")),
	})

	dir := filepath.Join(t.TempDir(), "bundles")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ja.json"), oldJa, 0644); err != nil {
		t.Fatal(err)
	}
	writePrevManifest(t, dir, Manifest{
		ManifestVersion: 1,
		ProjectID:       testProjectID,
		EnvID:           testEnvID,
		EnvName:         "production",
		Bundles:         []ManifestEntry{{Locale: "ja", File: "ja.json", Revision: 6, CreatedAt: "2026-06-01T00:00:00Z"}},
	})

	res, err := Pull(c, Options{Dir: dir, Locale: "en-US", EnvName: "production", CLIVersion: "dev"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantPulled := []PulledBundle{{Locale: "en-US", Revision: 42, File: "en-US.json"}}
	if !reflect.DeepEqual(res.Pulled, wantPulled) {
		t.Errorf("pulled = %+v, want %+v", res.Pulled, wantPulled)
	}
	if got := cdn.hits(cdnPath("ja")); got != 0 {
		t.Errorf("ja fetched %d times despite --locale filter", got)
	}
	gotJa, err := os.ReadFile(filepath.Join(dir, "ja.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotJa, oldJa) {
		t.Error("ja.json modified by filtered pull")
	}

	m := readManifestFile(t, dir)
	wantEntries := []ManifestEntry{
		{Locale: "en-US", File: "en-US.json", Revision: 42, CreatedAt: "2026-06-09T18:00:00Z"},
		{Locale: "ja", File: "ja.json", Revision: 6, CreatedAt: "2026-06-01T00:00:00Z"},
	}
	if !reflect.DeepEqual(m.Bundles, wantEntries) {
		t.Errorf("manifest bundles = %+v, want %+v", m.Bundles, wantEntries)
	}
}

func TestPull_LocaleFilterUnknown(t *testing.T) {
	c := newAPI(t, []client.BundleStatus{
		bundleMeta("en-US", 1, "2026-06-09T18:00:00Z", "http://127.0.0.1/unused"),
	})
	_, err := Pull(c, Options{Dir: filepath.Join(t.TempDir(), "bundles"), Locale: "fr", EnvName: "production", CLIVersion: "dev"})
	if err == nil {
		t.Fatal("expected error for unpublished locale")
	}
	if !strings.Contains(err.Error(), "fr") {
		t.Errorf("error %q does not mention the locale", err)
	}
}

func TestPull_NoPublishedBundles(t *testing.T) {
	c := newAPI(t, nil)
	_, err := Pull(c, Options{Dir: filepath.Join(t.TempDir(), "bundles"), EnvName: "production", CLIVersion: "dev"})
	if err == nil {
		t.Fatal("expected error when nothing is published")
	}
}

func initWorkspace(t *testing.T) (string, string, *workspace.WorkspaceConfig) {
	t.Helper()
	root := t.TempDir()
	if err := workspace.Init(root, workspace.WorkspaceConfig{ProjectID: testProjectID, ActiveEnv: testEnvID}); err != nil {
		t.Fatalf("init workspace: %v", err)
	}
	wsDir := filepath.Join(root, ".airstrings")
	cfg, err := workspace.LoadConfig(wsDir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return root, wsDir, cfg
}

func TestResolveDir_Default(t *testing.T) {
	root, wsDir, cfg := initWorkspace(t)

	dir, err := ResolveDir(wsDir, cfg, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(root, "airstrings", "bundles")
	if dir != want {
		t.Errorf("dir = %q, want %q", dir, want)
	}
	raw, err := os.ReadFile(filepath.Join(wsDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "bundles_dir") {
		t.Error("default resolution must not persist bundles_dir")
	}
}

func TestResolveDir_ArgPersistedAndReused(t *testing.T) {
	_, wsDir, cfg := initWorkspace(t)
	t.Chdir(filepath.Dir(wsDir))
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	dir, err := ResolveDir(wsDir, cfg, filepath.Join("custom", "seed"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(cwd, "custom", "seed")
	if dir != want {
		t.Errorf("dir = %q, want %q", dir, want)
	}

	raw, err := os.ReadFile(filepath.Join(wsDir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"bundles_dir": "custom/seed"`) {
		t.Errorf("bundles_dir not persisted, config: %s", raw)
	}

	cfg2, err := workspace.LoadConfig(wsDir)
	if err != nil {
		t.Fatal(err)
	}
	dir2, err := ResolveDir(wsDir, cfg2, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir2 != dir {
		t.Errorf("bare re-run resolved %q, want persisted %q", dir2, dir)
	}
}

func TestResolveDir_RejectsInsideWorkspaceDir(t *testing.T) {
	_, wsDir, cfg := initWorkspace(t)

	if _, err := ResolveDir(wsDir, cfg, wsDir); err == nil {
		t.Error("expected error for dir equal to .airstrings")
	}
	if _, err := ResolveDir(wsDir, cfg, filepath.Join(wsDir, "bundles")); err == nil {
		t.Error("expected error for dir inside .airstrings")
	}
	if cfg.BundlesDir != "" {
		t.Errorf("rejected dir persisted: %q", cfg.BundlesDir)
	}

	cfg.BundlesDir = filepath.Join(".airstrings", "nested")
	if _, err := ResolveDir(wsDir, cfg, ""); err == nil {
		t.Error("expected error for persisted dir inside .airstrings")
	}
}

func TestResultJSONShape(t *testing.T) {
	res := &Result{
		Dir:    "/tmp/x",
		EnvID:  testEnvID,
		Pulled: []PulledBundle{{Locale: "en-US", Revision: 42, File: "en-US.json"}},
	}
	data, err := json.Marshal(res.JSON())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if len(m) != 3 {
		t.Errorf("unexpected keys: %v", m)
	}
	if m["dir"] != "/tmp/x" || m["env_id"] != testEnvID {
		t.Errorf("unexpected payload: %v", m)
	}
	bundles, ok := m["bundles"].([]any)
	if !ok || len(bundles) != 1 {
		t.Fatalf("unexpected bundles: %v", m["bundles"])
	}
	b := bundles[0].(map[string]any)
	if b["locale"] != "en-US" || b["revision"] != float64(42) || b["file"] != "en-US.json" {
		t.Errorf("unexpected bundle entry: %v", b)
	}
}

func TestCanonicalSignedContent_ContractExample(t *testing.T) {
	env := &bundleEnvelope{
		FormatVersion: 1,
		ProjectID:     "proj_a1b2c3d4e5f6",
		Locale:        "en-US",
		Revision:      42,
		CreatedAt:     "2026-02-25T14:30:00Z",
		Strings: map[string]bundleString{
			"onboarding.welcome_title": {Value: "Welcome to Acme", Format: "text"},
			"onboarding.welcome_body":  {Value: "Get started in minutes.", Format: "text"},
			"settings.language":        {Value: "Language", Format: "text"},
			"items.count":              {Value: "{count, plural, one {# item} other {# items}}", Format: "icu"},
			"error.network":            {Value: "Something went wrong. Please try again.", Format: "text"},
		},
	}
	want := `{"format_version":1,"project_id":"proj_a1b2c3d4e5f6","locale":"en-US","revision":42,"created_at":"2026-02-25T14:30:00Z","strings":{"error.network":{"format":"text","value":"Something went wrong. Please try again."},"items.count":{"format":"icu","value":"{count, plural, one {# item} other {# items}}"},"onboarding.welcome_body":{"format":"text","value":"Get started in minutes."},"onboarding.welcome_title":{"format":"text","value":"Welcome to Acme"},"settings.language":{"format":"text","value":"Language"}}}`
	if got := string(canonicalSignedContent(env)); got != want {
		t.Errorf("canonical content mismatch:\ngot:  %s\nwant: %s", got, want)
	}
}

func TestCanonicalJSON_SortedRecursiveNoWhitespace(t *testing.T) {
	v := map[string]any{
		"b": float64(2),
		"a": map[string]any{"z": true, "y": nil, "x": []any{"s", float64(1)}},
	}
	want := `{"a":{"x":["s",1],"y":null,"z":true},"b":2}`
	if got := string(canonicalJSON(v)); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestCanonicalJSON_IntegersBare(t *testing.T) {
	cases := map[any]string{
		float64(42):       "42",
		int(7):            "7",
		json.Number("42"): "42",
	}
	for in, want := range cases {
		if got := string(canonicalJSON(in)); got != want {
			t.Errorf("canonicalJSON(%v) = %s, want %s", in, got, want)
		}
	}
}

func TestCanonicalJSON_StringEscaping(t *testing.T) {
	in := "\"\\\nA\x01é<>&"
	want := `"\"\\\nA\` + "u0001" + `é<>&"`
	if got := string(canonicalJSON(in)); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func validMeta() client.BundleStatus {
	return client.BundleStatus{ProjectID: testProjectID, Locale: "en-US", Revision: 42, CreatedAt: "2026-06-09T18:00:00Z"}
}

func TestVerifyBundle_Valid(t *testing.T) {
	kp := newKeypair(t)
	data := signedBundle(t, kp, testProjectID, "en-US", 42, "2026-06-09T18:00:00Z", sampleStrings())
	if err := verifyBundle(data, validMeta()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyBundle_TamperedStrings(t *testing.T) {
	kp := newKeypair(t)
	data := signedBundle(t, kp, testProjectID, "en-US", 42, "2026-06-09T18:00:00Z", sampleStrings())
	data = mutateBundle(t, data, func(m map[string]any) {
		m["strings"].(map[string]any)["items.count"].(map[string]any)["value"] = "tampered"
	})
	err := verifyBundle(data, validMeta())
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Errorf("expected signature failure, got %v", err)
	}
}

func TestVerifyBundle_SignatureNotBase64URLNoPadding(t *testing.T) {
	kp := newKeypair(t)
	data := signedBundle(t, kp, testProjectID, "en-US", 42, "2026-06-09T18:00:00Z", sampleStrings())
	data = mutateBundle(t, data, func(m map[string]any) {
		m["signature"] = m["signature"].(string) + "=="
	})
	err := verifyBundle(data, validMeta())
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Errorf("expected signature decode failure, got %v", err)
	}
}

func TestVerifyBundle_SignatureWrongLength(t *testing.T) {
	kp := newKeypair(t)
	data := signedBundle(t, kp, testProjectID, "en-US", 42, "2026-06-09T18:00:00Z", sampleStrings())
	data = mutateBundle(t, data, func(m map[string]any) {
		m["signature"] = base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	})
	err := verifyBundle(data, validMeta())
	if err == nil || !strings.Contains(err.Error(), "64") {
		t.Errorf("expected 64-byte signature error, got %v", err)
	}
}

func TestVerifyBundle_KeyIDNotStdBase64(t *testing.T) {
	kp := newKeypair(t)
	data := signedBundle(t, kp, testProjectID, "en-US", 42, "2026-06-09T18:00:00Z", sampleStrings())
	data = mutateBundle(t, data, func(m map[string]any) {
		m["key_id"] = strings.Repeat("-_", 22)
	})
	err := verifyBundle(data, validMeta())
	if err == nil || !strings.Contains(err.Error(), "key_id") {
		t.Errorf("expected key_id decode failure, got %v", err)
	}
}

func TestVerifyBundle_KeyIDWrongLength(t *testing.T) {
	kp := newKeypair(t)
	data := signedBundle(t, kp, testProjectID, "en-US", 42, "2026-06-09T18:00:00Z", sampleStrings())
	data = mutateBundle(t, data, func(m map[string]any) {
		m["key_id"] = base64.StdEncoding.EncodeToString(make([]byte, 16))
	})
	err := verifyBundle(data, validMeta())
	if err == nil || !strings.Contains(err.Error(), "32") {
		t.Errorf("expected 32-byte key error, got %v", err)
	}
}

func TestVerifyBundle_FormatVersionRejected(t *testing.T) {
	kp := newKeypair(t)
	data := signedBundle(t, kp, testProjectID, "en-US", 42, "2026-06-09T18:00:00Z", sampleStrings())
	data = mutateBundle(t, data, func(m map[string]any) {
		m["format_version"] = 2
	})
	err := verifyBundle(data, validMeta())
	if err == nil || !strings.Contains(err.Error(), "format_version") {
		t.Errorf("expected format_version rejection, got %v", err)
	}
}

func TestVerifyBundle_ProjectIDMismatch(t *testing.T) {
	kp := newKeypair(t)
	data := signedBundle(t, kp, "proj_zzzzzzzzzzzz", "en-US", 42, "2026-06-09T18:00:00Z", sampleStrings())
	err := verifyBundle(data, validMeta())
	if err == nil || !strings.Contains(err.Error(), "project_id") {
		t.Errorf("expected project_id mismatch, got %v", err)
	}
}

func TestVerifyBundle_LocaleMismatch(t *testing.T) {
	kp := newKeypair(t)
	data := signedBundle(t, kp, testProjectID, "ja", 42, "2026-06-09T18:00:00Z", sampleStrings())
	err := verifyBundle(data, validMeta())
	if err == nil || !strings.Contains(err.Error(), "locale") {
		t.Errorf("expected locale mismatch, got %v", err)
	}
}

func TestVerifyBundle_RevisionMismatchTyped(t *testing.T) {
	kp := newKeypair(t)
	data := signedBundle(t, kp, testProjectID, "en-US", 41, "2026-06-09T18:00:00Z", sampleStrings())
	err := verifyBundle(data, validMeta())
	if err == nil {
		t.Fatal("expected revision mismatch error")
	}
	var rm *revisionMismatchError
	if !errors.As(err, &rm) {
		t.Fatalf("expected revisionMismatchError, got %T: %v", err, err)
	}
	if rm.got != 41 || rm.want != 42 {
		t.Errorf("mismatch detail = %+v", rm)
	}
}
