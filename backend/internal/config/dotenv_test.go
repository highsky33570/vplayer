package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackendDotEnvOverridesRootDotEnv(t *testing.T) {
	const key = "VPLAYER_DOTENV_PROBE"
	root := t.TempDir()
	backendDir := filepath.Join(root, "backend")
	if err := os.MkdirAll(backendDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(key+"=from-root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backendDir, ".env"), []byte(key+"=from-backend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
		_ = os.Unsetenv(key)
	})

	_ = os.Unsetenv(key)
	if err := os.Chdir(backendDir); err != nil {
		t.Fatal(err)
	}
	loadEnvFiles()
	if got := os.Getenv(key); got != "from-backend" {
		t.Fatalf("cwd=backend: got %q want from-backend", got)
	}

	_ = os.Unsetenv(key)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	loadEnvFiles()
	if got := os.Getenv(key); got != "from-backend" {
		t.Fatalf("cwd=root: got %q want from-backend", got)
	}

	if err := os.Setenv(key, "from-process"); err != nil {
		t.Fatal(err)
	}
	loadEnvFiles()
	if got := os.Getenv(key); got != "from-process" {
		t.Fatalf("process env should win: got %q", got)
	}
}
