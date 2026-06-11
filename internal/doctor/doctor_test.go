package doctor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/symbionix-sl/airstrings-cli/internal/workspace"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func seedBundles(t *testing.T, root string) string {
	t.Helper()
	dir := filepath.Join(root, "airstrings", "bundles")
	write(t, filepath.Join(dir, "manifest.json"), `{"manifest_version":1}`)
	write(t, filepath.Join(dir, "en-US.json"), `{}`)
	return dir
}

func findCheck(rep *Report, name string) *Check {
	for i := range rep.Checks {
		if rep.Checks[i].Name == name {
			return &rep.Checks[i]
		}
	}
	return nil
}

func TestDetection(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		check string
	}{
		{"xcode", []string{"App.xcodeproj/project.pbxproj"}, "xcode"},
		{"spm", []string{"Package.swift"}, "spm"},
		{"bazel module", []string{"MODULE.bazel"}, "bazel"},
		{"bazel workspace", []string{"WORKSPACE"}, "bazel"},
		{"bazel workspace.bazel depth 2", []string{"sub/WORKSPACE.bazel"}, "bazel"},
		{"android build.gradle", []string{"app/build.gradle"}, "android"},
		{"android settings.gradle.kts", []string{"settings.gradle.kts"}, "android"},
		{"web", []string{"package.json"}, "web"},
		{"web at depth 4", []string{"a/b/c/package.json"}, "web"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := seedBundles(t, root)
			for _, f := range tc.files {
				write(t, filepath.Join(root, filepath.FromSlash(f)), "")
			}
			rep := Run(root, dir)
			if findCheck(rep, tc.check) == nil {
				t.Fatalf("expected %q check, got %+v", tc.check, rep.Checks)
			}
		})
	}
}

func TestDetectionSkipsAndDepthBound(t *testing.T) {
	cases := []struct {
		name   string
		files  []string
		absent string
	}{
		{"node_modules skipped", []string{"node_modules/pkg/package.json"}, "web"},
		{"Pods skipped", []string{"Pods/Lib/Package.swift"}, "spm"},
		{"bazel- prefix skipped", []string{"bazel-myproj/package.json"}, "web"},
		{"bundles dir skipped", []string{"airstrings/bundles/package.json"}, "web"},
		{"depth 5 not detected", []string{"a/b/c/d/package.json"}, "web"},
		{"bazel marker beyond depth 2", []string{"a/b/MODULE.bazel"}, "bazel"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := seedBundles(t, root)
			for _, f := range tc.files {
				write(t, filepath.Join(root, filepath.FromSlash(f)), "")
			}
			rep := Run(root, dir)
			if c := findCheck(rep, tc.absent); c != nil {
				t.Fatalf("expected no %q check, got %+v", tc.absent, c)
			}
		})
	}
}

func TestNoPlatformsDetected(t *testing.T) {
	root := t.TempDir()
	dir := seedBundles(t, root)
	rep := Run(root, dir)
	c := findCheck(rep, "platforms")
	if c == nil {
		t.Fatalf("expected platforms check, got %+v", rep.Checks)
	}
	if c.Status != StatusManual {
		t.Fatalf("status = %q, want %q", c.Status, StatusManual)
	}
	for _, hint := range []string{"folder reference", "src/main/assets", "build time"} {
		if !strings.Contains(c.Fix, hint) {
			t.Errorf("fix missing %q: %s", hint, c.Fix)
		}
	}
}

const pbxFolderRef = `// !$*UTF8*$!
{
	objects = {
		AB1 /* airstrings */ = {isa = PBXFileReference; lastKnownFileType = folder; path = airstrings; sourceTree = "<group>"; };
	};
}
`

const pbxSyncGroup = `// !$*UTF8*$!
{
	objects = {
/* Begin PBXFileSystemSynchronizedRootGroup section */
		AB2 /* airstrings */ = {
			isa = PBXFileSystemSynchronizedRootGroup;
			path = airstrings;
			sourceTree = "<group>";
		};
/* End PBXFileSystemSynchronizedRootGroup section */
	};
}
`

const pbxPlainGroup = `// !$*UTF8*$!
{
	objects = {
		AB3 /* airstrings */ = {isa = PBXGroup; path = airstrings; sourceTree = "<group>"; };
	};
}
`

func TestXcode(t *testing.T) {
	cases := []struct {
		name    string
		content string
		status  string
	}{
		{"folder reference", pbxFolderRef, StatusOK},
		{"synchronized root group", pbxSyncGroup, StatusOK},
		{"plain group", pbxPlainGroup, StatusMissing},
		{"empty project", "// !$*UTF8*$!\n", StatusMissing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := seedBundles(t, root)
			write(t, filepath.Join(root, "App.xcodeproj", "project.pbxproj"), tc.content)
			rep := Run(root, dir)
			c := findCheck(rep, "xcode")
			if c == nil {
				t.Fatalf("expected xcode check, got %+v", rep.Checks)
			}
			if c.Status != tc.status {
				t.Fatalf("status = %q, want %q (detail: %s)", c.Status, tc.status, c.Detail)
			}
			if tc.status == StatusMissing && !strings.Contains(c.Fix, "Create folder references") {
				t.Errorf("fix missing folder-reference steps: %s", c.Fix)
			}
		})
	}
}

func TestXcodeSyncGroupMatchesAncestor(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "App", "airstrings", "bundles")
	write(t, filepath.Join(dir, "manifest.json"), `{}`)
	write(t, filepath.Join(dir, "en-US.json"), `{}`)
	content := strings.ReplaceAll(pbxSyncGroup, "path = airstrings;", "path = App;")
	write(t, filepath.Join(root, "App.xcodeproj", "project.pbxproj"), content)
	rep := Run(root, dir)
	c := findCheck(rep, "xcode")
	if c == nil || c.Status != StatusOK {
		t.Fatalf("expected ok xcode check, got %+v", c)
	}
}

func TestSPM(t *testing.T) {
	cases := []struct {
		name     string
		content  string
		status   string
		inDetail string
		inFix    string
	}{
		{
			"copy resource",
			`let package = Package(targets: [.target(name: "App", resources: [.copy("airstrings")])])`,
			StatusOK, "", "",
		},
		{
			"process resource",
			`let package = Package(targets: [.target(name: "App", resources: [.process("airstrings")])])`,
			StatusMissing, "flattens", `.copy("airstrings")`,
		},
		{
			"absent",
			`let package = Package(targets: [.target(name: "App")])`,
			StatusMissing, "no .copy resource", `resources: [.copy("airstrings")]`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := seedBundles(t, root)
			write(t, filepath.Join(root, "Package.swift"), tc.content)
			rep := Run(root, dir)
			c := findCheck(rep, "spm")
			if c == nil {
				t.Fatalf("expected spm check, got %+v", rep.Checks)
			}
			if c.Status != tc.status {
				t.Fatalf("status = %q, want %q (detail: %s)", c.Status, tc.status, c.Detail)
			}
			if tc.inDetail != "" && !strings.Contains(c.Detail, tc.inDetail) {
				t.Errorf("detail missing %q: %s", tc.inDetail, c.Detail)
			}
			if tc.inFix != "" && !strings.Contains(c.Fix, tc.inFix) {
				t.Errorf("fix missing %q: %s", tc.inFix, c.Fix)
			}
		})
	}
}

func TestBazel(t *testing.T) {
	t.Run("reference found", func(t *testing.T) {
		root := t.TempDir()
		dir := seedBundles(t, root)
		write(t, filepath.Join(root, "MODULE.bazel"), "")
		write(t, filepath.Join(root, "app", "BUILD.bazel"), `apple_resource_group(name = "seed", structured_resources = glob(["airstrings/**"]))`)
		rep := Run(root, dir)
		c := findCheck(rep, "bazel")
		if c == nil || c.Status != StatusManual {
			t.Fatalf("expected manual bazel check, got %+v", c)
		}
		if !strings.Contains(c.Detail, "structured_resources") {
			t.Errorf("detail missing verification hint: %s", c.Detail)
		}
	})
	t.Run("absent", func(t *testing.T) {
		root := t.TempDir()
		dir := seedBundles(t, root)
		write(t, filepath.Join(root, "MODULE.bazel"), "")
		write(t, filepath.Join(root, "app", "BUILD"), `swift_library(name = "app")`)
		rep := Run(root, dir)
		c := findCheck(rep, "bazel")
		if c == nil || c.Status != StatusMissing {
			t.Fatalf("expected missing bazel check, got %+v", c)
		}
		if !strings.Contains(c.Fix, "structured_resources") {
			t.Errorf("fix missing snippet: %s", c.Fix)
		}
	})
}

func TestAndroid(t *testing.T) {
	t.Run("copied assets layout", func(t *testing.T) {
		root := t.TempDir()
		dir := seedBundles(t, root)
		write(t, filepath.Join(root, "app", "build.gradle"), "")
		write(t, filepath.Join(root, "app", "src", "main", "assets", "airstrings", "bundles", "manifest.json"), `{}`)
		rep := Run(root, dir)
		c := findCheck(rep, "android")
		if c == nil || c.Status != StatusOK {
			t.Fatalf("expected ok android check, got %+v", c)
		}
	})
	t.Run("gradle srcDirs mapping", func(t *testing.T) {
		root := t.TempDir()
		dir := seedBundles(t, root)
		write(t, filepath.Join(root, "app", "build.gradle.kts"),
			`android { sourceSets { getByName("main") { assets.srcDirs += listOf("../airstrings") } } }`)
		rep := Run(root, dir)
		c := findCheck(rep, "android")
		if c == nil || c.Status != StatusOK {
			t.Fatalf("expected ok android check, got %+v", c)
		}
	})
	t.Run("absent", func(t *testing.T) {
		root := t.TempDir()
		dir := seedBundles(t, root)
		write(t, filepath.Join(root, "app", "build.gradle"), "android {}")
		rep := Run(root, dir)
		c := findCheck(rep, "android")
		if c == nil || c.Status != StatusMissing {
			t.Fatalf("expected missing android check, got %+v", c)
		}
		if !strings.Contains(c.Fix, "src/main/assets") {
			t.Errorf("fix missing copy option: %s", c.Fix)
		}
		if !strings.Contains(c.Fix, `assets.srcDirs += [".."]`) {
			t.Errorf("fix missing srcDirs mapping with real relative path: %s", c.Fix)
		}
	})
}

func TestWeb(t *testing.T) {
	t.Run("default location", func(t *testing.T) {
		root := t.TempDir()
		dir := seedBundles(t, root)
		write(t, filepath.Join(root, "package.json"), `{}`)
		rep := Run(root, dir)
		c := findCheck(rep, "web")
		if c == nil || c.Status != StatusOK {
			t.Fatalf("expected ok web check, got %+v", c)
		}
	})
	t.Run("custom dir", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, "seed", "bundles")
		write(t, filepath.Join(dir, "manifest.json"), `{}`)
		write(t, filepath.Join(dir, "en-US.json"), `{}`)
		write(t, filepath.Join(root, "package.json"), `{}`)
		rep := Run(root, dir)
		c := findCheck(rep, "web")
		if c == nil || c.Status != StatusManual {
			t.Fatalf("expected manual web check, got %+v", c)
		}
	})
}

func TestBundlesSanity(t *testing.T) {
	t.Run("missing dir", func(t *testing.T) {
		root := t.TempDir()
		rep := Run(root, filepath.Join(root, "airstrings", "bundles"))
		c := findCheck(rep, "bundles")
		if c == nil || c.Status != StatusMissing {
			t.Fatalf("expected missing bundles check, got %+v", c)
		}
		if !strings.Contains(c.Fix, "bundles pull") {
			t.Errorf("fix missing pull hint: %s", c.Fix)
		}
	})
	t.Run("missing manifest", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, "airstrings", "bundles")
		write(t, filepath.Join(dir, "en-US.json"), `{}`)
		rep := Run(root, dir)
		c := findCheck(rep, "bundles")
		if c == nil || c.Status != StatusMissing {
			t.Fatalf("expected missing bundles check, got %+v", c)
		}
	})
	t.Run("no bundle files", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, "airstrings", "bundles")
		write(t, filepath.Join(dir, "manifest.json"), `{}`)
		rep := Run(root, dir)
		c := findCheck(rep, "bundles")
		if c == nil || c.Status != StatusMissing {
			t.Fatalf("expected missing bundles check, got %+v", c)
		}
	})
	t.Run("ok", func(t *testing.T) {
		root := t.TempDir()
		dir := seedBundles(t, root)
		rep := Run(root, dir)
		c := findCheck(rep, "bundles")
		if c == nil || c.Status != StatusOK {
			t.Fatalf("expected ok bundles check, got %+v", c)
		}
	})
}

func TestResolveDir(t *testing.T) {
	root := t.TempDir()
	wsDir := filepath.Join(root, ".airstrings")

	t.Run("default", func(t *testing.T) {
		dir, err := ResolveDir(wsDir, &workspace.WorkspaceConfig{}, "")
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(root, "airstrings", "bundles"); dir != want {
			t.Fatalf("dir = %q, want %q", dir, want)
		}
	})
	t.Run("configured relative", func(t *testing.T) {
		dir, err := ResolveDir(wsDir, &workspace.WorkspaceConfig{BundlesDir: "custom/seed"}, "")
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(root, "custom", "seed"); dir != want {
			t.Fatalf("dir = %q, want %q", dir, want)
		}
	})
	t.Run("arg absolute", func(t *testing.T) {
		abs := filepath.Join(root, "elsewhere")
		dir, err := ResolveDir(wsDir, &workspace.WorkspaceConfig{}, abs+string(os.PathSeparator))
		if err != nil {
			t.Fatal(err)
		}
		if dir != abs {
			t.Fatalf("dir = %q, want %q", dir, abs)
		}
	})
	t.Run("arg relative to cwd", func(t *testing.T) {
		t.Chdir(root)
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		dir, err := ResolveDir(wsDir, &workspace.WorkspaceConfig{}, "rel/dir")
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(cwd, "rel", "dir"); dir != want {
			t.Fatalf("dir = %q, want %q", dir, want)
		}
	})
}

func TestDoctorDoesNotMutateConfig(t *testing.T) {
	root := t.TempDir()
	if err := workspace.Init(root, workspace.WorkspaceConfig{ProjectID: "proj_x", ProjectName: "X"}); err != nil {
		t.Fatal(err)
	}
	wsDir := filepath.Join(root, ".airstrings")
	cfgPath := filepath.Join(wsDir, "config.json")
	before, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := workspace.LoadConfig(wsDir)
	if err != nil {
		t.Fatal(err)
	}

	dir, err := ResolveDir(wsDir, cfg, filepath.Join(root, "custom"))
	if err != nil {
		t.Fatal(err)
	}
	Run(root, dir)

	dir, err = ResolveDir(wsDir, cfg, "")
	if err != nil {
		t.Fatal(err)
	}
	Run(root, dir)

	if cfg.BundlesDir != "" {
		t.Fatalf("BundlesDir mutated in memory: %q", cfg.BundlesDir)
	}
	after, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("config.json changed:\nbefore: %s\nafter:  %s", before, after)
	}
}

func TestHasMissing(t *testing.T) {
	cases := []struct {
		name string
		rep  Report
		want bool
	}{
		{"missing present", Report{Checks: []Check{{Status: StatusOK}, {Status: StatusMissing}}}, true},
		{"ok and manual only", Report{Checks: []Check{{Status: StatusOK}, {Status: StatusManual}}}, false},
		{"empty", Report{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.rep.HasMissing(); got != tc.want {
				t.Fatalf("HasMissing() = %v, want %v", got, tc.want)
			}
		})
	}
}
