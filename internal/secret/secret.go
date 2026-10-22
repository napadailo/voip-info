// Package secret reads the tokens where the config says (secrets:): an
// environment variable, a file, the output of a command. The code knows no
// names of its own beyond the usual variables the config sets by default.
package secret

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Secret is where a secret is, tried in this order; any may be empty.
type Secret struct {
	// Env is an environment variable.
	Env string `yaml:"env,omitempty"`
	// File holds the secret: Docker and Kubernetes secrets, systemd
	// credentials, a file of mode 0600; "~/" is the home directory.
	File string `yaml:"file,omitempty"`
	// Command prints the secret, run by sh: pass show voip-info/github,
	// op read …, secret-tool lookup …, security find-generic-password -s … -w.
	Command string `yaml:"command,omitempty"`
}

// Set is the secrets voip-info reads.
type Set struct {
	GitHub Secret `yaml:"github,omitempty"`
	NVD    Secret `yaml:"nvd,omitempty"`
}

// Names of the secrets, as the config and voip-info secrets call them.
var Names = []string{"github", "nvd"}

// Of returns the secret named.
func (s Set) Of(name string) Secret {
	switch name {
	case "github":
		return s.GitHub
	case "nvd":
		return s.NVD
	}
	return Secret{}
}

var (
	mu      sync.Mutex
	current Set
	cache   = map[string]Found{} // name -> what was found, read once
)

// Configure sets where the secrets are; the config does it at the start.
func Configure(s Set) {
	mu.Lock()
	defer mu.Unlock()
	current, cache = s, map[string]Found{}
}

// GitHubToken and NVDKey are the secrets, "" when unset.
func GitHubToken() string { return Lookup("github").Value }
func NVDKey() string      { return Lookup("nvd").Value }

// Found is a secret and where it came from: "env", "file", "command", "" when none.
type Found struct {
	Value, From string
	Err         error // why the file or the command gave nothing
}

// Lookup finds a secret: the variable, the file, the command; the first
// that is not empty. Files and commands are read once.
func Lookup(name string) Found {
	mu.Lock()
	s := current.Of(name)
	f, ok := cache[name]
	mu.Unlock()
	if s.Env != "" {
		if v := strings.TrimSpace(os.Getenv(s.Env)); v != "" {
			return Found{Value: v, From: "env"}
		}
	}
	if ok {
		return f
	}
	f = read(s)
	mu.Lock()
	cache[name] = f
	mu.Unlock()
	return f
}

func read(s Secret) Found {
	var errs []error
	if s.File != "" {
		p := s.File
		if strings.HasPrefix(p, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				p = filepath.Join(home, p[2:])
			}
		}
		b, err := os.ReadFile(p)
		if v := strings.TrimSpace(string(b)); err == nil && v != "" {
			return Found{Value: v, From: "file"}
		}
		errs = append(errs, err)
	}
	if s.Command != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "sh", "-c", s.Command).Output()
		if v := strings.TrimSpace(string(out)); err == nil && v != "" {
			return Found{Value: v, From: "command"}
		}
		errs = append(errs, err)
	}
	return Found{Err: errors.Join(errs...)}
}
