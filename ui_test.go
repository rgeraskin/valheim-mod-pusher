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
	// first launch: only what the user must type, the rest are defaults
	a.Preferences().SetString("host", "203.0.113.10")
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
