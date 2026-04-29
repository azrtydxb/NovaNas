package novanas

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// VMPhase mirrors vms.Phase server-side.
type VMPhase string

// VMDisk, VMNetwork, VMCondition, VM, etc. mirror the server DTO shapes.
// The SDK defines its own copies (rather than importing the internal
// types) so the SDK module stays free of internal/* dependencies.
type VMDisk struct {
	Name         string `json:"name"`
	SizeGB       int    `json:"sizeGB"`
	Source       string `json:"source"`
	Boot         bool   `json:"boot,omitempty"`
	Bus          string `json:"bus,omitempty"`
	StorageClass string `json:"storageClass,omitempty"`
}

type VMNetwork struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type VMCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

type VMCloudInit struct {
	User     string   `json:"user,omitempty"`
	Password string   `json:"password,omitempty"`
	SSHKeys  []string `json:"sshKeys,omitempty"`
	Hostname string   `json:"hostname,omitempty"`
	UserData string   `json:"userData,omitempty"`
}

type VM struct {
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	UID         string            `json:"uid,omitempty"`
	CPU         int               `json:"cpu"`
	MemoryMB    int               `json:"memoryMB"`
	Running     bool              `json:"running"`
	Phase       VMPhase           `json:"phase"`
	IP          string            `json:"ip,omitempty"`
	NodeName    string            `json:"nodeName,omitempty"`
	Disks       []VMDisk          `json:"disks,omitempty"`
	Networks    []VMNetwork       `json:"networks,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   time.Time         `json:"createdAt,omitempty"`
	Conditions  []VMCondition     `json:"conditions,omitempty"`
	TemplateID  string            `json:"templateID,omitempty"`
}

type VMCreateRequest struct {
	Namespace     string            `json:"namespace,omitempty"`
	Name          string            `json:"name"`
	TemplateID    string            `json:"templateID,omitempty"`
	CPU           int               `json:"cpu"`
	MemoryMB      int               `json:"memoryMB"`
	Disks         []VMDisk          `json:"disks,omitempty"`
	Networks      []VMNetwork       `json:"networks,omitempty"`
	CloudInit     VMCloudInit       `json:"cloudInit,omitempty"`
	StartOnCreate bool              `json:"startOnCreate,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
}

type VMPatchRequest struct {
	CPU      *int              `json:"cpu,omitempty"`
	MemoryMB *int              `json:"memoryMB,omitempty"`
	Disks    []VMDisk          `json:"disks,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
}

type VMTemplate struct {
	ID                      string `json:"id"`
	DisplayName             string `json:"displayName"`
	OS                      string `json:"os"`
	Family                  string `json:"family"`
	Version                 string `json:"version"`
	Arch                    string `json:"arch"`
	ImageURL                string `json:"imageURL"`
	ImageFormat             string `json:"imageFormat"`
	DefaultCPU              int    `json:"defaultCPU"`
	DefaultMemoryMB         int    `json:"defaultMemoryMB"`
	DefaultDiskGB           int    `json:"defaultDiskGB"`
	CloudInitFriendly       bool   `json:"cloudInitFriendly"`
	GuestUser               string `json:"guestUser,omitempty"`
	RequiresUserSuppliedISO bool   `json:"requiresUserSuppliedISO,omitempty"`
	RequiresLicenseKey      bool   `json:"requiresLicenseKey,omitempty"`
	Description             string `json:"description,omitempty"`
}

type VMSnapshot struct {
	Namespace  string    `json:"namespace"`
	Name       string    `json:"name"`
	VMName     string    `json:"vmName"`
	Phase      string    `json:"phase,omitempty"`
	ReadyToUse bool      `json:"readyToUse,omitempty"`
	CreatedAt  time.Time `json:"createdAt,omitempty"`
}

type VMRestore struct {
	Namespace    string `json:"namespace"`
	Name         string `json:"name"`
	VMName       string `json:"vmName"`
	SnapshotName string `json:"snapshotName"`
	Complete     bool   `json:"complete"`
}

type VMConsoleSession struct {
	WSURL     string    `json:"wsUrl"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
	Kind      string    `json:"kind"`
}

// VMPage is the paginated list-VMs response.
type VMPage struct {
	Items      []VM   `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type listEnvelope[T any] struct {
	Items []T `json:"items"`
}

// ListVMs returns a paginated list of VMs across all "vm-*" namespaces.
func (c *Client) ListVMs(ctx context.Context, cursor string, pageSize int) (*VMPage, error) {
	q := url.Values{}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if pageSize > 0 {
		q.Set("pageSize", strconv.Itoa(pageSize))
	}
	var out VMPage
	if _, err := c.do(ctx, http.MethodGet, "/vms", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetVM fetches a single VM.
func (c *Client) GetVM(ctx context.Context, namespace, name string) (*VM, error) {
	if namespace == "" || name == "" {
		return nil, errors.New("novanas: namespace and name are required")
	}
	var out VM
	if _, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/vms/%s/%s", namespace, name), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateVM provisions a VM.
func (c *Client) CreateVM(ctx context.Context, req VMCreateRequest) (*VM, error) {
	if req.Name == "" {
		return nil, errors.New("novanas: VMCreateRequest.Name is required")
	}
	var out VM
	if _, err := c.do(ctx, http.MethodPost, "/vms", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PatchVM applies a partial update.
func (c *Client) PatchVM(ctx context.Context, namespace, name string, p VMPatchRequest) (*VM, error) {
	var out VM
	if _, err := c.do(ctx, http.MethodPatch, fmt.Sprintf("/vms/%s/%s", namespace, name), nil, p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteVM removes a VM (and cascades the per-VM namespace).
func (c *Client) DeleteVM(ctx context.Context, namespace, name string) error {
	_, err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/vms/%s/%s", namespace, name), nil, nil, nil)
	return err
}

// StartVM, StopVM, RestartVM, PauseVM, UnpauseVM, MigrateVM are
// idempotent lifecycle triggers. They return the (raw) HTTP status as
// part of the error from c.do — a 501 means "single-node, cannot
// migrate" for MigrateVM.
func (c *Client) StartVM(ctx context.Context, namespace, name string) error {
	_, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/vms/%s/%s/start", namespace, name), nil, nil, nil)
	return err
}
func (c *Client) StopVM(ctx context.Context, namespace, name string) error {
	_, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/vms/%s/%s/stop", namespace, name), nil, nil, nil)
	return err
}
func (c *Client) RestartVM(ctx context.Context, namespace, name string) error {
	_, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/vms/%s/%s/restart", namespace, name), nil, nil, nil)
	return err
}
func (c *Client) PauseVM(ctx context.Context, namespace, name string) error {
	_, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/vms/%s/%s/pause", namespace, name), nil, nil, nil)
	return err
}
func (c *Client) UnpauseVM(ctx context.Context, namespace, name string) error {
	_, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/vms/%s/%s/unpause", namespace, name), nil, nil, nil)
	return err
}
func (c *Client) MigrateVM(ctx context.Context, namespace, name string) error {
	_, err := c.do(ctx, http.MethodPost, fmt.Sprintf("/vms/%s/%s/migrate", namespace, name), nil, nil, nil)
	return err
}

// GetVMConsole mints a console session. kind is "vnc" (default), "spice",
// or "serial".
func (c *Client) GetVMConsole(ctx context.Context, namespace, name, kind string) (*VMConsoleSession, error) {
	q := url.Values{}
	if kind != "" {
		q.Set("kind", kind)
	}
	var out VMConsoleSession
	if _, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/vms/%s/%s/console", namespace, name), q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetVMSerial mints a serial-console session.
func (c *Client) GetVMSerial(ctx context.Context, namespace, name string) (*VMConsoleSession, error) {
	var out VMConsoleSession
	if _, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/vms/%s/%s/serial", namespace, name), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListVMTemplates returns the curated catalog.
func (c *Client) ListVMTemplates(ctx context.Context) ([]VMTemplate, error) {
	var env listEnvelope[VMTemplate]
	if _, err := c.do(ctx, http.MethodGet, "/vm-templates", nil, nil, &env); err != nil {
		return nil, err
	}
	return env.Items, nil
}

// ListVMSnapshots lists snapshots, optionally scoped to a namespace.
func (c *Client) ListVMSnapshots(ctx context.Context, namespace string) ([]VMSnapshot, error) {
	q := url.Values{}
	if namespace != "" {
		q.Set("namespace", namespace)
	}
	var env listEnvelope[VMSnapshot]
	if _, err := c.do(ctx, http.MethodGet, "/vm-snapshots", q, nil, &env); err != nil {
		return nil, err
	}
	return env.Items, nil
}

// CreateVMSnapshot creates a snapshot.
func (c *Client) CreateVMSnapshot(ctx context.Context, namespace, name, vmName string) (*VMSnapshot, error) {
	body := map[string]string{"namespace": namespace, "name": name, "vmName": vmName}
	var out VMSnapshot
	if _, err := c.do(ctx, http.MethodPost, "/vm-snapshots", nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteVMSnapshot removes a snapshot.
func (c *Client) DeleteVMSnapshot(ctx context.Context, namespace, name string) error {
	_, err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/vm-snapshots/%s/%s", namespace, name), nil, nil, nil)
	return err
}

// ListVMRestores lists restores, optionally scoped to a namespace.
func (c *Client) ListVMRestores(ctx context.Context, namespace string) ([]VMRestore, error) {
	q := url.Values{}
	if namespace != "" {
		q.Set("namespace", namespace)
	}
	var env listEnvelope[VMRestore]
	if _, err := c.do(ctx, http.MethodGet, "/vm-restores", q, nil, &env); err != nil {
		return nil, err
	}
	return env.Items, nil
}

// CreateVMRestore creates a restore.
func (c *Client) CreateVMRestore(ctx context.Context, namespace, name, vmName, snapshotName string) (*VMRestore, error) {
	body := map[string]string{
		"namespace": namespace, "name": name,
		"vmName": vmName, "snapshotName": snapshotName,
	}
	var out VMRestore
	if _, err := c.do(ctx, http.MethodPost, "/vm-restores", nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteVMRestore removes a restore.
func (c *Client) DeleteVMRestore(ctx context.Context, namespace, name string) error {
	_, err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/vm-restores/%s/%s", namespace, name), nil, nil, nil)
	return err
}
