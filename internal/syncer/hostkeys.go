package syncer

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

type HostKey struct {
	Line        string // known_hosts line as printed by ssh-keyscan
	Type        string
	Fingerprint string
}

// sshOption returns the first value of an option from `ssh -G`, which resolves
// Host aliases and defaults the same way a real connection would.
func sshOption(ctx context.Context, c Config, name string) (string, error) {
	cmd := exec.CommandContext(ctx, "ssh", "-G", "-p", strconv.Itoa(c.Port), c.target())
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ssh -G failed: %w", err)
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		k, v, _ := strings.Cut(sc.Text(), " ")
		if strings.EqualFold(k, name) {
			return strings.Fields(v)[0], nil
		}
	}
	return "", fmt.Errorf("ssh -G has no %s", name)
}

// ScanHostKeys fetches the server's host keys so the user can compare fingerprints.
func ScanHostKeys(ctx context.Context, c Config) ([]HostKey, error) {
	host, err := sshOption(ctx, c, "hostname")
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "ssh-keyscan", "-p", strconv.Itoa(c.Port), host)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ssh-keyscan failed: %w", err)
	}
	var keys []HostKey
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		_, _, pub, _, _, err := ssh.ParseKnownHosts([]byte(line))
		if err != nil {
			continue
		}
		keys = append(keys, HostKey{Line: line, Type: pub.Type(), Fingerprint: ssh.FingerprintSHA256(pub)})
	}
	if len(keys) == 0 {
		return nil, errors.New("server returned no host keys")
	}
	return keys, nil
}

// TrustHostKeys appends keys to the user's known_hosts file (as ssh -G reports it).
func TrustHostKeys(ctx context.Context, c Config, keys []HostKey) (string, error) {
	path, err := sshOption(ctx, c, "userknownhostsfile")
	if err != nil {
		return "", err
	}
	if rest, ok := strings.CutPrefix(path, "~"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, rest)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	// start on a fresh line in case the file lacks a trailing newline
	var b strings.Builder
	b.WriteString("\n")
	for _, k := range keys {
		b.WriteString(k.Line + "\n")
	}
	if _, err := f.WriteString(b.String()); err != nil {
		_ = f.Close()
		return "", err
	}
	return path, f.Close()
}
