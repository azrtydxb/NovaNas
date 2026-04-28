package dataset

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/novanas/nova-nas/internal/host/exec"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
)

// SendOpts controls flags passed to `zfs send`.
type SendOpts struct {
	Recursive       bool   // -R
	IncrementalFrom string // -i <snap>; pass empty to skip
	Raw             bool   // -w (raw, for encrypted send without unlocking)
	Compressed      bool   // -c
	LargeBlock      bool   // -L
	EmbeddedData    bool   // -e
}

// RecvOpts controls flags passed to `zfs receive`.
type RecvOpts struct {
	Force          bool   // -F
	Resumable      bool   // -s
	OriginSnapshot string // -o origin=<snap> (for receiving a clone-stream)
}

// --- Rename ----------------------------------------------------------------

func buildRenameArgs(oldName, newName string, recursive bool) ([]string, error) {
	if strings.HasPrefix(oldName, "-") || strings.HasPrefix(newName, "-") {
		return nil, fmt.Errorf("name cannot start with '-'")
	}
	// `zfs rename` accepts dataset and snapshot names. Both sides must
	// be the same kind. -r is for snapshot rename across descendants.
	oldSnap := strings.Contains(oldName, "@")
	newSnap := strings.Contains(newName, "@")
	if oldSnap != newSnap {
		return nil, fmt.Errorf("rename source and target must be same kind (dataset or snapshot)")
	}
	if oldSnap {
		if err := names.ValidateSnapshotName(oldName); err != nil {
			return nil, err
		}
		if err := names.ValidateSnapshotName(newName); err != nil {
			return nil, err
		}
	} else {
		if err := names.ValidateDatasetName(oldName); err != nil {
			return nil, err
		}
		if err := names.ValidateDatasetName(newName); err != nil {
			return nil, err
		}
	}
	args := []string{"rename"}
	if recursive {
		args = append(args, "-r")
	}
	args = append(args, oldName, newName)
	return args, nil
}

func (m *Manager) Rename(ctx context.Context, oldName, newName string, recursive bool) error {
	args, err := buildRenameArgs(oldName, newName, recursive)
	if err != nil {
		return err
	}
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	_, err = runner(ctx, m.ZFSBin, args...)
	return err
}

// --- Clone -----------------------------------------------------------------

func buildCloneArgs(snapshot, target string, properties map[string]string) ([]string, error) {
	if err := names.ValidateSnapshotName(snapshot); err != nil {
		return nil, err
	}
	if err := names.ValidateDatasetName(target); err != nil {
		return nil, err
	}
	args := []string{"clone"}
	keys := make([]string, 0, len(properties))
	for k := range properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-o", k+"="+properties[k])
	}
	args = append(args, snapshot, target)
	return args, nil
}

func (m *Manager) Clone(ctx context.Context, snapshot, target string, properties map[string]string) error {
	args, err := buildCloneArgs(snapshot, target, properties)
	if err != nil {
		return err
	}
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	_, err = runner(ctx, m.ZFSBin, args...)
	return err
}

// --- Promote ---------------------------------------------------------------

func buildPromoteArgs(name string) ([]string, error) {
	if err := names.ValidateDatasetName(name); err != nil {
		return nil, err
	}
	return []string{"promote", name}, nil
}

func (m *Manager) Promote(ctx context.Context, name string) error {
	args, err := buildPromoteArgs(name)
	if err != nil {
		return err
	}
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	_, err = runner(ctx, m.ZFSBin, args...)
	return err
}

// --- LoadKey / UnloadKey / ChangeKey --------------------------------------

func buildLoadKeyArgs(name, keylocation string, recursive bool) ([]string, error) {
	if err := names.ValidateDatasetName(name); err != nil {
		return nil, err
	}
	args := []string{"load-key"}
	if recursive {
		args = append(args, "-r")
	}
	if keylocation != "" {
		args = append(args, "-L", keylocation)
	}
	args = append(args, name)
	return args, nil
}

func (m *Manager) LoadKey(ctx context.Context, name, keylocation string, recursive bool) error {
	args, err := buildLoadKeyArgs(name, keylocation, recursive)
	if err != nil {
		return err
	}
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	_, err = runner(ctx, m.ZFSBin, args...)
	return err
}

func buildUnloadKeyArgs(name string, recursive bool) ([]string, error) {
	if err := names.ValidateDatasetName(name); err != nil {
		return nil, err
	}
	args := []string{"unload-key"}
	if recursive {
		args = append(args, "-r")
	}
	args = append(args, name)
	return args, nil
}

func (m *Manager) UnloadKey(ctx context.Context, name string, recursive bool) error {
	args, err := buildUnloadKeyArgs(name, recursive)
	if err != nil {
		return err
	}
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	_, err = runner(ctx, m.ZFSBin, args...)
	return err
}

func buildChangeKeyArgs(name string, properties map[string]string) ([]string, error) {
	if err := names.ValidateDatasetName(name); err != nil {
		return nil, err
	}
	args := []string{"change-key"}
	keys := make([]string, 0, len(properties))
	for k := range properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-o", k+"="+properties[k])
	}
	args = append(args, name)
	return args, nil
}

func (m *Manager) ChangeKey(ctx context.Context, name string, properties map[string]string) error {
	args, err := buildChangeKeyArgs(name, properties)
	if err != nil {
		return err
	}
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	_, err = runner(ctx, m.ZFSBin, args...)
	return err
}

// --- Send ------------------------------------------------------------------

// buildSendArgs emits flags in the order: -R -w -c -L -e -i <from> <snapshot>.
func buildSendArgs(snapshot string, opts SendOpts) ([]string, error) {
	if err := names.ValidateSnapshotName(snapshot); err != nil {
		return nil, err
	}
	if opts.IncrementalFrom != "" {
		// -i accepts either a full snapshot name or a short @name; only
		// validate when a full name is given (contains '@' before any '/').
		if strings.Contains(opts.IncrementalFrom, "@") {
			if err := names.ValidateSnapshotName(opts.IncrementalFrom); err != nil {
				return nil, fmt.Errorf("incremental from: %w", err)
			}
		}
	}
	args := []string{"send"}
	if opts.Recursive {
		args = append(args, "-R")
	}
	if opts.Raw {
		args = append(args, "-w")
	}
	if opts.Compressed {
		args = append(args, "-c")
	}
	if opts.LargeBlock {
		args = append(args, "-L")
	}
	if opts.EmbeddedData {
		args = append(args, "-e")
	}
	if opts.IncrementalFrom != "" {
		args = append(args, "-i", opts.IncrementalFrom)
	}
	args = append(args, snapshot)
	return args, nil
}

func (m *Manager) Send(ctx context.Context, snapshot string, opts SendOpts, w io.Writer) error {
	args, err := buildSendArgs(snapshot, opts)
	if err != nil {
		return err
	}
	runner := m.StreamRunner
	if runner == nil {
		runner = exec.RunStream
	}
	return runner(ctx, m.ZFSBin, nil, w, args...)
}

// --- Receive ---------------------------------------------------------------

func buildReceiveArgs(target string, opts RecvOpts) ([]string, error) {
	if err := names.ValidateDatasetName(target); err != nil {
		return nil, err
	}
	args := []string{"receive"}
	if opts.Force {
		args = append(args, "-F")
	}
	if opts.Resumable {
		args = append(args, "-s")
	}
	if opts.OriginSnapshot != "" {
		if err := names.ValidateSnapshotName(opts.OriginSnapshot); err != nil {
			return nil, fmt.Errorf("origin snapshot: %w", err)
		}
		args = append(args, "-o", "origin="+opts.OriginSnapshot)
	}
	args = append(args, target)
	return args, nil
}

func (m *Manager) Receive(ctx context.Context, target string, opts RecvOpts, r io.Reader) error {
	args, err := buildReceiveArgs(target, opts)
	if err != nil {
		return err
	}
	runner := m.StreamRunner
	if runner == nil {
		runner = exec.RunStream
	}
	return runner(ctx, m.ZFSBin, r, nil, args...)
}
