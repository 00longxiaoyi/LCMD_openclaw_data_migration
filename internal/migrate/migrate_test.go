package migrate

import (
	"bytes"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

func TestDataSource(t *testing.T) {
	got := DataSource("custom.package")
	want := filepath.Join(legacyBaseDir, "custom.package", "openclaw-data")
	if got != want {
		t.Fatalf("unexpected data source: got %q want %q", got, want)
	}
}

func TestWorkspaceSource(t *testing.T) {
	got := WorkspaceSource("custom.package")
	want := filepath.Join(legacyBaseDir, "custom.package", "openclaw-workspace")
	if got != want {
		t.Fatalf("unexpected workspace source: got %q want %q", got, want)
	}
}

func TestAppSource(t *testing.T) {
	got := AppSource("custom.package")
	want := filepath.Join(legacyBaseDir, "custom.package", "openclaw-app")
	if got != want {
		t.Fatalf("unexpected app source: got %q want %q", got, want)
	}
}

func TestRunCreatesTargetsCopiesAllContentAndCreatesSymlink(t *testing.T) {
	tempDir := t.TempDir()
	restore := overrideMigrationGlobals(tempDir)
	defer restore()

	packageName := "custom.package"
	mustWriteFile(t, filepath.Join(DataSource(packageName), "config.json"), "data-config")
	mustWriteFile(t, filepath.Join(WorkspaceSource(packageName), "project", "main.txt"), "workspace-main")
	mustWriteFile(t, filepath.Join(AppSource(packageName), "dist", "index.html"), "app-index")
	mustWriteFile(t, filepath.Join(dataTargetRoot, "stale.txt"), "old-data")
	mustWriteFile(t, filepath.Join(workspaceTargetRoot, "old.txt"), "old-workspace")
	mustWriteFile(t, filepath.Join(appTargetRoot, "legacy.txt"), "old-app")
	mustWriteFile(t, homeDataSymlinkPath, "old-link-target")

	var ownershipPaths []string
	changeOwnership = func(path string) error {
		ownershipPaths = append(ownershipPaths, path)
		return nil
	}

	var output bytes.Buffer
	if err := Run(packageName, &output); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	assertDirExists(t, homeNodeRoot)
	assertDirExists(t, appTargetRoot)
	assertFileContent(t, filepath.Join(dataTargetRoot, "config.json"), "data-config")
	assertFileContent(t, filepath.Join(workspaceTargetRoot, "project", "main.txt"), "workspace-main")
	assertFileContent(t, filepath.Join(appTargetRoot, "dist", "index.html"), "app-index")
	assertPathAbsent(t, filepath.Join(dataTargetRoot, "stale.txt"))
	assertPathAbsent(t, filepath.Join(workspaceTargetRoot, "old.txt"))
	assertPathAbsent(t, filepath.Join(appTargetRoot, "legacy.txt"))
	assertStringSliceEqual(t, ownershipPaths, []string{dataTargetRoot, workspaceTargetRoot, appTargetRoot})
	assertSymlinkTarget(t, homeDataSymlinkPath, dataTargetRoot)

	text := output.String()
	assertContains(t, text, "开始准备 /home/node")
	assertContains(t, text, "openclaw-data 复制进度 [")
	assertContains(t, text, "openclaw-workspace 复制进度 [")
	assertContains(t, text, "openclaw-app 复制进度 [")
	assertContains(t, text, "已准备 /app")
	assertContains(t, text, "已创建 ~/.openclaw 软链")
}

func TestRunCopiesOnlyDataWhenWorkspaceAndAppMissing(t *testing.T) {
	tempDir := t.TempDir()
	restore := overrideMigrationGlobals(tempDir)
	defer restore()

	packageName := "custom.package"
	mustWriteFile(t, filepath.Join(DataSource(packageName), "config.json"), "data-config")

	var ownershipPaths []string
	changeOwnership = func(path string) error {
		ownershipPaths = append(ownershipPaths, path)
		return nil
	}

	if err := Run(packageName, &bytes.Buffer{}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	assertFileContent(t, filepath.Join(dataTargetRoot, "config.json"), "data-config")
	assertPathAbsent(t, workspaceTargetRoot)
	assertPathAbsent(t, appTargetRoot)
	assertStringSliceEqual(t, ownershipPaths, []string{dataTargetRoot})
	assertSymlinkTarget(t, homeDataSymlinkPath, dataTargetRoot)
}

func TestRunCopiesOnlyWorkspaceWhenDataAndAppMissing(t *testing.T) {
	tempDir := t.TempDir()
	restore := overrideMigrationGlobals(tempDir)
	defer restore()

	packageName := "custom.package"
	mustWriteFile(t, filepath.Join(WorkspaceSource(packageName), "project", "main.txt"), "workspace-main")

	var ownershipPaths []string
	changeOwnership = func(path string) error {
		ownershipPaths = append(ownershipPaths, path)
		return nil
	}

	if err := Run(packageName, &bytes.Buffer{}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	assertFileContent(t, filepath.Join(workspaceTargetRoot, "project", "main.txt"), "workspace-main")
	assertPathAbsent(t, dataTargetRoot)
	assertPathAbsent(t, appTargetRoot)
	assertPathAbsent(t, homeDataSymlinkPath)
	assertStringSliceEqual(t, ownershipPaths, []string{workspaceTargetRoot})
}

func TestRunCopiesOnlyAppWhenDataAndWorkspaceMissing(t *testing.T) {
	tempDir := t.TempDir()
	restore := overrideMigrationGlobals(tempDir)
	defer restore()

	packageName := "custom.package"
	mustWriteFile(t, filepath.Join(AppSource(packageName), "dist", "index.html"), "app-index")

	var ownershipPaths []string
	changeOwnership = func(path string) error {
		ownershipPaths = append(ownershipPaths, path)
		return nil
	}

	if err := Run(packageName, &bytes.Buffer{}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	assertFileContent(t, filepath.Join(appTargetRoot, "dist", "index.html"), "app-index")
	assertPathAbsent(t, dataTargetRoot)
	assertPathAbsent(t, workspaceTargetRoot)
	assertPathAbsent(t, homeDataSymlinkPath)
	assertStringSliceEqual(t, ownershipPaths, []string{appTargetRoot})
}

func TestRunReturnsErrorWhenNoSourceDirectoryExists(t *testing.T) {
	tempDir := t.TempDir()
	restore := overrideMigrationGlobals(tempDir)
	defer restore()

	err := Run("custom.package", &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error when no source directory exists")
	}
	assertContains(t, err.Error(), "未找到可迁移目录")
}

func TestRunStopsOnCopyFailure(t *testing.T) {
	tempDir := t.TempDir()
	restore := overrideMigrationGlobals(tempDir)
	defer restore()

	packageName := "custom.package"
	mustWriteFile(t, filepath.Join(DataSource(packageName), "config.json"), "data-config")

	copyDirectory = func(src, dst, label string, output io.Writer) error {
		return io.ErrUnexpectedEOF
	}

	err := Run(packageName, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error when copy fails")
	}
	assertContains(t, err.Error(), "执行复制 openclaw-data 到 /home/node/.openclaw失败")
}

func TestRunStopsOnSymlinkFailure(t *testing.T) {
	tempDir := t.TempDir()
	restore := overrideMigrationGlobals(tempDir)
	defer restore()

	packageName := "custom.package"
	mustWriteFile(t, filepath.Join(DataSource(packageName), "config.json"), "data-config")
	blockedParent := filepath.Join(tempDir, "blocked")
	mustWriteFile(t, blockedParent, "not-a-directory")
	homeDataSymlinkPath = filepath.Join(blockedParent, ".openclaw")

	err := Run(packageName, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error when symlink creation fails")
	}
	assertContains(t, err.Error(), "执行创建 ~/.openclaw 软链失败")
}

func TestRunInteractiveUsesDefaultPackageName(t *testing.T) {
	tempDir := t.TempDir()
	restore := overrideMigrationGlobals(tempDir)
	defer restore()

	var gotPackageName string
	runMigration = func(packageName string, output io.Writer) error {
		gotPackageName = packageName
		return nil
	}

	input := strings.NewReader("\n\n\n")
	if err := RunInteractive(input, &bytes.Buffer{}); err != nil {
		t.Fatalf("RunInteractive returned error: %v", err)
	}
	if gotPackageName != defaultPackage {
		t.Fatalf("unexpected package name: got %q want %q", gotPackageName, defaultPackage)
	}
}

func TestRunInteractiveUsesCustomPackageName(t *testing.T) {
	tempDir := t.TempDir()
	restore := overrideMigrationGlobals(tempDir)
	defer restore()

	var gotPackageName string
	runMigration = func(packageName string, output io.Writer) error {
		gotPackageName = packageName
		return nil
	}

	input := strings.NewReader("\nn\ncustom.openclaw.pkg\n\n")
	if err := RunInteractive(input, &bytes.Buffer{}); err != nil {
		t.Fatalf("RunInteractive returned error: %v", err)
	}
	if gotPackageName != "custom.openclaw.pkg" {
		t.Fatalf("unexpected package name: got %q want %q", gotPackageName, "custom.openclaw.pkg")
	}
}

func TestRunInteractiveStopsWhenDataIsNotExposed(t *testing.T) {
	input := strings.NewReader("n\n")
	err := RunInteractive(input, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error when data is not exposed")
	}
	assertContains(t, err.Error(), "请先将 OpenClaw 应用数据暴露到网盘后再执行迁移")
}

func TestRunInteractiveStopsWhenOldAppStillInUse(t *testing.T) {
	tempDir := t.TempDir()
	restore := overrideMigrationGlobals(tempDir)
	defer restore()

	runMigration = func(packageName string, output io.Writer) error {
		return nil
	}

	input := strings.NewReader("\n\nn\n")
	err := RunInteractive(input, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error when old app is still in use")
	}
	assertContains(t, err.Error(), "请先停止使用旧版 OpenClaw，再执行迁移")
}

func TestResolveHomeDataSymlinkPathUsesOverride(t *testing.T) {
	tempDir := t.TempDir()
	restore := overrideMigrationGlobals(tempDir)
	defer restore()

	got, err := resolveHomeDataSymlinkPath()
	if err != nil {
		t.Fatalf("resolveHomeDataSymlinkPath returned error: %v", err)
	}
	if got != homeDataSymlinkPath {
		t.Fatalf("unexpected symlink path: got %q want %q", got, homeDataSymlinkPath)
	}
}

func TestResolveHomeDataSymlinkPathUsesSudoUserHome(t *testing.T) {
	oldHomeDataSymlinkPath := homeDataSymlinkPath
	oldLookupUser := lookupUser
	oldUserHomeDir := userHomeDir
	defer func() {
		homeDataSymlinkPath = oldHomeDataSymlinkPath
		lookupUser = oldLookupUser
		userHomeDir = oldUserHomeDir
	}()

	homeDataSymlinkPath = ""
	lookupUser = func(username string) (*user.User, error) {
		if username != "alice" {
			t.Fatalf("unexpected username: %s", username)
		}
		return &user.User{HomeDir: "/home/alice"}, nil
	}
	userHomeDir = func() (string, error) {
		t.Fatal("userHomeDir should not be called when SUDO_USER lookup succeeds")
		return "", nil
	}
	t.Setenv("SUDO_USER", "alice")

	got, err := resolveHomeDataSymlinkPath()
	if err != nil {
		t.Fatalf("resolveHomeDataSymlinkPath returned error: %v", err)
	}
	if got != "/home/alice/.openclaw" {
		t.Fatalf("unexpected symlink path: got %q want %q", got, "/home/alice/.openclaw")
	}
}

func TestResolveHomeDataSymlinkPathFallsBackToCurrentUserHome(t *testing.T) {
	oldHomeDataSymlinkPath := homeDataSymlinkPath
	oldLookupUser := lookupUser
	oldUserHomeDir := userHomeDir
	defer func() {
		homeDataSymlinkPath = oldHomeDataSymlinkPath
		lookupUser = oldLookupUser
		userHomeDir = oldUserHomeDir
	}()

	homeDataSymlinkPath = ""
	lookupUser = func(username string) (*user.User, error) {
		return nil, os.ErrNotExist
	}
	userHomeDir = func() (string, error) {
		return "/home/current", nil
	}
	t.Setenv("SUDO_USER", "alice")

	got, err := resolveHomeDataSymlinkPath()
	if err != nil {
		t.Fatalf("resolveHomeDataSymlinkPath returned error: %v", err)
	}
	if got != "/home/current/.openclaw" {
		t.Fatalf("unexpected symlink path: got %q want %q", got, "/home/current/.openclaw")
	}
}

func overrideMigrationGlobals(tempDir string) func() {
	oldLegacyBaseDir := legacyBaseDir
	oldHomeNodeRoot := homeNodeRoot
	oldDataTargetRoot := dataTargetRoot
	oldWorkspaceTargetRoot := workspaceTargetRoot
	oldAppTargetRoot := appTargetRoot
	oldHomeDataSymlinkPath := homeDataSymlinkPath
	oldRunShellCommand := runShellCommand
	oldLookupUser := lookupUser
	oldUserHomeDir := userHomeDir
	oldRunMigration := runMigration
	oldCopyDirectory := copyDirectory
	oldChangeOwnership := changeOwnership

	legacyBaseDir = tempDir
	homeNodeRoot = filepath.Join(tempDir, "home", "node")
	dataTargetRoot = filepath.Join(homeNodeRoot, ".openclaw")
	workspaceTargetRoot = filepath.Join(homeNodeRoot, "clawd")
	appTargetRoot = filepath.Join(tempDir, "app")
	homeDataSymlinkPath = filepath.Join(tempDir, "user-home", ".openclaw")
	runShellCommand = func(command string) (string, error) {
		return "", nil
	}
	lookupUser = user.Lookup
	userHomeDir = os.UserHomeDir
	runMigration = Run
	copyDirectory = copyDirectoryWithProgress
	changeOwnership = func(path string) error {
		return nil
	}

	return func() {
		legacyBaseDir = oldLegacyBaseDir
		homeNodeRoot = oldHomeNodeRoot
		dataTargetRoot = oldDataTargetRoot
		workspaceTargetRoot = oldWorkspaceTargetRoot
		appTargetRoot = oldAppTargetRoot
		homeDataSymlinkPath = oldHomeDataSymlinkPath
		runShellCommand = oldRunShellCommand
		lookupUser = oldLookupUser
		userHomeDir = oldUserHomeDir
		runMigration = oldRunMigration
		copyDirectory = oldCopyDirectory
		changeOwnership = oldChangeOwnership
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("unexpected file content for %s: got %q want %q", path, string(data), want)
	}
}

func assertDirExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", path)
	}
}

func assertPathAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be absent, got err=%v", path, err)
	}
}

func assertSymlinkTarget(t *testing.T, path, want string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected %s to be a symlink", path)
	}
	target, err := os.Readlink(path)
	if err != nil {
		t.Fatalf("readlink %s: %v", path, err)
	}
	if target != want {
		t.Fatalf("unexpected symlink target for %s: got %q want %q", path, target, want)
	}
}

func assertStringSliceEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("unexpected slice length: got %d want %d, got=%v want=%v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("unexpected slice content at %d: got=%v want=%v", i, got, want)
		}
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected %q to contain %q", got, want)
	}
}
