package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/napadailo/voip-info/internal/secret"
)

func TestSecrets(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	key := filepath.Join(t.TempDir(), "nvd.key")
	_ = os.WriteFile(key, []byte("nvd-key\n"), 0o600)
	_ = os.WriteFile(p, []byte("secrets:\n  github: {env: MY_TOKEN, command: echo from-command}\n  nvd: {file: "+key+"}\n"), 0o644)
	t.Setenv("NVD_API_KEY", "not read: secrets.nvd names a file, no env")
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Secrets.GitHub != (secret.Secret{Env: "MY_TOKEN", Command: "echo from-command"}) || c.Secrets.NVD.Env != "" {
		t.Errorf("%+v", c.Secrets)
	}
	t.Setenv("MY_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "not read: no default names")
	c.Apply()
	if f := secret.Lookup("github"); f.Value != "from-command" || f.From != "command" {
		t.Errorf("github %+v", f)
	}
	if f := secret.Lookup("nvd"); f.Value != "nvd-key" || f.From != "file" {
		t.Errorf("nvd %+v", f)
	}
	t.Setenv("MY_TOKEN", "abc") // the variable comes first
	if secret.GitHubToken() != "abc" {
		t.Errorf("token %q", secret.GitHubToken())
	}
	// a failing command gives nothing and says why
	_ = os.WriteFile(p, []byte("secrets:\n  nvd: {command: exit 1}\n"), 0o644)
	c, _ = Load(p)
	c.Apply()
	if f := secret.Lookup("nvd"); f.Value != "" || f.Err == nil {
		t.Errorf("a failing command: %+v", f)
	}
	// no settings, no secret
	c, _ = Load(filepath.Join(t.TempDir(), "none.yaml"))
	c.Apply()
	if secret.GitHubToken() != "" {
		t.Error("a secret without settings")
	}
}
