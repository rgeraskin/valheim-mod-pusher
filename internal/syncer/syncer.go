// Package syncer uploads a local BepInEx plugins folder to a server over the
// system ssh client, so the user's ssh config, agent and known_hosts all apply.
package syncer

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	LocalDir   string // local folder to push (~/ allowed), e.g. a Gale profile's BepInEx/plugins
	Host       string
	Port       int
	User       string // empty = whatever ssh config says
	PluginsDir string // path on the server: absolute or ~/relative to the ssh user's home
	FollowUp   string // shell command run on the server after a sync; empty = skip
}

// Logger receives output lines; Progress receives 0..1.
type (
	Logger   func(string)
	Progress func(float64)
)

// ErrHostUnknown means ssh refused the server because its key is not in known_hosts.
var ErrHostUnknown = errors.New("server host key is not trusted yet")

func (c Config) target() string {
	if c.User != "" {
		return c.User + "@" + c.Host
	}
	return c.Host
}

func (c Config) validate() error {
	if c.Host == "" {
		return errors.New("server host is empty")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port %d", c.Port)
	}
	absolute := strings.HasPrefix(c.PluginsDir, "/") || strings.HasPrefix(c.PluginsDir, "~/")
	if !absolute || len(c.PluginsDir) < 3 {
		return errors.New("server plugins dir must be an absolute path or start with ~/")
	}
	return nil
}

// expandHome resolves a leading ~/ in a local path.
func expandHome(p string) (string, error) {
	rest, ok := strings.CutPrefix(filepath.ToSlash(p), "~/")
	if !ok {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, filepath.FromSlash(rest)), nil
}

// remotePath quotes a server path for sh, keeping a leading ~/ expandable.
func remotePath(p string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return `"$HOME"/` + shQuote(rest)
	}
	return shQuote(p)
}

// TestConnection checks that ssh works and reports whether the plugins dir exists.
func TestConnection(ctx context.Context, c Config, log Logger) error {
	if err := c.validate(); err != nil {
		return err
	}
	cmd := fmt.Sprintf(`echo "connected as $(whoami)@$(hostname)"; `+
		`if [ -d %[1]s ]; then echo "plugins dir exists: %[2]s"; else echo "plugins dir missing, will be created: %[2]s"; fi`,
		remotePath(c.PluginsDir), c.PluginsDir)
	return runSSH(ctx, c, cmd, nil, log)
}

// Sync uploads the local plugins dir to the server and swaps them in,
// keeping the previous dir as <dir>.bak, then runs the follow-up command.
func Sync(ctx context.Context, c Config, log Logger, progress Progress) error {
	if err := c.validate(); err != nil {
		return err
	}
	src, err := expandHome(c.LocalDir)
	if err != nil {
		return err
	}
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		return fmt.Errorf("plugins dir not found: %s", src)
	}

	total, files, err := measure(src)
	if err != nil {
		return err
	}
	log(fmt.Sprintf("Packing %d files (%.1f MB) from %s", files, float64(total)/1e6, src))

	// The archive goes through a temp file rather than a pipe: ssh shares the
	// file offset with us, which gives upload progress, and Windows ssh.exe was
	// seen hanging on pipe stdin (at least when started from an ssh session).
	tmp, err := os.CreateTemp("", "valheim-mod-pusher-*.tar.gz")
	if err != nil {
		return err
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	const packShare = 0.3
	if err := writeTarGz(tmp, src, total, func(v float64) { progress(v * packShare) }); err != nil {
		return err
	}
	size, err := tmp.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	log(fmt.Sprintf("Uploading %.1f MB...", float64(size)/1e6))

	// ssh shares the file offset with us, so polling it tracks the upload
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(200 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if pos, err := tmp.Seek(0, io.SeekCurrent); err == nil && size > 0 {
					progress(packShare + (1-packShare)*0.99*float64(pos)/float64(size))
				}
			}
		}
	}()

	// tar reads the archive until EOF, then the swap runs in the same session
	remote := fmt.Sprintf(`set -e
dst=%s
rm -rf "$dst.new"
mkdir -p "$dst.new"
tar -xzf - -C "$dst.new"
rm -rf "$dst.bak"
if [ -d "$dst" ]; then mv "$dst" "$dst.bak"; fi
mv "$dst.new" "$dst"
echo "Synced to $dst:"
ls -1 "$dst"`, remotePath(c.PluginsDir))

	err = runSSH(ctx, c, remote, tmp, log)
	close(stop)
	if err != nil {
		return err
	}
	progress(1)

	if strings.TrimSpace(c.FollowUp) == "" {
		return nil
	}
	log("Running follow-up command...")
	return runSSH(ctx, c, c.FollowUp, nil, log)
}

func measure(root string) (total int64, files int, err error) {
	err = filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		files++
		return nil
	})
	return total, files, err
}

func writeTarGz(w io.Writer, root string, total int64, progress Progress) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	var done int64
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." {
			return err
		}
		// skip symlinks and other special files
		if !d.IsDir() && !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel) // Windows paths -> tar paths
		hdr.Uid, hdr.Gid, hdr.Uname, hdr.Gname = 0, 0, "", ""
		if d.IsDir() {
			hdr.Name += "/"
			hdr.Mode = 0o755
		} else {
			hdr.Mode = 0o644
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		n, err := io.Copy(tw, f)
		done += n
		if total > 0 {
			progress(float64(done) / float64(total))
		}
		return err
	})
	if err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// extraSSHOpts lets tests point ssh at a scratch known_hosts.
var extraSSHOpts []string

// runSSH runs a remote command, streaming stdout/stderr lines to log.
func runSSH(ctx context.Context, c Config, remote string, stdin io.Reader, log Logger) error {
	args := []string{
		"-p", strconv.Itoa(c.Port),
		"-o", "BatchMode=yes", // never prompt: there is no terminal to answer
		"-o", "ConnectTimeout=15",
	}
	args = append(args, extraSSHOpts...)
	args = append(args, c.target(), remote)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	hideWindow(cmd)
	cmd.Stdin = stdin

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("cannot start ssh (is OpenSSH installed?): %w", err)
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		errLines []string
	)
	pump := func(r io.Reader, isErr bool) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			if isErr && isNoise(line) {
				continue
			}
			if isErr {
				mu.Lock()
				errLines = append(errLines, line)
				mu.Unlock()
			}
			log(line)
		}
	}
	wg.Add(2)
	go pump(stdout, false)
	go pump(stderr, true)
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		all := strings.Join(errLines, "\n")
		if strings.Contains(all, "Host key verification failed") && !strings.Contains(all, "IDENTIFICATION HAS CHANGED") {
			return ErrHostUnknown
		}
		return fmt.Errorf("ssh failed: %w", err)
	}
	return nil
}

// isNoise filters OpenSSH's informational post-quantum warning.
func isNoise(line string) bool {
	return strings.HasPrefix(line, "** ") &&
		(strings.Contains(line, "post-quantum") || strings.Contains(line, "store now, decrypt later") ||
			strings.Contains(line, "pq.html"))
}

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
