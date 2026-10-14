package tui

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/casablanque-code/komposer/pkg/composer"
)

func TestEnsureGitignoreEntry_CreatesFileIfMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".gitignore")
	if err := ensureGitignoreEntry(path, ".env"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	if string(got) != ".env\n" {
		t.Fatalf("content = %q, want %q", got, ".env\n")
	}
}

func TestEnsureGitignoreEntry_AppendsWithoutClobberingExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".gitignore")
	if err := os.WriteFile(path, []byte("node_modules/\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := ensureGitignoreEntry(path, ".env"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	want := "node_modules/\n.env\n"
	if string(got) != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

func TestEnsureGitignoreEntry_NoDuplicateOnExactMatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".gitignore")
	if err := os.WriteFile(path, []byte(".env\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := ensureGitignoreEntry(path, ".env"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	if string(got) != ".env\n" {
		t.Fatalf("content = %q, want no duplicate line", got)
	}
}

func TestEnsureGitignoreEntry_TrailingSlashTreatedAsSameEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".gitignore")
	// Already ignored without a trailing slash — asking to add it
	// with one (or vice versa) shouldn't add a second, redundant line.
	if err := os.WriteFile(path, []byte("secrets\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := ensureGitignoreEntry(path, "secrets/"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	if string(got) != "secrets\n" {
		t.Fatalf("content = %q, want no duplicate line for secrets vs secrets/", got)
	}
}

func TestAppendEnvFileLine_CreatesFileIfMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := appendEnvFileLine(path, "POSTGRES_PASSWORD=hunter2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	if string(got) != "POSTGRES_PASSWORD=hunter2\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestAppendEnvFileLine_NoDuplicateForSameKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("POSTGRES_PASSWORD=already-here\n"), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := appendEnvFileLine(path, "POSTGRES_PASSWORD=hunter2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	// The existing value is left alone — appendEnvFileLine only adds a
	// line for a key that isn't there yet, it never overwrites one.
	if string(got) != "POSTGRES_PASSWORD=already-here\n" {
		t.Fatalf("content = %q, want the existing value preserved unchanged", got)
	}
}

func TestWriteSecretFile_CreatesParentDirAndWritesValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets", "db_password.txt")
	if err := writeSecretFile(path, "hunter2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	if string(got) != "hunter2" {
		t.Fatalf("content = %q, want %q", got, "hunter2")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// Windows/NTFS doesn't have POSIX permission bits — os.Stat there
	// reports a fixed, coarse mode (read-write or read-only) regardless
	// of what was passed to WriteFile, never a specific 0600, so this
	// assertion only means anything on platforms that actually have
	// that permission model.
	if runtime.GOOS != "windows" {
		if info.Mode().Perm() != 0600 {
			t.Errorf("mode = %v, want 0600 (secret files are more restrictive than the compose file itself)", info.Mode().Perm())
		}
	}
}

// newSecretTestModel returns a model bound to dir with one service
// holding a hardcoded POSTGRES_PASSWORD.
func newSecretTestModel(t *testing.T, dir string) *Model {
	t.Helper()
	cfg := composer.NewComposeConfig()
	svc := cfg.AddService("db")
	svc.Image = "postgres:16"
	svc.Environment = []string{"POSTGRES_PASSWORD=hunter2"}
	return &Model{config: cfg, composeDir: dir}
}

func TestConvertSecretCmd_EnvFileGoesNextToComposeFile(t *testing.T) {
	dir := t.TempDir()
	cwd := t.TempDir()
	t.Chdir(cwd) // the shell's working directory must stay untouched

	m := newSecretTestModel(t, dir)
	res := m.convertSecretCmd("db", "POSTGRES_PASSWORD", 0)().(secretActionResult)
	if res.err != nil {
		t.Fatalf("unexpected error: %v", res.err)
	}

	env, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatalf(".env not written next to compose file: %v", err)
	}
	if string(env) != "POSTGRES_PASSWORD=hunter2\n" {
		t.Fatalf(".env = %q", env)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); err != nil {
		t.Fatalf(".gitignore not written next to compose file: %v", err)
	}
	for _, name := range []string{".env", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(cwd, name)); err == nil {
			t.Fatalf("%s leaked into the working directory", name)
		}
	}
}

func TestConvertSecretCmd_ComposeSecretGoesNextToComposeFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits are not meaningful on Windows")
	}
	dir := t.TempDir()
	cwd := t.TempDir()
	t.Chdir(cwd)

	m := newSecretTestModel(t, dir)
	res := m.convertSecretCmd("db", "POSTGRES_PASSWORD", 1)().(secretActionResult)
	if res.err != nil {
		t.Fatalf("unexpected error: %v", res.err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "secrets", "db_postgres_password.txt"))
	if err != nil {
		t.Fatalf("secret file not written next to compose file: %v", err)
	}
	if string(got) != "hunter2" {
		t.Fatalf("secret content = %q", got)
	}
	if _, err := os.Stat(filepath.Join(cwd, "secrets")); err == nil {
		t.Fatal("secrets/ leaked into the working directory")
	}
}

func TestConvertSecretCmd_UnboundModelFallsBackToWorkingDir(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	m := newSecretTestModel(t, "")
	res := m.convertSecretCmd("db", "POSTGRES_PASSWORD", 0)().(secretActionResult)
	if res.err != nil {
		t.Fatalf("unexpected error: %v", res.err)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".env")); err != nil {
		t.Fatalf(".env not written to working directory: %v", err)
	}
}

func TestImportFile_BindsComposeDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(path, []byte("services:\n  web:\n    image: nginx:1.27\n"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	m := &Model{config: composer.NewComposeConfig()}
	if err := m.importFile(path); err != nil {
		t.Fatalf("import: %v", err)
	}
	if m.composeDir != dir {
		t.Fatalf("composeDir = %q, want %q", m.composeDir, dir)
	}
}

func TestSaveResult_BindsComposeDir(t *testing.T) {
	dir := t.TempDir()
	m := Model{config: composer.NewComposeConfig(), dirty: true}
	next, _ := m.Update(saveResult{filename: filepath.Join(dir, "compose.yaml")})
	if got := next.(Model).composeDir; got != dir {
		t.Fatalf("composeDir = %q, want %q", got, dir)
	}
}
