package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// rewriteTransport sends every request to a test server, whatever host the
// code under test asked for.
type rewriteTransport struct {
	target *url.URL
	calls  int
}

func (r *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.calls++
	req = req.Clone(req.Context())
	req.URL.Scheme = r.target.Scheme
	req.URL.Host = r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func testClient(t *testing.T, handler http.HandlerFunc) (*http.Client, *rewriteTransport) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	rt := &rewriteTransport{target: target}
	return &http.Client{Transport: rt}, rt
}

// fakeHome points every home-relative lookup at a fresh directory and clears
// the ADC override. It returns the directory and the default ADC path in it.
func fakeHome(t *testing.T) (home, adcPath string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", home)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	if runtime.GOOS == "windows" {
		return home, filepath.Join(home, "gcloud", "application_default_credentials.json")
	}
	return home, filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
}

func writeADC(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

const userADC = `{"type":"authorized_user","client_id":"c","client_secret":"s","refresh_token":"r","account":"me@example.com"}`

func TestADCPath(t *testing.T) {
	_, want := fakeHome(t)
	if got, err := ADCPath(); err != nil || got != want {
		t.Fatalf("ADCPath = %q, %v; want %q", got, err, want)
	}
	custom := filepath.Join(t.TempDir(), "adc.json")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", custom)
	if got, err := ADCPath(); err != nil || got != custom {
		t.Fatalf("ADCPath with override = %q, %v", got, err)
	}
}

func TestDetectAccount(t *testing.T) {
	cases := []struct {
		name    string
		data    string // empty means no file
		want    string
		wantErr error
		errText string
	}{
		{"missing file", "", "", ErrNoADC, ""},
		{"corrupt", "{", "", nil, "parse"},
		{"account field", userADC, "me@example.com", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, path := fakeHome(t)
			if tc.data != "" {
				writeADC(t, path, tc.data)
			}
			got, err := DetectAccount(context.Background())
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
			case tc.errText != "":
				if err == nil || !strings.Contains(err.Error(), tc.errText) {
					t.Fatalf("err = %v, want %q", err, tc.errText)
				}
			default:
				if err != nil || got.Email != tc.want {
					t.Fatalf("DetectAccount = %+v, %v", got, err)
				}
			}
		})
	}
}

func TestSourceTokenFailsBeforeNetwork(t *testing.T) {
	_, path := fakeHome(t)
	if _, err := SourceToken(context.Background()); !errors.Is(err, ErrNoADC) {
		t.Fatalf("missing ADC = %v", err)
	}
	writeADC(t, path, `{"type":"service_account","private_key":"x"}`)
	if _, err := SourceToken(context.Background()); err == nil || !strings.Contains(err.Error(), "authorized_user") {
		t.Fatalf("service account ADC = %v", err)
	}
	writeADC(t, path, `{"type":"authorized_user","client_id":"c","client_secret":"s"}`)
	if _, err := SourceToken(context.Background()); err == nil || !strings.Contains(err.Error(), "authorized_user") {
		t.Fatalf("ADC without refresh token = %v", err)
	}
}

func TestWriteImpersonatedADCDetails(t *testing.T) {
	_, src := fakeHome(t)
	out := filepath.Join(t.TempDir(), "gcp", "sess.json")
	if err := WriteImpersonatedADC(out, "sa@p.iam.gserviceaccount.com"); !errors.Is(err, ErrNoADC) {
		t.Fatalf("missing source = %v, want ErrNoADC", err)
	}
	if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("output written without a source")
	}

	writeADC(t, src, userADC)
	if err := WriteImpersonatedADC(out, "sa@p.iam.gserviceaccount.com"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Type      string         `json:"type"`
		Source    map[string]any `json:"source_credentials"`
		URL       string         `json:"service_account_impersonation_url"`
		Delegates []string       `json:"delegates"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	var wantSource map[string]any
	if err := json.Unmarshal([]byte(userADC), &wantSource); err != nil {
		t.Fatal(err)
	}
	if got.Type != "impersonated_service_account" || !reflect.DeepEqual(got.Source, wantSource) {
		t.Fatalf("adc = %+v", got)
	}
	if got.URL != "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/sa@p.iam.gserviceaccount.com:generateAccessToken" {
		t.Fatalf("url = %q", got.URL)
	}
	if got.Delegates == nil || len(got.Delegates) != 0 {
		t.Fatalf("delegates = %v, want an empty list", got.Delegates)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(out)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("perm = %o, want 600", perm)
		}
	}
}

func TestListProjectsPaginates(t *testing.T) {
	var auths []string
	client, rt := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		auths = append(auths, r.Header.Get("Authorization"))
		if r.URL.Path != "/v1/projects" || r.URL.Query().Get("filter") != "lifecycleState:ACTIVE" {
			t.Errorf("unexpected request %s", r.URL)
		}
		switch r.URL.Query().Get("pageToken") {
		case "":
			fmt.Fprint(w, `{"projects":[{"projectId":"a","name":"A"}],"nextPageToken":"p2"}`)
		case "p2":
			fmt.Fprint(w, `{"projects":[{"projectId":"b","name":"B"}]}`)
		default:
			t.Errorf("unexpected page token %q", r.URL.Query().Get("pageToken"))
		}
	})
	got, err := ListProjects(context.Background(), client, "tok")
	if err != nil {
		t.Fatal(err)
	}
	want := []Project{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}
	if !reflect.DeepEqual(got, want) || rt.calls != 2 {
		t.Fatalf("projects = %+v (%d calls)", got, rt.calls)
	}
	for _, a := range auths {
		if a != "Bearer tok" {
			t.Fatalf("authorization = %q", a)
		}
	}
}

func TestListProjectsErrors(t *testing.T) {
	client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "permission denied", http.StatusForbidden)
	})
	_, err := ListProjects(context.Background(), client, "tok")
	if err == nil || !strings.Contains(err.Error(), "list projects") || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("err = %v", err)
	}
	client, _ = testClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not json")
	})
	if _, err := ListProjects(context.Background(), client, "tok"); err == nil {
		t.Fatal("expected a JSON error")
	}
}

func TestImpersonate(t *testing.T) {
	cases := []struct {
		name     string
		lifetime time.Duration
		want     string
	}{
		{"default lifetime", 0, "3600s"},
		{"custom lifetime", 15 * time.Minute, "900s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotReq *http.Request
			var gotBody map[string]any
			client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				gotReq = r
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &gotBody)
				fmt.Fprint(w, `{"accessToken":"sa-token","expireTime":"2026-01-01T14:00:00+02:00"}`)
			})
			got, err := Impersonate(context.Background(), client, "src", "sa@p.iam.gserviceaccount.com", tc.lifetime)
			if err != nil {
				t.Fatal(err)
			}
			if gotReq.Method != http.MethodPost || gotReq.URL.Path != "/v1/projects/-/serviceAccounts/sa@p.iam.gserviceaccount.com:generateAccessToken" {
				t.Fatalf("request = %s %s", gotReq.Method, gotReq.URL)
			}
			if gotReq.Header.Get("Authorization") != "Bearer src" || gotReq.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("headers = %v", gotReq.Header)
			}
			if gotBody["lifetime"] != tc.want || !reflect.DeepEqual(gotBody["scope"], []any{"https://www.googleapis.com/auth/cloud-platform"}) {
				t.Fatalf("body = %v", gotBody)
			}
			wantExp := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
			if got.Token != "sa-token" || got.Expiration == nil || !got.Expiration.Equal(wantExp) || got.Expiration.Location() != time.UTC {
				t.Fatalf("creds = %+v", got)
			}
		})
	}
}

func TestImpersonateErrors(t *testing.T) {
	client, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "iam.serviceAccounts.getAccessToken denied", http.StatusForbidden)
	})
	_, err := Impersonate(context.Background(), client, "src", "sa@p.iam.gserviceaccount.com", 0)
	if err == nil || !strings.Contains(err.Error(), "impersonate sa@p.iam.gserviceaccount.com") || !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v", err)
	}
	client, _ = testClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not json")
	})
	if _, err := Impersonate(context.Background(), client, "src", "sa", 0); err == nil {
		t.Fatal("expected a JSON error")
	}
}
