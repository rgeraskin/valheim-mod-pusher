// Valheim Mod Pusher: upload a local folder (e.g. a Gale profile's BepInEx/plugins)
// to a server over ssh and run a follow-up command there.
package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/rgeraskin/valheim-mod-pusher/internal/syncer"
)

//go:embed Icon.png
var iconPNG []byte

// Defaults match valheim-server-docker
// (https://github.com/community-valheim-tools/valheim-server-docker) with the
// volumes from its README: $HOME/valheim-server/{config,data}.
const (
	defaultPluginsDir = "~/valheim-server/data/bepinex/BepInEx/plugins"
	defaultFollowUp   = `# valheim-server-docker: restart the container only if no players are online
if docker exec valheim-server valheim-is-idle; then docker restart valheim-server; else echo "server was not restarted: there are players"; fi`
)

// localDirExample is the hint under the local plugins dir: Gale's default
// profile location where Gale exists (Windows, Linux); on macOS, where no mod
// manager is widely used, BepInEx installed into the Steam game folder.
func localDirExample() string {
	switch runtime.GOOS {
	case "windows":
		return `For example: %APPDATA%\com.kesomannen.gale\valheim\profiles\<name>\BepInEx\plugins`
	case "linux":
		return "For example: ~/.local/share/com.kesomannen.gale/valheim/profiles/<name>/BepInEx/plugins"
	default:
		return "For example: ~/Library/Application Support/Steam/steamapps/common/Valheim/BepInEx/plugins"
	}
}

// Set by goreleaser / mise run build via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("valheim-mod-pusher %s (commit %s, built %s)\n", version, commit, date)
		return
	}
	a := app.NewWithID("io.github.rgeraskin.valheimmodpusher")
	a.SetIcon(fyne.NewStaticResource("Icon.png", iconPNG))
	w := a.NewWindow("Valheim Mod Pusher " + version)
	w.SetContent(buildUI(a.Preferences(), w))
	w.Resize(fyne.NewSize(760, 680))
	w.ShowAndRun()
}

func buildUI(prefs fyne.Preferences, w fyne.Window) fyne.CanvasObject {

	// --- inputs (restored from preferences) ---
	localDir := widget.NewEntry()
	localDir.SetPlaceHolder("BepInEx/plugins folder to push")
	localDir.SetText(prefs.String("localDir"))
	browse := widget.NewButton("Browse…", func() {
		dialog.ShowFolderOpen(func(dir fyne.ListableURI, err error) {
			if err == nil && dir != nil {
				localDir.SetText(dir.Path())
			}
		}, w)
	})

	host := widget.NewEntry()
	host.SetPlaceHolder("server address or ssh config alias")
	host.SetText(prefs.String("host"))
	port := widget.NewEntry()
	port.SetText(prefs.StringWithFallback("port", "22"))
	user := widget.NewEntry()
	user.SetPlaceHolder("optional, defaults to ssh config / local user")
	user.SetText(prefs.String("user"))
	pluginsDir := widget.NewEntry()
	pluginsDir.SetPlaceHolder(defaultPluginsDir)
	pluginsDir.SetText(prefs.StringWithFallback("pluginsDir", defaultPluginsDir))

	followUp := widget.NewMultiLineEntry()
	followUp.SetMinRowsVisible(3)
	followUp.Wrapping = fyne.TextWrapWord
	followUp.SetText(prefs.StringWithFallback("followUp", defaultFollowUp))
	followUpOn := widget.NewCheck("Run on the server after sync", func(on bool) {
		if on {
			followUp.Enable()
		} else {
			followUp.Disable()
		}
	})
	followUpOn.SetChecked(prefs.BoolWithFallback("followUpOn", true))
	followUpOn.OnChanged(followUpOn.Checked)

	form := widget.NewForm(
		&widget.FormItem{Text: "Local plugins dir", Widget: container.NewBorder(nil, nil, nil, browse, localDir),
			HintText: localDirExample()},
		widget.NewFormItem("Server", container.NewBorder(nil, nil, nil,
			container.NewHBox(widget.NewLabel("Port"), container.NewGridWrap(fyne.NewSize(80, port.MinSize().Height), port)), host)),
		widget.NewFormItem("SSH user", user),
		&widget.FormItem{Text: "Server plugins dir", Widget: pluginsDir,
			HintText: "Default: valheim-server-docker with its README volumes ($HOME/valheim-server/data)"},
		&widget.FormItem{Text: "Follow-up", Widget: container.NewVBox(followUpOn, followUp),
			HintText: "Default: restart the valheim-server-docker container if no players are online"},
	)

	// --- output ---
	var logMu sync.Mutex
	var logText strings.Builder
	logLabel := widget.NewLabel("")
	logLabel.TextStyle = fyne.TextStyle{Monospace: true}
	logLabel.Wrapping = fyne.TextWrapBreak
	logScroll := container.NewVScroll(logLabel)
	logScroll.SetMinSize(fyne.NewSize(0, 220))
	logf := func(line string) {
		logMu.Lock()
		logText.WriteString(line + "\n")
		text := logText.String()
		logMu.Unlock()
		fyne.Do(func() {
			logLabel.SetText(text)
			logScroll.ScrollToBottom()
		})
	}
	bar := widget.NewProgressBar()
	progress := func(v float64) { fyne.Do(func() { bar.SetValue(v) }) }

	// --- actions ---
	config := func() (syncer.Config, error) {
		p, err := strconv.Atoi(strings.TrimSpace(port.Text))
		if err != nil {
			return syncer.Config{}, fmt.Errorf("invalid port %q", port.Text)
		}
		c := syncer.Config{
			LocalDir:   strings.TrimSpace(localDir.Text),
			Host:       strings.TrimSpace(host.Text),
			Port:       p,
			User:       strings.TrimSpace(user.Text),
			PluginsDir: strings.TrimSpace(pluginsDir.Text),
		}
		if followUpOn.Checked {
			c.FollowUp = strings.TrimSpace(followUp.Text)
		}
		prefs.SetString("localDir", c.LocalDir)
		prefs.SetString("host", c.Host)
		prefs.SetString("port", port.Text)
		prefs.SetString("user", c.User)
		prefs.SetString("pluginsDir", c.PluginsDir)
		prefs.SetBool("followUpOn", followUpOn.Checked)
		prefs.SetString("followUp", followUp.Text)
		return c, nil
	}

	// ask blocks a worker goroutine on a confirm dialog shown on the UI thread
	ask := func(title, msg string) bool {
		ch := make(chan bool, 1)
		fyne.Do(func() { dialog.ShowConfirm(title, msg, func(ok bool) { ch <- ok }, w) })
		return <-ch
	}

	trustHost := func(ctx context.Context, c syncer.Config) bool {
		logf("Server host key is unknown, fetching fingerprints...")
		keys, err := syncer.ScanHostKeys(ctx, c)
		if err != nil {
			logf("Error: " + err.Error())
			return false
		}
		var b strings.Builder
		b.WriteString("This computer has never connected to " + c.Host + " before.\n" +
			"Trust it only if these fingerprints match the server's:\n\n")
		for _, k := range keys {
			b.WriteString(k.Type + "  " + k.Fingerprint + "\n")
		}
		if !ask("Trust server?", b.String()) {
			logf("Host key not trusted.")
			return false
		}
		path, err := syncer.TrustHostKeys(ctx, c, keys)
		if err != nil {
			logf("Error: " + err.Error())
			return false
		}
		logf("Added host keys to " + path)
		return true
	}

	var (
		testBtn, syncBtn, cancelBtn *widget.Button
		cancel                      context.CancelFunc
	)
	setBusy := func(busy bool) {
		for _, b := range []*widget.Button{testBtn, syncBtn} {
			if busy {
				b.Disable()
			} else {
				b.Enable()
			}
		}
		if busy {
			cancelBtn.Enable()
		} else {
			cancelBtn.Disable()
		}
	}

	run := func(title string, fn func(context.Context, syncer.Config) error) {
		c, err := config()
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		var ctx context.Context
		ctx, cancel = context.WithCancel(context.Background())
		setBusy(true)
		bar.SetValue(0)
		logf(fmt.Sprintf("── %s  %s ──", title, time.Now().Format("15:04:05")))
		go func() {
			err := fn(ctx, c)
			if errors.Is(err, syncer.ErrHostUnknown) && trustHost(ctx, c) {
				err = fn(ctx, c)
			}
			switch {
			case ctx.Err() != nil:
				logf("Cancelled.")
			case err != nil:
				logf("Error: " + err.Error())
			default:
				logf("Done.")
			}
			fyne.Do(func() { setBusy(false) })
		}()
	}

	testBtn = widget.NewButton("Test connection", func() {
		run("Test connection", func(ctx context.Context, c syncer.Config) error {
			return syncer.TestConnection(ctx, c, logf)
		})
	})
	syncBtn = widget.NewButton("Sync", func() {
		c, err := config()
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		msg := fmt.Sprintf("Replace the server's plugins with the ones from\n%s?\n\nThe current server dir is kept as %s.bak.",
			c.LocalDir, c.PluginsDir)
		dialog.ShowConfirm("Sync mods", msg, func(ok bool) {
			if ok {
				run("Sync", func(ctx context.Context, c syncer.Config) error {
					return syncer.Sync(ctx, c, logf, progress)
				})
			}
		}, w)
	})
	syncBtn.Importance = widget.HighImportance
	cancelBtn = widget.NewButton("Cancel", func() {
		if cancel != nil {
			cancel()
		}
	})
	cancelBtn.Disable()

	buttons := container.NewHBox(testBtn, syncBtn, cancelBtn)
	top := container.NewVBox(form, buttons, bar)
	return container.NewPadded(container.NewBorder(top, nil, nil, nil, logScroll))
}
