package aws

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// callback is what the browser redirect carried back to the loopback listener.
type callback struct{ code, err string }

// loopback is a one-shot listener for the browser redirect of an
// authorization code flow. Only the redirect that carries state counts.
type loopback struct {
	// redirect is the URI the authorization server sends the browser to.
	redirect string
	got      chan callback
	stop     func()
}

// listenLoopback opens a listener on a free port of 127.0.0.1 and serves the
// callback page on redirectPath.
func listenLoopback(state string) (*loopback, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	got := make(chan callback, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(redirectPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// Only the redirect that carries this login's state counts. Anything
		// else on the loopback port is ignored and does not consume the slot.
		if q.Get("state") != state {
			w.WriteHeader(http.StatusBadRequest)
			_ = callbackPage.Execute(w, callbackView{Class: "err", Title: "This page does not belong to the current sign-in", Text: "You can close this tab and try again from rolle."})
			return
		}
		// A redirect with neither a code nor an error is not an outcome. It
		// must not take the single slot a real redirect needs.
		if q.Get("code") == "" && q.Get("error") == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = callbackPage.Execute(w, callbackView{Class: "err", Title: "The sign-in did not complete", Text: "You can close this tab and try again from rolle."})
			return
		}
		view := callbackView{Class: "ok", Title: "Signed in", Text: "You can close this tab and return to rolle."}
		if q.Get("error") != "" {
			view = callbackView{Class: "err", Title: "Sign-in was not approved", Text: "You can close this tab and try again from rolle."}
		}
		_ = callbackPage.Execute(w, view)
		// The page is sent before the flow continues, so the tab never sees a dropped connection.
		select {
		case got <- callback{code: q.Get("code"), err: q.Get("error")}:
		default:
		}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	stop := func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}
	return &loopback{redirect: "http://" + ln.Addr().String() + redirectPath, got: got, stop: stop}, nil
}

// wait blocks until the browser returns with a code, the user declines, ctx
// ends, or the timeout passes. The handler forwards only the redirect that
// carries this login's state. prefix names the flow in errors.
func (l *loopback) wait(ctx context.Context, prefix string) (string, error) {
	var cb callback
	select {
	case cb = <-l.got:
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(10 * time.Minute):
		return "", fmt.Errorf("%s: login timed out", prefix)
	}
	if cb.err != "" {
		return "", fmt.Errorf("%s: %s", prefix, cb.err)
	}
	if cb.code == "" {
		return "", errors.New(prefix + ": callback did not match this login")
	}
	return cb.code, nil
}
