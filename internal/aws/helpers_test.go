package aws

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// failStore returns err from every call.
type failStore struct{ err error }

func (f failStore) Get(string) (string, error) { return "", f.err }
func (f failStore) Set(string, string) error   { return f.err }
func (f failStore) Delete(string) error        { return f.err }

// rewriteTransport sends every request to a test server, whatever host the
// code under test asked for. It counts the requests it forwards.
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

// testClient starts a server for handler and returns a client routed to it.
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
