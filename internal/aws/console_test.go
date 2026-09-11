package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/nateships/rolle/internal/core"
)

var tempCreds = core.Credentials{AccessKeyID: "AKIA", SecretAccessKey: "sec/ret+", SessionToken: "tok=en"}

func TestConsoleURL(t *testing.T) {
	cases := []struct {
		region string
		dest   string
	}{
		{"", "https://console.aws.amazon.com/"},
		{"eu-west-1", "https://eu-west-1.console.aws.amazon.com/console/home?region=eu-west-1"},
		{"us-gov-west-1", "https://us-gov-west-1.console.aws.amazon.com/console/home?region=us-gov-west-1"},
	}
	for _, tc := range cases {
		t.Run("region="+tc.region, func(t *testing.T) {
			var gotPath string
			var gotQuery url.Values
			client, rt := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotQuery = r.URL.Path, r.URL.Query()
				fmt.Fprint(w, `{"SigninToken":"signin-123"}`)
			})
			got, err := ConsoleURL(context.Background(), client, tempCreds, tc.region)
			if err != nil {
				t.Fatal(err)
			}
			if rt.calls != 1 || gotPath != "/federation" {
				t.Fatalf("calls = %d, path = %q", rt.calls, gotPath)
			}
			if gotQuery.Get("Action") != "getSigninToken" || gotQuery.Has("SessionDuration") {
				t.Fatalf("token request query = %v", gotQuery)
			}
			var sess map[string]string
			if err := json.Unmarshal([]byte(gotQuery.Get("Session")), &sess); err != nil {
				t.Fatal(err)
			}
			if sess["sessionId"] != "AKIA" || sess["sessionKey"] != "sec/ret+" || sess["sessionToken"] != "tok=en" || len(sess) != 3 {
				t.Fatalf("session = %v", sess)
			}

			u, err := url.Parse(got)
			if err != nil {
				t.Fatal(err)
			}
			if u.Scheme+"://"+u.Host+u.Path != "https://signin.aws.amazon.com/federation" {
				t.Fatalf("login url = %s", got)
			}
			q := u.Query()
			if q.Get("Action") != "login" || q.Get("Issuer") != "rolle" || q.Get("SigninToken") != "signin-123" || q.Get("Destination") != tc.dest {
				t.Fatalf("login query = %v", q)
			}
		})
	}
}

func TestConsoleURLErrors(t *testing.T) {
	t.Run("long-lived credentials never reach the network", func(t *testing.T) {
		client, rt := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			t.Error("unexpected request")
		})
		_, err := ConsoleURL(context.Background(), client, core.Credentials{AccessKeyID: "AKIA", SecretAccessKey: "s"}, "")
		if err == nil || !strings.Contains(err.Error(), "temporary credentials") || rt.calls != 0 {
			t.Fatalf("err = %v, calls = %d", err, rt.calls)
		}
	})
	t.Run("non-200 reports status and body", func(t *testing.T) {
		client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "access denied", http.StatusForbidden)
		})
		_, err := ConsoleURL(context.Background(), client, tempCreds, "")
		if err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "access denied") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("malformed body", func(t *testing.T) {
		client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "<html>nope</html>")
		})
		if _, err := ConsoleURL(context.Background(), client, tempCreds, ""); err == nil {
			t.Fatal("expected a JSON error")
		}
	})
	t.Run("cancelled context", func(t *testing.T) {
		client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"SigninToken":"x"}`)
		})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := ConsoleURL(ctx, client, tempCreds, ""); err == nil {
			t.Fatal("expected a context error")
		}
	})
}
