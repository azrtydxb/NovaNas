package scheduler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	osexec "os/exec"
	"strconv"

	"github.com/novanas/nova-nas/internal/host/zfs/names"
	storedb "github.com/novanas/nova-nas/internal/store/gen"
)

// SSHTransport ships a zfs send stream to a remote box by spawning
// "ssh ... zfs receive <dst>" and copying the local stream into ssh's
// stdin. It is the production Transport; tests should provide their own.
//
// Tradeoff: we shell out to /usr/bin/ssh rather than embedding
// golang.org/x/crypto/ssh because (a) it's one fewer dependency, (b)
// ssh_config (Hostname, ControlMaster, etc.) just works, and (c) ssh
// already handles host key trust-on-first-use.
type SSHTransport struct {
	// SSHBin defaults to "ssh" if empty.
	SSHBin string
	// RemoteZFSBin defaults to "zfs" if empty (the binary name on the
	// remote host; resolved via the remote's PATH).
	RemoteZFSBin string
	// StrictHostKeyChecking is the value of -o StrictHostKeyChecking.
	// Empty defaults to "accept-new" — this is the same trust-on-first-
	// use posture as the OpenSSH default for new operators, but we make
	// it explicit so behavior doesn't depend on the operator's
	// ~/.ssh/config.
	StrictHostKeyChecking string
}

// Receive opens ssh and runs zfs receive on the remote host, copying
// from r into ssh's stdin until r returns EOF or ssh exits.
func (t *SSHTransport) Receive(ctx context.Context, target *storedb.ReplicationTarget, dst string, r io.Reader) error {
	if target == nil {
		return errors.New("ssh transport: nil target")
	}
	if err := names.ValidateDatasetName(dst); err != nil {
		return fmt.Errorf("ssh transport: invalid dst dataset: %w", err)
	}
	sshBin := t.SSHBin
	if sshBin == "" {
		sshBin = "ssh"
	}
	zfsBin := t.RemoteZFSBin
	if zfsBin == "" {
		zfsBin = "zfs"
	}
	strict := t.StrictHostKeyChecking
	if strict == "" {
		strict = "accept-new"
	}
	port := int(target.Port)
	if port == 0 {
		port = 22
	}
	args := []string{
		"-i", target.SshKeyPath,
		"-p", strconv.Itoa(port),
		"-o", "StrictHostKeyChecking=" + strict,
		"-o", "BatchMode=yes",
		target.SshUser + "@" + target.Host,
		// Quote the dst on the remote shell. Dataset names are validated
		// above; the only chars allowed are [A-Za-z0-9 _.:/-], all of
		// which are shell-safe inside double quotes.
		zfsBin + " receive \"" + dst + "\"",
	}
	cmd := osexec.CommandContext(ctx, sshBin, args...)
	cmd.Stdin = r
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = io.Discard
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ssh receive: %w: %s", err, stderr.String())
	}
	return nil
}
