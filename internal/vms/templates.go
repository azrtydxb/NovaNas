// Package vms — curated VM templates loader.
package vms

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
)

// Template is a curated cloud-image entry the GUI exposes as a "create
// VM from this image" choice. It mirrors the structure in
// deploy/vms/templates.json.
type Template struct {
	ID                       string `json:"id"`
	DisplayName              string `json:"displayName"`
	OS                       string `json:"os"`
	Family                   string `json:"family"`
	Version                  string `json:"version"`
	Arch                     string `json:"arch"`
	ImageURL                 string `json:"imageURL"`
	ImageFormat              string `json:"imageFormat"`
	DefaultCPU               int    `json:"defaultCPU"`
	DefaultMemoryMB          int    `json:"defaultMemoryMB"`
	DefaultDiskGB            int    `json:"defaultDiskGB"`
	CloudInitFriendly        bool   `json:"cloudInitFriendly"`
	GuestUser                string `json:"guestUser,omitempty"`
	RequiresUserSuppliedISO  bool   `json:"requiresUserSuppliedISO,omitempty"`
	RequiresLicenseKey       bool   `json:"requiresLicenseKey,omitempty"`
	Description              string `json:"description,omitempty"`
}

// templateFile is the on-disk schema.
type templateFile struct {
	Version   int        `json:"version"`
	Templates []Template `json:"templates"`
}

// TemplateCatalog is a thread-safe in-memory snapshot of the curated
// templates JSON.
type TemplateCatalog struct {
	mu        sync.RWMutex
	templates []Template
	byID      map[string]Template
}

// LoadCatalog reads templates from path and returns a populated catalog.
// The default production path is /etc/novanas/vms/templates.json (laid
// down by the deploy/vms/templates.json source-of-truth).
func LoadCatalog(path string) (*TemplateCatalog, error) {
	if path == "" {
		return nil, errors.New("vms: empty template path")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("vms: read templates: %w", err)
	}
	var f templateFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("vms: parse templates: %w", err)
	}
	if len(f.Templates) == 0 {
		return nil, errors.New("vms: templates file contains zero entries")
	}
	cat := &TemplateCatalog{
		templates: append([]Template(nil), f.Templates...),
		byID:      make(map[string]Template, len(f.Templates)),
	}
	for _, t := range f.Templates {
		if t.ID == "" {
			return nil, fmt.Errorf("vms: template missing id (displayName=%q)", t.DisplayName)
		}
		if _, dup := cat.byID[t.ID]; dup {
			return nil, fmt.Errorf("vms: duplicate template id %q", t.ID)
		}
		cat.byID[t.ID] = t
	}
	return cat, nil
}

// NewCatalogFromTemplates builds a catalog from an in-memory slice. Used
// by tests so they don't need a real on-disk file.
func NewCatalogFromTemplates(ts []Template) *TemplateCatalog {
	cat := &TemplateCatalog{
		templates: append([]Template(nil), ts...),
		byID:      make(map[string]Template, len(ts)),
	}
	for _, t := range ts {
		cat.byID[t.ID] = t
	}
	return cat
}

// List returns a copy of all templates.
func (c *TemplateCatalog) List() []Template {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Template, len(c.templates))
	copy(out, c.templates)
	return out
}

// Get returns the template with ID id, or false if no such template.
func (c *TemplateCatalog) Get(id string) (Template, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	t, ok := c.byID[id]
	return t, ok
}

// Count returns the number of templates currently in the catalog.
func (c *TemplateCatalog) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.templates)
}
