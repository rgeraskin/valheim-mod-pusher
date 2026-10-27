package syncer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestTarGzLayout(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"Author-Mod/Mod.dll", "Author-Mod/sub/a.txt", "Loose.dll"} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	var last float64
	if err := writeTarGz(&buf, root, 100, func(v float64) { last = v }); err != nil {
		t.Fatal(err)
	}
	gz, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
		if strings.Contains(h.Name, `\`) {
			t.Errorf("backslash in tar name %q", h.Name)
		}
	}
	sort.Strings(names)
	want := "Author-Mod/ Author-Mod/Mod.dll Author-Mod/sub/ Author-Mod/sub/a.txt Loose.dll"
	if got := strings.Join(names, " "); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if last <= 0 {
		t.Error("progress not reported")
	}
}

func TestShQuote(t *testing.T) {
	if got := shQuote(`/a b/it's`); got != `'/a b/it'\''s'` {
		t.Errorf("got %s", got)
	}
}
