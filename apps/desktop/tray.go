package main

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/core"
)

//go:embed build/trayicon.png
var trayIcon []byte

// tray keeps a system tray icon whose menu lists sessions for one-click start and stop.
type tray struct {
	app    *application.App
	svc    *app.Service
	window *application.WebviewWindow
	rolle  *RolleService
	item   *application.SystemTray
}

func newTray(a *application.App, svc *app.Service, rolle *RolleService, window *application.WebviewWindow) *tray {
	t := &tray{app: a, svc: svc, window: window, rolle: rolle}
	t.item = a.SystemTray.New()
	t.item.SetTemplateIcon(trayIcon)
	t.item.SetTooltip("Rolle")
	t.rebuild()
	a.Event.On(EventWorkspaceChanged, func(*application.CustomEvent) { t.rebuild() })
	return t
}

// rebuild regenerates the menu from the current workspace.
func (t *tray) rebuild() {
	w, err := t.svc.Load()
	if err != nil {
		return
	}
	menu := t.app.NewMenu()
	active := 0
	for _, s := range w.Sessions {
		if s.Status == core.StatusActive {
			active++
		}
	}
	header := menu.Add(fmt.Sprintf("Rolle · %d active", active))
	header.SetEnabled(false)
	menu.AddSeparator()

	if len(w.Sessions) == 0 {
		menu.Add("No sessions yet").SetEnabled(false)
	}
	for _, s := range w.Sessions {
		sess := s
		label := sess.Name
		if sess.Status == core.StatusActive && sess.Expires != nil {
			label = fmt.Sprintf("%s  (%s)", sess.Name, until(*sess.Expires))
		}
		item := menu.AddCheckbox(label, sess.Status == core.StatusActive)
		if needsInput(sess) {
			item.SetEnabled(false)
		}
		item.OnClick(func(*application.Context) { t.toggle(sess) })
	}
	menu.AddSeparator()
	menu.Add("Open Rolle").OnClick(func(*application.Context) {
		t.window.Show()
		t.window.Focus()
	})
	menu.Add("Quit").OnClick(func(*application.Context) { t.app.Quit() })
	t.item.SetMenu(menu)
	if active > 0 {
		t.item.SetLabel(fmt.Sprintf("%d", active))
	} else {
		t.item.SetLabel("")
	}
}

func (t *tray) toggle(sess core.Session) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if sess.Status == core.StatusActive {
		_ = t.svc.Stop(sess.ID)
	} else {
		_, _ = t.svc.Start(ctx, sess.ID, app.StartOptions{})
	}
	t.rolle.changed()
}

// needsInput reports whether starting the session needs an MFA code, which the
// tray cannot collect.
func needsInput(s core.Session) bool {
	return s.Status != core.StatusActive && s.AWS != nil && s.AWS.MFADevice != ""
}

func until(exp time.Time) string {
	d := time.Until(exp).Truncate(time.Minute)
	if d <= 0 {
		return "expired"
	}
	if d >= time.Hour {
		return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}
