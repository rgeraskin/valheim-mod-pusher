package syncer

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func randomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return string(b)
}

// Live test against a real server. Run with e.g.
//
//	MOD_PUSHER_HOST=1.2.3.4 MOD_PUSHER_PORT=2266 go test ./internal/syncer -run Live -v
//
// It only touches /tmp/valheim-mod-pusher-it on the server.
func liveConfig(t *testing.T) Config {
	host := os.Getenv("MOD_PUSHER_HOST")
	if host == "" {
		t.Skip("MOD_PUSHER_HOST not set")
	}
	port, _ := strconv.Atoi(os.Getenv("MOD_PUSHER_PORT"))
	if port == 0 {
		port = 22
	}
	local := filepath.Join(t.TempDir(), "plugins")
	for name, body := range map[string]string{
		"Author-Mod/Mod.dll":         "dll",
		"Author-Mod/manifest.json":   "{}",
		"Other-Mod/sub dir/it's.txt": "quotes and spaces",
		"Loose.dll":                  randomString(4 << 20),
	} {
		p := filepath.Join(local, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return Config{
		LocalDir:   local,
		Host:       host,
		Port:       port,
		PluginsDir: "/tmp/valheim-mod-pusher-it/BepInEx/plugins",
		FollowUp:   `if docker exec valheim-server valheim-is-idle; then echo WOULD-RUN: docker restart; else echo "server was not restarted: there are players"; fi`,
	}
}

func TestLiveSync(t *testing.T) {
	c := liveConfig(t)
	ctx := context.Background()
	var out []string
	log := func(s string) { out = append(out, s); t.Log(s) }
	var last float64
	var mu sync.Mutex
	uploading := 0 // samples strictly inside the upload phase
	progress := func(v float64) {
		mu.Lock()
		defer mu.Unlock()
		last = v
		if v > 0.31 && v < 0.99 {
			uploading++
		}
	}

	if err := TestConnection(ctx, c, log); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // second run must produce .bak
		if err := Sync(ctx, c, log, progress); err != nil {
			t.Fatal(err)
		}
	}
	if last != 1 {
		t.Errorf("progress ended at %v", last)
	}
	t.Logf("upload-phase progress samples: %d", uploading)
	if uploading == 0 {
		t.Error("upload progress never moved")
	}
	out = nil
	check := `cd /tmp/valheim-mod-pusher-it/BepInEx && find plugins plugins.bak -type f | sort && cat "plugins/Other-Mod/sub dir/it's.txt"; echo; rm -rf /tmp/valheim-mod-pusher-it`
	if err := runSSH(ctx, c, check, nil, log); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(out, "\n")
	for _, want := range []string{"plugins/Loose.dll", "plugins.bak/Author-Mod/Mod.dll", "plugins/Other-Mod/sub dir/it's.txt", "quotes and spaces"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestLiveUnknownHost(t *testing.T) {
	c := liveConfig(t)
	extraSSHOpts = []string{"-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile=" + filepath.Join(t.TempDir(), "kh"), "-o", "GlobalKnownHostsFile=" + filepath.Join(t.TempDir(), "gkh")}
	defer func() { extraSSHOpts = nil }()
	err := TestConnection(context.Background(), c, func(s string) { t.Log(s) })
	if !errors.Is(err, ErrHostUnknown) {
		t.Fatalf("want ErrHostUnknown, got %v", err)
	}
	keys, err := ScanHostKeys(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		t.Logf("%s %s", k.Type, k.Fingerprint)
	}
}
