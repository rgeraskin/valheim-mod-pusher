package main

import (
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// Renders the window to $MOD_PUSHER_SCREENSHOT for a visual check.
func TestScreenshot(t *testing.T) {
	out := os.Getenv("MOD_PUSHER_SCREENSHOT")
	if out == "" {
		t.Skip("MOD_PUSHER_SCREENSHOT not set")
	}
	a := test.NewApp()
	defer a.Quit()
	w := test.NewWindow(nil)
	a.Preferences().SetString("localDir", `C:\Users\rg\AppData\Roaming\com.kesomannen.gale\valheim\profiles\Dedicated\BepInEx\plugins`)
	a.Preferences().SetString("host", "135.106.217.244")
	a.Preferences().SetString("port", "2266")
	a.Preferences().SetString("pluginsDir", "/opt/valheim-server/data/bepinex/BepInEx/plugins")
	a.Preferences().SetBool("followUpOn", true)
	w.SetContent(buildUI(a.Preferences(), w))
	w.Resize(fyne.NewSize(760, 680))
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, w.Canvas().Capture()); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
