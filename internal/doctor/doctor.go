package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/symbionix-sl/airstrings-cli/internal/workspace"
)

const (
	StatusOK      = "ok"
	StatusMissing = "missing"
	StatusManual  = "manual"

	manifestFile = "manifest.json"
	maxDepth     = 4
	pullFix      = "run `airstrings bundles pull`"
)

type Check struct {
	Name   string `json:"name"`
	Path   string `json:"path,omitempty"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Fix    string `json:"fix,omitempty"`
}

type Report struct {
	BundlesDir string  `json:"bundles_dir"`
	Checks     []Check `json:"checks"`
}

func (r *Report) HasMissing() bool {
	for _, c := range r.Checks {
		if c.Status == StatusMissing {
			return true
		}
	}
	return false
}

func ResolveDir(wsDir string, cfg *workspace.WorkspaceConfig, arg string) (string, error) {
	if arg != "" {
		if filepath.IsAbs(arg) {
			return filepath.Clean(arg), nil
		}
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(cwd, arg), nil
	}
	configured := cfg.BundlesDir
	if configured == "" {
		configured = filepath.Join("airstrings", "bundles")
	}
	if filepath.IsAbs(configured) {
		return filepath.Clean(configured), nil
	}
	return filepath.Join(filepath.Dir(wsDir), configured), nil
}

func Run(root, bundlesDir string) *Report {
	rep := &Report{BundlesDir: bundlesDir}
	rep.Checks = append(rep.Checks, checkBundles(bundlesDir))

	scan := scanTree(root, bundlesDir)
	seed := seedDir(bundlesDir)
	seedName := filepath.Base(seed)

	if !scan.any() {
		rep.Checks = append(rep.Checks, Check{
			Name:   "platforms",
			Status: StatusManual,
			Detail: "no supported platform detected at project root",
			Fix:    genericFix(seedName),
		})
		return rep
	}

	for _, p := range scan.pbxprojs {
		rep.Checks = append(rep.Checks, checkXcode(p, seed, seedName, root))
	}
	for _, p := range scan.packageSwifts {
		rep.Checks = append(rep.Checks, checkSPM(p, seedName))
	}
	if scan.bazel {
		rep.Checks = append(rep.Checks, checkBazel(scan.buildFiles, seedName))
	}
	if len(scan.gradleFiles) > 0 {
		rep.Checks = append(rep.Checks, checkAndroid(scan, seed, seedName))
	}
	if scan.web {
		rep.Checks = append(rep.Checks, checkWeb(root, bundlesDir))
	}
	return rep
}

type scanResult struct {
	pbxprojs        []string
	packageSwifts   []string
	bazel           bool
	buildFiles      []string
	gradleFiles     []string
	assetsManifests []string
	web             bool
}

func (s *scanResult) any() bool {
	return len(s.pbxprojs) > 0 || len(s.packageSwifts) > 0 || s.bazel || len(s.gradleFiles) > 0 || s.web
}

var skipDirs = map[string]bool{
	".git":         true,
	".airstrings":  true,
	"node_modules": true,
	"Pods":         true,
	"build":        true,
	".build":       true,
	"DerivedData":  true,
	"dist":         true,
	"vendor":       true,
}

func scanTree(root, bundlesDir string) *scanResult {
	res := &scanResult{}
	var visit func(dir string, depth int)
	visit = func(dir string, depth int) {
		if p := filepath.Join(dir, "src", "main", "assets", "airstrings", "bundles", manifestFile); fileExists(p) {
			res.assetsManifests = append(res.assetsManifests, p)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			name := e.Name()
			path := filepath.Join(dir, name)
			d := depth + 1
			if e.IsDir() {
				if skipDirs[name] || strings.HasPrefix(name, "bazel-") || path == bundlesDir {
					continue
				}
				if strings.HasSuffix(name, ".xcodeproj") {
					if p := filepath.Join(path, "project.pbxproj"); fileExists(p) {
						res.pbxprojs = append(res.pbxprojs, p)
					}
					continue
				}
				if d < maxDepth {
					visit(path, d)
				}
				continue
			}
			switch name {
			case "Package.swift":
				res.packageSwifts = append(res.packageSwifts, path)
			case "MODULE.bazel", "WORKSPACE", "WORKSPACE.bazel":
				if d <= 2 {
					res.bazel = true
				}
			case "BUILD", "BUILD.bazel":
				res.buildFiles = append(res.buildFiles, path)
			case "build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts":
				res.gradleFiles = append(res.gradleFiles, path)
			case "package.json":
				res.web = true
			}
		}
	}
	visit(root, 0)
	return res
}

func seedDir(bundlesDir string) string {
	if filepath.Base(bundlesDir) == "bundles" && filepath.Base(filepath.Dir(bundlesDir)) == "airstrings" {
		return filepath.Dir(bundlesDir)
	}
	return bundlesDir
}

func checkBundles(dir string) Check {
	c := Check{Name: "bundles", Path: dir}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		c.Status = StatusMissing
		c.Detail = "seed directory not found"
		c.Fix = pullFix
		return c
	}
	if !fileExists(filepath.Join(dir, manifestFile)) {
		c.Status = StatusMissing
		c.Detail = "manifest.json not found"
		c.Fix = pullFix
		return c
	}
	entries, _ := os.ReadDir(dir)
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") && e.Name() != manifestFile {
			count++
		}
	}
	if count == 0 {
		c.Status = StatusMissing
		c.Detail = "no bundle files (*.json) besides manifest.json"
		c.Fix = pullFix
		return c
	}
	c.Status = StatusOK
	c.Detail = fmt.Sprintf("%d bundle(s), manifest present", count)
	return c
}

var (
	pathOrNameRe = regexp.MustCompile(`\b(?:path|name) = "?([^";]+)"?;`)
	pathRe       = regexp.MustCompile(`\bpath = "?([^";]+)"?;`)
)

func checkXcode(pbxproj, seed, seedName, root string) Check {
	c := Check{Name: "xcode", Path: pbxproj}
	data, err := os.ReadFile(pbxproj)
	if err != nil {
		c.Status = StatusMissing
		c.Detail = fmt.Sprintf("read project: %s", err)
		c.Fix = xcodeFix(seedName)
		return c
	}
	content := string(data)
	names := append([]string{seedName}, ancestorNames(seed, root)...)
	if hasFolderReference(content, seedName) || hasSyncedRootGroup(content, names) {
		c.Status = StatusOK
		c.Detail = fmt.Sprintf("folder reference to %q found", seedName)
		return c
	}
	c.Status = StatusMissing
	c.Detail = fmt.Sprintf("no folder reference to %q", seedName)
	c.Fix = xcodeFix(seedName)
	return c
}

func hasFolderReference(content, seedName string) bool {
	for _, line := range strings.Split(content, "\n") {
		if !strings.Contains(line, "PBXFileReference") || !strings.Contains(line, "lastKnownFileType = folder;") {
			continue
		}
		for _, m := range pathOrNameRe.FindAllStringSubmatch(line, -1) {
			if filepath.Base(m[1]) == seedName {
				return true
			}
		}
	}
	return false
}

func hasSyncedRootGroup(content string, names []string) bool {
	const marker = "PBXFileSystemSynchronizedRootGroup"
	for {
		i := strings.Index(content, marker)
		if i < 0 {
			return false
		}
		content = content[i+len(marker):]
		chunk := content
		if j := strings.Index(chunk, "isa ="); j >= 0 {
			chunk = chunk[:j]
		}
		for _, m := range pathRe.FindAllStringSubmatch(chunk, -1) {
			base := filepath.Base(m[1])
			for _, n := range names {
				if base == n {
					return true
				}
			}
		}
	}
}

func ancestorNames(seed, root string) []string {
	rel, err := filepath.Rel(root, filepath.Dir(seed))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return nil
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return parts
}

func xcodeFix(seedName string) string {
	return fmt.Sprintf(`in Xcode: File → Add Files to "<App>"… → select the %s/ folder → choose "Create folder references" (blue folder, NOT "Create groups") → ensure your app target is checked. Folder must appear blue in the navigator.`, seedName)
}

func checkSPM(path, seedName string) Check {
	c := Check{Name: "spm", Path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		c.Status = StatusMissing
		c.Detail = fmt.Sprintf("read manifest: %s", err)
		c.Fix = fmt.Sprintf("add resources: [.copy(%q)] to the target", seedName)
		return c
	}
	content := string(data)
	if callArgContains(content, ".copy(", seedName) {
		c.Status = StatusOK
		c.Detail = fmt.Sprintf(".copy resource for %q found", seedName)
		return c
	}
	c.Status = StatusMissing
	if callArgContains(content, ".process(", seedName) {
		c.Detail = fmt.Sprintf(".process flattens the directory and the SDK treats it as absent — change to .copy(%q)", seedName)
		c.Fix = fmt.Sprintf("change .process(%q) to .copy(%q)", seedName, seedName)
		return c
	}
	c.Detail = fmt.Sprintf("no .copy resource for %q", seedName)
	c.Fix = fmt.Sprintf("add resources: [.copy(%q)] to the target", seedName)
	return c
}

func callArgContains(content, call, want string) bool {
	for {
		i := strings.Index(content, call)
		if i < 0 {
			return false
		}
		content = content[i+len(call):]
		end := strings.Index(content, ")")
		if end < 0 {
			return false
		}
		if strings.Contains(content[:end], want) {
			return true
		}
	}
}

func checkBazel(buildFiles []string, seedName string) Check {
	c := Check{Name: "bazel"}
	for _, f := range buildFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), seedName) {
			c.Path = f
			c.Status = StatusManual
			c.Detail = "reference found — verify it preserves directory structure (e.g. apple_resource_group with structured_resources)"
			return c
		}
	}
	c.Status = StatusMissing
	c.Detail = fmt.Sprintf("no BUILD file references %q", seedName)
	c.Fix = fmt.Sprintf(`add a structure-preserving resource rule, e.g.:
apple_resource_group(
    name = "airstrings_seed",
    structured_resources = glob([%q]),
)`, seedName+"/**")
	return c
}

func checkAndroid(scan *scanResult, seed, seedName string) Check {
	c := Check{Name: "android"}
	if len(scan.assetsManifests) > 0 {
		c.Path = scan.assetsManifests[0]
		c.Status = StatusOK
		c.Detail = "seed found in src/main/assets/airstrings/bundles"
		return c
	}
	for _, f := range scan.gradleFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		content := string(data)
		if (strings.Contains(content, "assets.srcDirs") || strings.Contains(content, "assets.setSrcDirs")) && strings.Contains(content, seedName) {
			c.Path = f
			c.Status = StatusOK
			c.Detail = "assets source set maps the seed folder"
			return c
		}
	}
	module := scan.gradleFiles[0]
	for _, f := range scan.gradleFiles {
		if strings.HasPrefix(filepath.Base(f), "build.gradle") {
			module = f
			break
		}
	}
	rel := filepath.Dir(seed)
	if r, err := filepath.Rel(filepath.Dir(module), filepath.Dir(seed)); err == nil {
		rel = r
	}
	c.Path = module
	c.Status = StatusMissing
	c.Detail = "seed not found in assets or gradle source sets"
	c.Fix = fmt.Sprintf(`copy %s/ into src/main/assets/, or map it:
sourceSets { main { assets.srcDirs += [%q] } }`, seedName, filepath.ToSlash(rel))
	return c
}

func checkWeb(root, bundlesDir string) Check {
	c := Check{Name: "web", Path: bundlesDir}
	if bundlesDir == filepath.Join(root, "airstrings", "bundles") {
		c.Status = StatusOK
		c.Detail = "default location — Node seeds automatically from <cwd>/airstrings/bundles"
		return c
	}
	c.Status = StatusManual
	c.Detail = "custom seed location — Node needs an explicit seed dir option; browsers need build-time imports passed via the SDK seed option"
	return c
}

func genericFix(seedName string) string {
	return fmt.Sprintf(`iOS:     add the %[1]s/ folder to your app target as a folder reference (SPM: resources: [.copy(%[1]q)])
Android: copy or map the folder into src/main/assets/
Web:     Node seeds from <cwd>/airstrings/bundles/ automatically; browsers import bundle JSON at build time`, seedName)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
