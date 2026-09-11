package main

import (
	"fmt"
	"time"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/debug"
)

// warnBefore is how long before expiry a session that did not renew gets a
// warning. Renewal runs inside the five-minute cache skew, so a session that
// is still this close to expiry is not going to renew.
const warnBefore = 2 * time.Minute

// notice is one notification to send.
type notice struct {
	ID, Title, Body string
}

// expiryNotices compares the previous and current session lists and returns
// the notifications due now. warned records, per session, the expiry that was
// already announced, so a renewed session warns again for its new expiry.
func expiryNotices(prev, cur []core.Session, now time.Time, warned map[string]time.Time) []notice {
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
		if left > 0 && left <= warnBefore && !warned[s.ID].Equal(*s.Expires) {
			warned[s.ID] = *s.Expires
			out = append(out, notice{
				ID:    "expiry-" + s.ID,
				Title: s.Name + " expires soon",
				Body:  fmt.Sprintf("About %d minute(s) left. Start it again to keep working.", int(left.Minutes())+1),
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
			out = append(out, notice{ID: "expired-" + s.ID, Title: s.Name + " expired", Body: "The session ended. Start it again when you need it."})
		}
	}
	return out
}

// notifier sends expiry notices through the OS notification center.
type notifier struct {
	svc    *notifications.NotificationService
	prev   []core.Session
	warned map[string]time.Time
}

func newNotifier(ns *notifications.NotificationService) *notifier {
	n := &notifier{svc: ns, warned: map[string]time.Time{}}
	go func() {
		if ok, err := ns.RequestNotificationAuthorization(); err != nil || !ok {
			debug.Logf("notify", "authorization: ok=%v err=%v", ok, err)
		}
	}()
	return n
}

// tick runs after every refresh with the current workspace and settings.
func (n *notifier) tick(w *core.Workspace, st core.Settings) {
	if w == nil {
		return
	}
	if st.NotifyOff {
		n.prev = w.Sessions
		return
	}
	for _, msg := range expiryNotices(n.prev, w.Sessions, time.Now(), n.warned) {
		if err := n.svc.SendNotification(notifications.NotificationOptions{ID: msg.ID, Title: msg.Title, Body: msg.Body}); err != nil {
			debug.Logf("notify", "%s: %v", msg.ID, err)
		}
	}
	n.prev = w.Sessions
}
