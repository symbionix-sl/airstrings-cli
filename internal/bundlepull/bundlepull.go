package bundlepull

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/symbionix-sl/airstrings-cli/internal/client"
	"github.com/symbionix-sl/airstrings-cli/internal/workspace"
)

const (
	manifestFile   = "manifest.json"
	maxBundleBytes = 5 << 20
)

type Options struct {
	Dir        string
	Locale     string
	EnvName    string
	CLIVersion string
}

type PulledBundle struct {
	Locale   string `json:"locale"`
	Revision int    `json:"revision"`
	File     string `json:"file"`
}

type Result struct {
	Dir       string
	EnvID     string
	FirstPull bool
	Pulled    []PulledBundle
}

type JSONResult struct {
	Dir     string         `json:"dir"`
	EnvID   string         `json:"env_id"`
	Bundles []PulledBundle `json:"bundles"`
}

func (r *Result) JSON() JSONResult {
	return JSONResult{Dir: r.Dir, EnvID: r.EnvID, Bundles: r.Pulled}
}

type Manifest struct {
	ManifestVersion int             `json:"manifest_version"`
	GeneratedAt     string          `json:"generated_at"`
	CLIVersion      string          `json:"cli_version"`
	OrgID           string          `json:"org_id,omitempty"`
	ProjectID       string          `json:"project_id"`
	EnvID           string          `json:"env_id"`
	EnvName         string          `json:"env_name"`
	Bundles         []ManifestEntry `json:"bundles"`
}

type ManifestEntry struct {
	Locale    string `json:"locale"`
	File      string `json:"file"`
	Revision  int    `json:"revision"`
	CreatedAt string `json:"created_at"`
}

func (m *Manifest) equivalent(other *Manifest) bool {
	if m.ManifestVersion != other.ManifestVersion ||
		m.OrgID != other.OrgID ||
		m.ProjectID != other.ProjectID ||
		m.EnvID != other.EnvID ||
		m.EnvName != other.EnvName ||
		len(m.Bundles) != len(other.Bundles) {
		return false
	}
	for i, e := range m.Bundles {
		if e != other.Bundles[i] {
			return false
		}
	}
	return true
}

func ResolveDir(wsDir string, cfg *workspace.WorkspaceConfig, arg string) (string, error) {
	root := filepath.Dir(wsDir)
	if arg != "" {
		dir := arg
		if !filepath.IsAbs(dir) {
			cwd, err := os.Getwd()
			if err != nil {
				return "", err
			}
			dir = filepath.Join(cwd, dir)
		}
		dir = filepath.Clean(dir)
		if err := guardDir(dir, wsDir); err != nil {
			return "", err
		}
		persist := dir
		if rel, err := filepath.Rel(root, dir); err == nil && !escapes(rel) {
			persist = rel
		}
		if cfg.BundlesDir != persist {
			cfg.BundlesDir = persist
			if err := workspace.SaveConfig(wsDir, cfg); err != nil {
				return "", fmt.Errorf("save workspace config: %w", err)
			}
		}
		return dir, nil
	}

	configured := cfg.BundlesDir
	if configured == "" {
		configured = filepath.Join("airstrings", "bundles")
	}
	dir := configured
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	dir = filepath.Clean(dir)
	if err := guardDir(dir, wsDir); err != nil {
		return "", err
	}
	return dir, nil
}

func escapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func guardDir(dir, wsDir string) error {
	rel, err := filepath.Rel(wsDir, dir)
	if err == nil && !escapes(rel) {
		return fmt.Errorf("output directory %s is inside %s — bundles must live outside the workspace config dir", dir, wsDir)
	}
	return nil
}

func Pull(c *client.Client, opts Options) (*Result, error) {
	metas, err := c.ListBundles()
	if err != nil {
		return nil, fmt.Errorf("list bundles: %w", err)
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].Locale < metas[j].Locale })

	published := make(map[string]bool, len(metas))
	for _, m := range metas {
		published[m.Locale] = true
	}

	selected := metas
	if opts.Locale != "" {
		selected = nil
		for _, m := range metas {
			if m.Locale == opts.Locale {
				selected = append(selected, m)
			}
		}
		if len(selected) == 0 {
			return nil, fmt.Errorf("no published bundle for locale %q", opts.Locale)
		}
	}
	if len(selected) == 0 {
		return nil, errors.New("no published bundles for this environment")
	}
	for _, m := range selected {
		if !validLocaleName(m.Locale) {
			return nil, fmt.Errorf("unsafe locale name %q", m.Locale)
		}
	}

	hc := &http.Client{Timeout: 30 * time.Second}
	files := make([]stagedFile, 0, len(selected))
	pulled := make([]PulledBundle, 0, len(selected))
	entries := make([]ManifestEntry, 0, len(selected))
	pulledSet := make(map[string]bool, len(selected))
	for _, m := range selected {
		data, err := fetchVerified(hc, m)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.Locale, err)
		}
		name := m.Locale + ".json"
		files = append(files, stagedFile{name: name, data: data})
		pulled = append(pulled, PulledBundle{Locale: m.Locale, Revision: m.Revision, File: name})
		entries = append(entries, ManifestEntry{Locale: m.Locale, File: name, Revision: m.Revision, CreatedAt: m.CreatedAt})
		pulledSet[m.Locale] = true
	}

	prev, prevExists := readManifest(filepath.Join(opts.Dir, manifestFile))

	var stale []string
	if prev != nil {
		for _, e := range prev.Bundles {
			if e.File != e.Locale+".json" || !validLocaleName(e.Locale) || pulledSet[e.Locale] {
				continue
			}
			if !published[e.Locale] {
				stale = append(stale, e.File)
				continue
			}
			if _, err := os.Stat(filepath.Join(opts.Dir, e.File)); err == nil {
				entries = append(entries, e)
			}
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Locale < entries[j].Locale })

	manifest := &Manifest{
		ManifestVersion: 1,
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		CLIVersion:      opts.CLIVersion,
		OrgID:           deriveOrgID(selected, c.ProjectID(), c.EnvID()),
		ProjectID:       c.ProjectID(),
		EnvID:           c.EnvID(),
		EnvName:         opts.EnvName,
		Bundles:         entries,
	}

	install := manifest
	if prev != nil && manifest.equivalent(prev) {
		install = nil
	}

	if err := applyStaged(opts.Dir, files, install, stale); err != nil {
		return nil, err
	}

	return &Result{Dir: opts.Dir, EnvID: c.EnvID(), FirstPull: !prevExists, Pulled: pulled}, nil
}

type stagedFile struct {
	name string
	data []byte
}

func applyStaged(dir string, files []stagedFile, manifest *Manifest, stale []string) error {
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("create %s: %w", parent, err)
	}
	stage, err := os.MkdirTemp(parent, ".airstrings-pull-")
	if err != nil {
		return fmt.Errorf("create staging dir: %w", err)
	}
	defer os.RemoveAll(stage)

	for _, f := range files {
		if err := os.WriteFile(filepath.Join(stage, f.name), f.data, 0644); err != nil {
			return fmt.Errorf("stage %s: %w", f.name, err)
		}
	}
	if manifest != nil {
		mdata, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal manifest: %w", err)
		}
		mdata = append(mdata, '\n')
		if err := os.WriteFile(filepath.Join(stage, manifestFile), mdata, 0644); err != nil {
			return fmt.Errorf("stage manifest: %w", err)
		}
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	for _, name := range stale {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale %s: %w", name, err)
		}
	}
	for _, f := range files {
		if err := os.Rename(filepath.Join(stage, f.name), filepath.Join(dir, f.name)); err != nil {
			return fmt.Errorf("install %s: %w", f.name, err)
		}
	}
	if manifest != nil {
		if err := os.Rename(filepath.Join(stage, manifestFile), filepath.Join(dir, manifestFile)); err != nil {
			return fmt.Errorf("install manifest: %w", err)
		}
	}
	return nil
}

func fetchVerified(hc *http.Client, meta client.BundleStatus) ([]byte, error) {
	data, err := fetchBundle(hc, meta.CDNURL)
	if err != nil {
		return nil, err
	}
	verr := verifyBundle(data, meta)
	var rm *revisionMismatchError
	if errors.As(verr, &rm) {
		data, err = fetchBundle(hc, cacheBustURL(meta.CDNURL))
		if err != nil {
			return nil, err
		}
		verr = verifyBundle(data, meta)
	}
	if verr != nil {
		return nil, verr
	}
	return data, nil
}

func fetchBundle(hc *http.Client, rawURL string) ([]byte, error) {
	resp, err := hc.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: unexpected status %s", rawURL, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBundleBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read bundle: %w", err)
	}
	if len(data) > maxBundleBytes {
		return nil, fmt.Errorf("bundle exceeds %d bytes", maxBundleBytes)
	}
	return data, nil
}

func cacheBustURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	q.Set("airstrings_cb", strconv.FormatInt(time.Now().UnixNano(), 10))
	u.RawQuery = q.Encode()
	return u.String()
}

type bundleEnvelope struct {
	FormatVersion int                     `json:"format_version"`
	ProjectID     string                  `json:"project_id"`
	Locale        string                  `json:"locale"`
	Revision      int                     `json:"revision"`
	CreatedAt     string                  `json:"created_at"`
	KeyID         string                  `json:"key_id"`
	Signature     string                  `json:"signature"`
	Strings       map[string]bundleString `json:"strings"`
}

type bundleString struct {
	Value  string `json:"value"`
	Format string `json:"format"`
}

type revisionMismatchError struct {
	got  int
	want int
}

func (e *revisionMismatchError) Error() string {
	return fmt.Sprintf("revision mismatch: bundle has %d, metadata says %d", e.got, e.want)
}

func verifyBundle(data []byte, meta client.BundleStatus) error {
	var env bundleEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("parse bundle: %w", err)
	}
	if env.FormatVersion != 1 {
		return fmt.Errorf("unsupported format_version %d", env.FormatVersion)
	}
	if env.Strings == nil {
		return errors.New("bundle has no strings object")
	}
	pub, err := base64.StdEncoding.DecodeString(env.KeyID)
	if err != nil {
		return fmt.Errorf("decode key_id: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("key_id must decode to %d bytes, got %d", ed25519.PublicKeySize, len(pub))
	}
	sig, err := base64.RawURLEncoding.DecodeString(env.Signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("signature must decode to %d bytes, got %d", ed25519.SignatureSize, len(sig))
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), canonicalSignedContent(&env), sig) {
		return errors.New("signature verification failed")
	}
	if env.ProjectID != meta.ProjectID {
		return fmt.Errorf("project_id mismatch: bundle has %q, metadata says %q", env.ProjectID, meta.ProjectID)
	}
	if env.Locale != meta.Locale {
		return fmt.Errorf("locale mismatch: bundle has %q, metadata says %q", env.Locale, meta.Locale)
	}
	if env.Revision != meta.Revision {
		return &revisionMismatchError{got: env.Revision, want: meta.Revision}
	}
	return nil
}

func canonicalSignedContent(env *bundleEnvelope) []byte {
	strs := make(map[string]any, len(env.Strings))
	for k, v := range env.Strings {
		strs[k] = map[string]any{"format": v.Format, "value": v.Value}
	}
	var buf bytes.Buffer
	buf.WriteString(`{"format_version":`)
	buf.WriteString(strconv.Itoa(env.FormatVersion))
	buf.WriteString(`,"project_id":`)
	appendCanonicalString(&buf, env.ProjectID)
	buf.WriteString(`,"locale":`)
	appendCanonicalString(&buf, env.Locale)
	buf.WriteString(`,"revision":`)
	buf.WriteString(strconv.Itoa(env.Revision))
	buf.WriteString(`,"created_at":`)
	appendCanonicalString(&buf, env.CreatedAt)
	buf.WriteString(`,"strings":`)
	appendCanonical(&buf, strs)
	buf.WriteByte('}')
	return buf.Bytes()
}

func canonicalJSON(v any) []byte {
	var buf bytes.Buffer
	appendCanonical(&buf, v)
	return buf.Bytes()
}

func appendCanonical(buf *bytes.Buffer, v any) {
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		appendCanonicalString(buf, t)
	case int:
		buf.WriteString(strconv.Itoa(t))
	case int64:
		buf.WriteString(strconv.FormatInt(t, 10))
	case json.Number:
		buf.WriteString(t.String())
	case float64:
		if t == math.Trunc(t) && math.Abs(t) < 1<<53 {
			buf.WriteString(strconv.FormatInt(int64(t), 10))
		} else {
			buf.WriteString(strconv.FormatFloat(t, 'g', -1, 64))
		}
	case []any:
		buf.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			appendCanonical(buf, e)
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			appendCanonicalString(buf, k)
			buf.WriteByte(':')
			appendCanonical(buf, t[k])
		}
		buf.WriteByte('}')
	default:
		panic(fmt.Sprintf("canonical JSON: unsupported type %T", v))
	}
}

func appendCanonicalString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for i := 0; i < len(s); i++ {
		b := s[i]
		switch {
		case b == '"':
			buf.WriteString(`\"`)
		case b == '\\':
			buf.WriteString(`\\`)
		case b >= 0x20:
			buf.WriteByte(b)
		case b == '\b':
			buf.WriteString(`\b`)
		case b == '\t':
			buf.WriteString(`\t`)
		case b == '\n':
			buf.WriteString(`\n`)
		case b == '\f':
			buf.WriteString(`\f`)
		case b == '\r':
			buf.WriteString(`\r`)
		default:
			fmt.Fprintf(buf, `\u%04x`, b)
		}
	}
	buf.WriteByte('"')
}

func readManifest(path string) (*Manifest, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, true
	}
	return &m, true
}

func deriveOrgID(metas []client.BundleStatus, projectID, envID string) string {
	for _, m := range metas {
		u, err := url.Parse(m.CDNURL)
		if err != nil {
			continue
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) == 5 && parts[4] == "bundle.json" && parts[1] == projectID && parts[2] == envID && parts[3] == m.Locale && parts[0] != "" {
			return parts[0]
		}
	}
	return ""
}

func validLocaleName(locale string) bool {
	if locale == "" || len(locale) > 35 {
		return false
	}
	for i := 0; i < len(locale); i++ {
		b := locale[i]
		if (b < 'a' || b > 'z') && (b < 'A' || b > 'Z') && (b < '0' || b > '9') && b != '-' {
			return false
		}
	}
	return true
}
