// Package config reads ~/.config/voip-info/config.yaml (what a person edits)
// and locates the data directory (what the tool writes: cache, state).
package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/napadailo/voip-info/internal/secret"
)

// Config is the user configuration.
type Config struct {
	Theme    string                       `yaml:"theme,omitempty"`
	Language string                       `yaml:"language,omitempty"`
	Themes   map[string]map[string]string `yaml:"themes,omitempty"`
	DataDir  string                       `yaml:"dataDir,omitempty"`
	Digest   struct {
		Days int  `yaml:"days,omitempty"`
		Mine bool `yaml:"mine,omitempty"`
	} `yaml:"digest,omitempty"`
	// Secrets: where each token is: a variable, a file, a command
	Secrets secret.Set `yaml:"secrets,omitempty"`

	path string
}

// Sample is printed by -sample-config.
const Sample = `# voip-info config: ~/.config/voip-info/config.yaml
# theme: nord              # :theme lists them, custom ones below
# language: ru             # en, ru; default: from LANG; :lang switches
# dataDir: ~/.local/share/voip-info   # cache and state
# digest:
#   days: 7                # default period of :digest and voip-info digest
#   mine: true             # only "my stack" by default
# secrets:                 # where the tokens are, tried in order: env, file, command;
#                          # a secret without settings is not read
#   github: {command: gh auth token}                  # the token of the gh CLI
#   nvd:                                               # an NVD API key
#     env: NVD_API_KEY
#     file: ~/.config/voip-info/nvd.key                # a file, Docker or Kubernetes secrets
#     command: security find-generic-password -s voipinfo-nvd -w   # the macOS keychain, any secret manager
#   # voip-info secrets tells where each was found
# themes:
#   noc:
#     base: nord
#     accent: "#ffaf00"
`

// DefaultPath is $VOIP_INFO_CONFIG, $XDG_CONFIG_HOME/voip-info/config.yaml or ~/.config/voip-info/config.yaml.
func DefaultPath() string {
	if p := os.Getenv("VOIP_INFO_CONFIG"); p != "" {
		return p
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "voip-info", "config.yaml")
}

// Load reads the config; a missing file is an empty config.
func Load(path string) (*Config, error) {
	c := &Config{path: path}
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		if err := yaml.Unmarshal(b, c); err != nil {
			return nil, err
		}
	}
	if c.Digest.Days == 0 {
		c.Digest.Days = 7
	}
	return c, nil
}

// Path is where the config was read from.
func (c *Config) Path() string { return c.path }

// Data returns the data directory, created on demand: $VOIP_INFO_DATA,
// dataDir from the config, $XDG_DATA_HOME/voip-info or ~/.local/share/voip-info.
func (c *Config) Data() (string, error) {
	d := os.Getenv("VOIP_INFO_DATA")
	if d == "" {
		d = c.DataDir
	}
	if d == "" {
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			home, _ := os.UserHomeDir()
			base = filepath.Join(home, ".local", "share")
		}
		d = filepath.Join(base, "voip-info")
	}
	d = ExpandHome(d)
	return d, os.MkdirAll(d, 0o755)
}

// ExpandHome replaces a leading "~/" with the home directory.
func ExpandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}

// Apply makes the secrets of the config the ones the program reads.
func (c *Config) Apply() { secret.Configure(c.Secrets) }
