package main

import (
	"context"
	_ "embed"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/browser"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/debug"
	"github.com/nateships/rolle/internal/terminal"
)

// EventOpenSettings asks the window to open the settings dialog.
const EventOpenSettings = "settings:open"

// macOS draws template icons in the menu bar tint; the label shows the count.
//
//go:embed build/trayicon.png
var trayIconTemplate []byte

// Windows and Linux trays show the icon in color. The active variant carries a dot.
//
//go:embed build/trayicon-color.png
var trayIconColor []byte

//go:embed build/trayicon-color-active.png
var trayIconColorActive []byte

// tray is the system tray icon. Its menu starts and stops sessions, opens the
// console or a terminal, copies credentials, and opens the window.
type tray struct {
	app    *application.App
	svc    *app.Service
	window *application.WebviewWindow
	item   *application.SystemTray
	mu     sync.Mutex
	active int
}

func newTray(a *application.App, svc *app.Service, window *application.WebviewWindow) *tray {
	t := &tray{app: a, svc: svc, window: window}
	t.item = a.SystemTray.New()
	t.setIcon(false)
	t.item.SetTooltip("Rolle")
	t.rebuild()
	a.Event.On(EventWorkspaceChanged, func(*application.CustomEvent) { t.rebuild() })
	// Countdowns in the menu refresh once a minute while something is active.
	go func() {
		for range time.Tick(time.Minute) {
			t.mu.Lock()
			active := t.active
			t.mu.Unlock()
			if active > 0 {
				t.rebuild()
			}
		}
	}()
	return t
}

func (t *tray) setIcon(active bool) {
	if runtime.GOOS == "darwin" {
		t.item.SetTemplateIcon(trayIconTemplate)
		return
	}
	if active {
		t.item.SetIcon(trayIconColorActive)
	} else {
		t.item.SetIcon(trayIconColor)
	}
}

// rebuild regenerates the menu from the current workspace.
func (t *tray) rebuild() {
	menu := t.app.NewMenu()
	w, err := t.svc.Load()
	if err != nil {
		// Keep Open and Quit reachable when the workspace cannot be read.
		menu.Add("Workspace could not be read").SetEnabled(false)
		t.addFooter(menu)
		t.item.SetMenu(menu)
		return
	}

	var active, inactive []core.Session
	for _, s := range w.Sessions {
		if s.Status == core.StatusActive {
			active = append(active, s)
		} else {
			inactive = append(inactive, s)
		}
	}
	sort.Slice(active, func(i, j int) bool { return active[i].Name < active[j].Name })

	menu.Add(fmt.Sprintf("Rolle · %s", countLabel(len(active)))).SetEnabled(false)
	menu.AddSeparator()

	if len(w.Sessions) == 0 {
		menu.Add("No sessions yet").SetEnabled(false)
	}

	// Active sessions: one submenu each with the session actions.
	if len(active) > 0 {
		for _, s := range active {
			sess := s
			label := sess.Name
			if sess.Expires != nil {
				label = fmt.Sprintf("%s · %s", sess.Name, until(*sess.Expires))
			}
			t.addSessionMenu(menu.AddSubmenu(label), sess)
		}
		menu.Add("Stop all").OnClick(func(*application.Context) {
			for _, s := range active {
				if err := t.svc.Stop(s.ID); err != nil {
					debug.Logf("tray", "stop %s: %v", s.Name, err)
				}
			}
		})
		menu.AddSeparator()
	}

	// Favorites that are not active start with one click.
	var favs []core.Session
	for _, s := range inactive {
		if s.Favorite {
			favs = append(favs, s)
		}
	}
	if len(favs) > 0 {
		menu.Add("Favorites").SetEnabled(false)
		for _, s := range favs {
			t.addStartItem(menu, s, s.Name)
		}
		menu.AddSeparator()
	}

	// Every other inactive session, grouped by provider and AWS account.
	t.addProviderMenus(menu, w, inactive)

	t.addFooter(menu)
	t.item.SetMenu(menu)

	t.mu.Lock()
	t.active = len(active)
	t.mu.Unlock()
	t.setIcon(len(active) > 0)
	t.item.SetTooltip(tooltip(active))
	if runtime.GOOS == "darwin" {
		if len(active) > 0 {
			t.item.SetLabel(fmt.Sprintf("%d", len(active)))
		} else {
			t.item.SetLabel("")
		}
	}
}

// addProviderMenus adds AWS, Azure, and Google Cloud submenus with the inactive sessions.
func (t *tray) addProviderMenus(menu *application.Menu, w *core.Workspace, inactive []core.Session) {
	byCloud := map[core.Cloud][]core.Session{}
	for _, s := range inactive {
		byCloud[s.Kind.Cloud()] = append(byCloud[s.Kind.Cloud()], s)
	}
	for _, c := range []struct {
		cloud core.Cloud
		title string
	}{{core.CloudAWS, "AWS"}, {core.CloudAzure, "Azure"}, {core.CloudGCP, "Google Cloud"}} {
		sessions := byCloud[c.cloud]
		if len(sessions) == 0 {
			continue
		}
		sub := menu.AddSubmenu(fmt.Sprintf("%s · %d", c.title, len(sessions)))
		if c.cloud != core.CloudAWS {
			sort.Slice(sessions, func(i, j int) bool { return sessions[i].Name < sessions[j].Name })
			for _, s := range sessions {
				t.addStartItem(sub, s, s.Name)
			}
			continue
		}
		// Identity Center roles group under their account; other AWS sessions list directly.
		accounts := map[string][]core.Session{}
		var order []string
		var standalone []core.Session
		for _, s := range sessions {
			if s.Kind != core.KindAWSSSORole || s.AWS == nil {
				standalone = append(standalone, s)
				continue
			}
			key := accountLabel(w, s)
			if _, seen := accounts[key]; !seen {
				order = append(order, key)
			}
			accounts[key] = append(accounts[key], s)
		}
		sort.Strings(order)
		for _, key := range order {
			roles := accounts[key]
			sort.Slice(roles, func(i, j int) bool { return roles[i].AWS.RoleName < roles[j].AWS.RoleName })
			acct := sub.AddSubmenu(key)
			for _, s := range roles {
				t.addStartItem(acct, s, s.AWS.RoleName)
			}
		}
		if len(order) > 0 && len(standalone) > 0 {
			sub.AddSeparator()
		}
		sort.Slice(standalone, func(i, j int) bool { return standalone[i].Name < standalone[j].Name })
		for _, s := range standalone {
			t.addStartItem(sub, s, s.Name)
		}
	}
}

// accountLabel is the account name from "Account/Role", with the ID as fallback.
func accountLabel(_ *core.Workspace, s core.Session) string {
	if i := strings.Index(s.Name, "/"); i > 0 {
		return s.Name[:i]
	}
	return s.AWS.AccountID
}

// addStartItem adds an item that starts the session. Sessions that need an
// MFA code stay disabled; the window collects the code.
func (t *tray) addStartItem(menu *application.Menu, sess core.Session, label string) {
	item := menu.Add(label)
	if needsInput(sess) {
		item.SetLabel(label + " · needs MFA in the app")
		item.SetEnabled(false)
		return
	}
	item.OnClick(func(*application.Context) { t.start(sess) })
}

// addSessionMenu fills the submenu of an active session.
func (t *tray) addSessionMenu(menu *application.Menu, sess core.Session) {
	menu.Add("Stop").OnClick(func(*application.Context) {
		if err := t.svc.Stop(sess.ID); err != nil {
			debug.Logf("tray", "stop %s: %v", sess.Name, err)
		}
	})
	menu.AddSeparator()
	menu.Add("Open console").OnClick(func(*application.Context) {
		c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		u, err := t.svc.ConsoleURLFor(c, sess.ID)
		if err == nil {
			err = browser.Open(u)
		}
		if err != nil {
			debug.Logf("tray", "console %s: %v", sess.Name, err)
		}
	})
	menu.Add("Open terminal").OnClick(func(*application.Context) {
		c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := t.svc.OpenTerminal(c, sess.ID); err != nil {
			debug.Logf("tray", "terminal %s: %v", sess.Name, err)
		}
	})
	menu.Add("Copy credentials as env").OnClick(func(*application.Context) {
		c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		env, err := t.svc.SessionEnv(c, sess.ID)
		if err != nil {
			debug.Logf("tray", "env %s: %v", sess.Name, err)
			return
		}
		t.app.Clipboard.SetText(terminal.Exports(env, runtime.GOOS == "windows"))
	})
	if sess.Kind.Cloud() == core.CloudAWS {
		menu.Add("Copy profile command").OnClick(func(*application.Context) {
			t.app.Clipboard.SetText("aws --profile " + app.ProfileName(&sess))
		})
	}
}

func (t *tray) addFooter(menu *application.Menu) {
	menu.AddSeparator()
	menu.Add("Open Rolle").OnClick(func(*application.Context) { t.showWindow() })
	menu.Add("Settings…").OnClick(func(*application.Context) {
		t.showWindow()
		t.app.Event.Emit(EventOpenSettings, struct{}{})
	})
	menu.Add("Quit Rolle").OnClick(func(*application.Context) { t.app.Quit() })
}

func (t *tray) showWindow() {
	t.window.Show()
	t.window.Focus()
}

func (t *tray) start(sess core.Session) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := t.svc.Start(ctx, sess.ID, app.StartOptions{}); err != nil {
		debug.Logf("tray", "start %s: %v", sess.Name, err)
		// A sign-in or a code is needed: the window handles both.
		t.showWindow()
	}
}

// needsInput reports whether starting the session needs an MFA code, which the
// tray cannot collect.
func needsInput(s core.Session) bool {
	return s.Status != core.StatusActive && s.AWS != nil && s.AWS.MFADevice != ""
}

func countLabel(n int) string {
	switch n {
	case 0:
		return "no active sessions"
	case 1:
		return "1 active session"
	}
	return fmt.Sprintf("%d active sessions", n)
}

// tooltip names the active count and the soonest expiry.
func tooltip(active []core.Session) string {
	if len(active) == 0 {
		return "Rolle · no active sessions"
	}
	var next *time.Time
	for _, s := range active {
		if s.Expires != nil && (next == nil || s.Expires.Before(*next)) {
			next = s.Expires
		}
	}
	out := "Rolle · " + countLabel(len(active))
	if next != nil {
		out += " · next expiry in " + until(*next)
	}
	return out
}

func until(exp time.Time) string {
	d := time.Until(exp)
	switch {
	case d <= 0:
		return "expired"
	case d >= time.Hour:
		return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return "<1m"
}
