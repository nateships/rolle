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
	"github.com/nateships/rolle/internal/version"
)

// EventOpenSettings asks the window to open the settings dialog.
const EventOpenSettings = "settings:open"

// EventStartNeedsLogin asks the window to sign in to an integration and then
// start a session. The tray sends it when a start needs the browser.
const EventStartNeedsLogin = "session:login"

// StartRequest is the payload of EventStartNeedsLogin.
type StartRequest struct {
	SessionID     string `json:"sessionId"`
	IntegrationID string `json:"integrationId"`
}

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

// trayIconColorWarn is the active icon with its dot turned amber, made once
// from the active variant so the two stay in step.
var trayIconColorWarn = sync.OnceValue(func() []byte {
	out, err := recolorDot(trayIconColorActive, trayIconColor, amber)
	if err != nil {
		debug.Logf("tray", "warn icon: %v", err)
		return trayIconColorActive
	}
	return out
})

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
	t.setIcon(false, false)
	t.item.SetTooltip("rolle")
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

// setIcon shows the state: plain, active, or active with something about to
// expire. macOS draws the template icon in the menu bar tint, so the warning
// goes into the label there.
func (t *tray) setIcon(active, warn bool) {
	if runtime.GOOS == "darwin" {
		t.item.SetTemplateIcon(trayIconTemplate)
		return
	}
	switch {
	case warn:
		t.item.SetIcon(trayIconColorWarn())
	case active:
		t.item.SetIcon(trayIconColorActive)
	default:
		t.item.SetIcon(trayIconColor)
	}
}

// onNotification runs the action a user took on an expiry notification.
// "start" starts the session again, "signin" opens the window on the
// sign-in for the portal, and a plain click opens the window.
func (t *tray) onNotification(action string, data map[string]any) {
	sessionID, _ := data["sessionId"].(string)
	integrationID, _ := data["integrationId"].(string)
	switch action {
	case actionStart:
		if w, err := t.svc.Load(); err == nil {
			if sess, err := app.FindSession(w, sessionID); err == nil {
				// The platform delivers the response on its notification
				// thread; a start makes network calls, so it runs apart.
				go t.start(*sess)
				return
			}
		}
	case actionSignIn:
		if integrationID != "" {
			t.app.Event.Emit(EventStartNeedsLogin, StartRequest{IntegrationID: integrationID})
		}
	}
	t.showWindow()
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

	active, inactive := splitForMenu(w.Sessions)
	sort.Slice(active, func(i, j int) bool { return active[i].Name < active[j].Name })

	menu.Add(fmt.Sprintf("rolle · %s", countLabel(len(active)))).SetEnabled(false)
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
	// Expiry notifications off silences the tray flag too.
	st := w.EffectiveSettings()
	warn := !st.NotifyOff && expiringSoon(w, time.Now(), st.NotifyLead())
	t.setIcon(len(active) > 0, warn)
	t.item.SetTooltip(tooltip(active))
	if runtime.GOOS == "darwin" {
		t.item.SetLabel(trayLabel(len(active), warn))
	}
}

// splitForMenu separates the sessions the menu shows: active ones first,
// then the inactive ones that are not hidden. A hidden session appears only
// while it runs.
func splitForMenu(sessions []core.Session) (active, inactive []core.Session) {
	for _, s := range sessions {
		switch {
		case s.Status == core.StatusActive:
			active = append(active, s)
		case s.Hidden:
		default:
			inactive = append(inactive, s)
		}
	}
	return active, inactive
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
	menu.Add("Open rolle").OnClick(func(*application.Context) { t.showWindow() })
	menu.Add("Report a problem…").OnClick(func(*application.Context) {
		if err := browser.Open(supportURL(version.Version, runtime.GOOS, runtime.GOARCH)); err != nil {
			debug.Logf("tray", "support: %v", err)
		}
	})
	menu.Add("Settings…").OnClick(func(*application.Context) {
		t.showWindow()
		t.app.Event.Emit(EventOpenSettings, struct{}{})
	})
	menu.Add("Quit rolle").OnClick(func(*application.Context) { t.app.Quit() })
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
		// The window runs the sign-in and starts the session after it.
		if app.LoginRequired(err) {
			t.app.Event.Emit(EventStartNeedsLogin, StartRequest{SessionID: sess.ID, IntegrationID: sess.IntegrationID})
		}
		t.showWindow()
	}
}

// needsInput reports whether starting the session needs an MFA code, which the
// tray cannot collect.
func needsInput(s core.Session) bool {
	return s.Status != core.StatusActive && s.AWS != nil && s.AWS.MFADevice != ""
}

// trayLabel is the macOS menu bar text: the active count, with an
// exclamation mark while something is about to expire.
func trayLabel(active int, warn bool) string {
	if active == 0 {
		return ""
	}
	if warn {
		return fmt.Sprintf("%d!", active)
	}
	return fmt.Sprintf("%d", active)
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
		return "rolle · no active sessions"
	}
	var next *time.Time
	for _, s := range active {
		if s.Expires != nil && (next == nil || s.Expires.Before(*next)) {
			next = s.Expires
		}
	}
	out := "rolle · " + countLabel(len(active))
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
