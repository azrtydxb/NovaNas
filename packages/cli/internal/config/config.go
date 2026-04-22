// Package config handles novanasctl configuration file load/save.
//
// Config lives at ~/.config/novanasctl/config.yaml and holds a list of named
// connection contexts and the currently selected one.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Context is a named connection to a NovaNas API server.
type Context struct {
	Name                  string `yaml:"name"`
	Server                string `yaml:"server"`
	InsecureSkipTLSVerify bool   `yaml:"insecure-skip-tls-verify,omitempty"`
}

// Config is the top-level CLI configuration.
type Config struct {
	CurrentContext string    `yaml:"current-context"`
	Contexts       []Context `yaml:"contexts"`
}

// DefaultPath returns the canonical config file path.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "novanasctl", "config.yaml"), nil
}

// Load reads and parses the config file at path. A missing file yields an
// empty Config and no error.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &c, nil
}

// Save writes the config as YAML, creating parent directories as needed.
func Save(path string, c *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Current returns the currently selected context, or nil if none match.
func (c *Config) Current() *Context {
	for i := range c.Contexts {
		if c.Contexts[i].Name == c.CurrentContext {
			return &c.Contexts[i]
		}
	}
	return nil
}

// Upsert adds or replaces a context by name.
func (c *Config) Upsert(ctx Context) {
	for i := range c.Contexts {
		if c.Contexts[i].Name == ctx.Name {
			c.Contexts[i] = ctx
			return
		}
	}
	c.Contexts = append(c.Contexts, ctx)
}
