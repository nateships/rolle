package main

import (
	"fmt"
	"time"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/debug"
)

// portalWarnBefore is how long before an Identity Center sign-in expires the
// warning shows. Every session under the portal ends with it, and a new
// sign-in takes a browser round trip, so this lead is longer than a session's.
const portalWarnBefore = 15 * time.Minute

// Notification categories. Each carries one action button.
const (
	categoryStart  = "rolle-start"
	categorySignIn = "rolle-signin"
	actionStart    = "start"
	actionSignIn   = "signin"
)

// notice is one notification to send. Category names the action button, and
// Data is what the action needs: a session or an integration ID.
type notice struct {
	ID, Title, Body string
	Category        string
	Data            map[string]any
}

// expiryNotices compares the previous and current session lists and returns
// the notifications due now. warned records, per session, the expiry that was
// already announced, so a renewed session warns again for its new expiry.
// lead is how long before expiry the warning shows.
func expiryNotices(prev, cur []core.Session, now time.Time, warned map[string]time.Time, lead time.Duration) []notice {
	var out []notice
	before := map[string]core.Session{}
	for _, s := range prev {
		before[s.ID] = s
	}
	for _, s := range cur {
		if s.Status != core.StatusActive || s.Expires == nil {
			continue
		}
		left := s.Expires.Sub(now)
		if left > 0 && left <= lead && !warned[s.ID].Equal(*s.Expires) {
			warned[s.ID] = *s.Expires
			out = append(out, notice{
				ID:       "expiry-" + s.ID,
				Title:    s.Name,
				Body:     fmt.Sprintf("Expires in %d min.", int(left.Minutes())+1),
				Category: categoryStart,
				Data:     map[string]any{"sessionId": s.ID},
			})
		}
	}
	// A session that was active and is now inactive with its expiry in the past ended by itself.
	for _, s := range cur {
		p, was := before[s.ID]
		if !was || p.Status != core.StatusActive || s.Status == core.StatusActive {
			continue
		}
		if p.Expires != nil && !p.Expires.After(now) {
			delete(warned, s.ID)
			out = append(out, notice{
				ID:       "expired-" + s.ID,
				Title:    s.Name,
				Body:     "Expired.",
				Category: categoryStart,
				Data:     map[string]any{"sessionId": s.ID},
			})
		}
	}
	return out
}

// portalNotices warns before an Identity Center sign-in expires, for portals
// that have an active session. warned records the expiry already announced
// per integration, so a new sign-in warns again for its own expiry.
func portalNotices(w *core.Workspace, now time.Time, warned map[string]time.Time) []notice {
	var out []notice
	for _, in := range w.Integrations {
		// portalDue is false for an integration without a portal token, so
		// the expiry is read only after the check.
		left, ok := portalDue(w, in, now)
		if !ok {
			continue
		}
		exp := *in.AWSSSO.TokenExpires
		key := "portal-" + in.ID
		if warned[key].Equal(exp) {
			continue
		}
		warned[key] = exp
		out = append(out, notice{
			ID:       key,
			Title:    in.Alias + " sign-in",
			Body:     fmt.Sprintf("Expires in %d min. Its sessions end with it.", int(left.Minutes())+1),
			Category: categorySignIn,
			Data:     map[string]any{"integrationId": in.ID},
		})
	}
	return out
}

// portalDue reports whether the Identity Center sign-in of in ends inside
// portalWarnBefore while a session under it is active, and how long is left.
// A login with a refresh token renews itself, so it is never due.
func portalDue(w *core.Workspace, in core.Integration, now time.Time) (time.Duration, bool) {
	if in.AWSSSO == nil || in.AWSSSO.TokenExpires == nil || in.AWSSSO.Renews || !hasActive(w.Sessions, in.ID) {
		return 0, false
	}
	left := in.AWSSSO.TokenExpires.Sub(now)
	return left, left > 0 && left <= portalWarnBefore
}

func hasActive(sessions []core.Session, integrationID string) bool {
	for _, s := range sessions {
		if s.IntegrationID == integrationID && s.Status == core.StatusActive {
			return true
		}
	}
	return false
}

// expiringCount is the number of items the tray flags: active sessions
// inside lead, and portal sign-ins inside portalWarnBefore that still have
// active sessions. The Dock badge shows it.
func expiringCount(w *core.Workspace, now time.Time, lead time.Duration) int {
	n := 0
	for _, s := range w.Sessions {
		if s.Status == core.StatusActive && s.Expires != nil && s.Expires.After(now) && s.Expires.Sub(now) <= lead {
			n++
		}
	}
	for _, in := range w.Integrations {
		if _, ok := portalDue(w, in, now); ok {
			n++
		}
	}
	return n
}

// notifier sends expiry notices through the OS notification center.
type notifier struct {
	svc    *notifications.NotificationService
	prev   []core.Session
	warned map[string]time.Time
}

// newNotifier registers the notification categories and routes the action a
// user takes on a notification to act. A click on the notification body
// arrives with an empty action. The categories are registered before the
// first send: a notice sent with an unknown category shows no button, and the
// platforms report no error for it.
func newNotifier(ns *notifications.NotificationService, act func(action string, data map[string]any)) *notifier {
	n := &notifier{svc: ns, warned: map[string]time.Time{}}
	if ns == nil {
		debug.Logf("notify", "no notification center: not running from an app bundle")
		return n
	}
	for _, c := range []notifications.NotificationCategory{
		{ID: categoryStart, Actions: []notifications.NotificationAction{{ID: actionStart, Title: "Start again"}}},
		{ID: categorySignIn, Actions: []notifications.NotificationAction{{ID: actionSignIn, Title: "Sign in"}}},
	} {
		if err := ns.RegisterNotificationCategory(c); err != nil {
			debug.Logf("notify", "category %s: %v", c.ID, err)
		}
	}
	// The authorization prompt can wait on the user; it does not block the refresh loop.
	go func() {
		if ok, err := ns.RequestNotificationAuthorization(); err != nil || !ok {
			debug.Logf("notify", "authorization: ok=%v err=%v", ok, err)
		}
	}()
	if act != nil {
		ns.OnNotificationResponse(func(r notifications.NotificationResult) {
			if r.Error != nil {
				debug.Logf("notify", "response: %v", r.Error)
				return
			}
			act(userAction(r.Response.ActionIdentifier), r.Response.UserInfo)
		})
	}
	return n
}

// userAction maps a platform action identifier to ours. The platforms name
// a plain click on the notification in their own way; that becomes "".
func userAction(id string) string {
	switch id {
	case actionStart, actionSignIn:
		return id
	}
	return ""
}

// tick runs after every refresh with the current workspace and settings.
func (n *notifier) tick(w *core.Workspace, st core.Settings) {
	if w == nil {
		return
	}
	if st.NotifyOff || n.svc == nil {
		n.prev = w.Sessions
		return
	}
	now := time.Now()
	msgs := expiryNotices(n.prev, w.Sessions, now, n.warned, st.NotifyLead())
	msgs = append(msgs, portalNotices(w, now, n.warned)...)
	for _, msg := range msgs {
		opts := notifications.NotificationOptions{ID: msg.ID, Title: msg.Title, Body: msg.Body, CategoryID: msg.Category, Data: msg.Data}
		if err := n.svc.SendNotificationWithActions(opts); err != nil {
			debug.Logf("notify", "%s: %v", msg.ID, err)
			continue
		}
		debug.Logf("notify", "sent %s: %s", msg.ID, msg.Title)
	}
	n.prev = w.Sessions
}
