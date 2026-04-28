package dataset

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
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

// --- Diff ------------------------------------------------------------------

// DatasetDiffEntry is one line of `zfs diff -H` output. Change is one of
// "+" (added), "-" (removed), "M" (modified), or "R" (renamed). For renames,
// Path holds the old path and NewPath holds the new path; for all other
// change types, NewPath is empty.
type DatasetDiffEntry struct {
	Change  string `json:"change"`
	Path    string `json:"path"`
	NewPath string `json:"newPath,omitempty"`
}

// validateDiffTarget accepts either a dataset name or a snapshot name. It
// tries dataset validation first and falls back to snapshot validation.
func validateDiffTarget(s string) error {
	if strings.Contains(s, "@") {
		return names.ValidateSnapshotName(s)
	}
	return names.ValidateDatasetName(s)
}

func buildDiffArgs(from, to string) ([]string, error) {
	if err := names.ValidateSnapshotName(from); err != nil {
		return nil, fmt.Errorf("from snapshot: %w", err)
	}
	args := []string{"diff", "-H", from}
	if to != "" {
		if err := validateDiffTarget(to); err != nil {
			return nil, fmt.Errorf("to target: %w", err)
		}
		args = append(args, to)
	}
	return args, nil
}

// parseDiff parses tab-separated `zfs diff -H` output. Each line is either
// "<change>\t<path>" for +, -, M; or "R\t<old>\t<new>" for renames.
func parseDiff(data []byte) ([]DatasetDiffEntry, error) {
	var out []DatasetDiffEntry
	sc := bufio.NewScanner(bytes.NewReader(data))
	// `zfs diff` paths can be very long; bump the buffer.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 2 {
			return nil, fmt.Errorf("zfs diff: bad line %q", line)
		}
		switch f[0] {
		case "+", "-", "M":
			if len(f) != 2 {
				return nil, fmt.Errorf("zfs diff: %s expects 2 fields, got %d in %q", f[0], len(f), line)
			}
			out = append(out, DatasetDiffEntry{Change: f[0], Path: f[1]})
		case "R":
			if len(f) != 3 {
				return nil, fmt.Errorf("zfs diff: R expects 3 fields, got %d in %q", len(f), line)
			}
			out = append(out, DatasetDiffEntry{Change: "R", Path: f[1], NewPath: f[2]})
		default:
			return nil, fmt.Errorf("zfs diff: unknown change code %q in %q", f[0], line)
		}
	}
	return out, sc.Err()
}

func (m *Manager) Diff(ctx context.Context, fromSnapshot, toName string) ([]DatasetDiffEntry, error) {
	args, err := buildDiffArgs(fromSnapshot, toName)
	if err != nil {
		return nil, err
	}
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	out, err := runner(ctx, m.ZFSBin, args...)
	if err != nil {
		return nil, err
	}
	return parseDiff(out)
}

// --- Bookmarks -------------------------------------------------------------

// Bookmark is a row from `zfs list -t bookmark`.
type Bookmark struct {
	Name         string `json:"name"`
	CreationUnix int64  `json:"creationUnix"`
}

// validateBookmarkName checks <pool/ds#name> form. The part before '#' must
// be a valid dataset name; the part after must be alphanumeric/dash/underscore
// and may not start with a dash.
func validateBookmarkName(s string) error {
	if strings.HasPrefix(s, "-") {
		return fmt.Errorf("bookmark name cannot start with '-'")
	}
	hash := strings.IndexByte(s, '#')
	if hash <= 0 {
		return fmt.Errorf("bookmark name must contain '<dataset>#<short>'")
	}
	if strings.Count(s, "#") != 1 {
		return fmt.Errorf("bookmark name must contain exactly one '#'")
	}
	if err := names.ValidateDatasetName(s[:hash]); err != nil {
		return err
	}
	short := s[hash+1:]
	if short == "" {
		return fmt.Errorf("bookmark short name empty")
	}
	for _, r := range short {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_'
		if !ok {
			return fmt.Errorf("bookmark short name has illegal character %q", r)
		}
	}
	return nil
}

func buildBookmarkArgs(snapshot, bookmarkName string) ([]string, error) {
	if err := names.ValidateSnapshotName(snapshot); err != nil {
		return nil, err
	}
	if err := validateBookmarkName(bookmarkName); err != nil {
		return nil, err
	}
	return []string{"bookmark", snapshot, bookmarkName}, nil
}

func (m *Manager) Bookmark(ctx context.Context, snapshot, bookmarkName string) error {
	args, err := buildBookmarkArgs(snapshot, bookmarkName)
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

func buildListBookmarksArgs(root string) ([]string, error) {
	args := []string{"list", "-H", "-p", "-t", "bookmark", "-o", "name,creation"}
	if root != "" {
		if err := names.ValidateDatasetName(root); err != nil {
			return nil, err
		}
		args = append(args, "-r", root)
	}
	return args, nil
}

func parseBookmarkList(data []byte) ([]Bookmark, error) {
	var out []Bookmark
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 2 {
			return nil, fmt.Errorf("zfs bookmark list: 2 fields expected, got %d in %q", len(f), line)
		}
		creation, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("zfs bookmark list: bad creation %q: %w", f[1], err)
		}
		out = append(out, Bookmark{Name: f[0], CreationUnix: creation})
	}
	return out, sc.Err()
}

func (m *Manager) ListBookmarks(ctx context.Context, root string) ([]Bookmark, error) {
	args, err := buildListBookmarksArgs(root)
	if err != nil {
		return nil, err
	}
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	out, err := runner(ctx, m.ZFSBin, args...)
	if err != nil {
		return nil, err
	}
	return parseBookmarkList(out)
}

func buildDestroyBookmarkArgs(bookmarkName string) ([]string, error) {
	if err := validateBookmarkName(bookmarkName); err != nil {
		return nil, err
	}
	return []string{"destroy", bookmarkName}, nil
}

func (m *Manager) DestroyBookmark(ctx context.Context, bookmarkName string) error {
	args, err := buildDestroyBookmarkArgs(bookmarkName)
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
