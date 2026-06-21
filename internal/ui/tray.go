package ui

import (
	"os"

	"fyne.io/systray"
	"github.com/gotk3/gotk3/glib"
)

// setupTray registers a cross-platform system tray icon using fyne.io/systray.
// It does not block GTK's main loop, so it works alongside gotk3 on Linux,
// BSD, macOS and Windows.
func (a *App) setupTray() {
	systray.Register(a.onTrayReady, a.onTrayExit)
}

func (a *App) onTrayReady() {
	systray.SetTitle("Nozik")
	systray.SetTooltip("Nozik - AI music agent for Gozik")

	if iconPath := AssetPath("icons/nozik.png"); fileExists(iconPath) {
		if b, err := os.ReadFile(iconPath); err == nil {
			systray.SetIcon(b)
		}
	}

	showItem := systray.AddMenuItem("Show Nozik", "Show the main window")
	quitItem := systray.AddMenuItem("Quit", "Exit Nozik")

	go func() {
		for {
			select {
			case <-showItem.ClickedCh:
				a.showWindow()
			case <-quitItem.ClickedCh:
				a.quit()
			}
		}
	}()
}

func (a *App) onTrayExit() {
	// No explicit cleanup needed.
}

// showWindow presents and deiconifies the main window.
// It is safe to call from any goroutine.
func (a *App) showWindow() {
	if a.window == nil {
		return
	}
	glib.IdleAdd(func() {
		a.window.Present()
		a.window.Deiconify()
		a.window.ShowAll()
	})
}

// hideWindow hides the main window so only the tray icon remains visible.
// It is safe to call from any goroutine.
func (a *App) hideWindow() {
	if a.window == nil {
		return
	}
	glib.IdleAdd(func() { a.window.Hide() })
}

// quit cleanly exits the GTK application.
// It is safe to call from any goroutine.
func (a *App) quit() {
	glib.IdleAdd(func() { a.application.Quit() })
}
