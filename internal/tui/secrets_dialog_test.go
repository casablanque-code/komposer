package tui

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
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
