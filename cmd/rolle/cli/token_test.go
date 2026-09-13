package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/kube"
)

func TestTokenAndKubeTokenPrintTheCachedBearerToken(t *testing.T) {
	s := testCLI(t)
	exp := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	w, _ := s.Load()
	w.Sessions = append(w.Sessions, core.Session{ID: "g1", Name: "proj", Kind: core.KindGCP, Status: core.StatusActive, Expires: &exp, GCP: &core.GCPSession{ProjectID: "proj"}})
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	if err := s.Cache.Put("g1", core.Credentials{Token: "ya29.tok", Expiration: &exp}); err != nil {
		t.Fatal(err)
	}
	// token prints the bare value, for a curl header or a mise template.
	if out := mustRun(t, "token", "proj"); out != "ya29.tok\n" {
		t.Fatalf("token output: %q", out)
	}
	// kube token wraps it the way a kubectl exec plugin must.
	var cred struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Status     struct {
			Token               string `json:"token"`
			ExpirationTimestamp string `json:"expirationTimestamp"`
		} `json:"status"`
	}
	if err := json.Unmarshal([]byte(mustRun(t, "kube", "token", "proj")), &cred); err != nil {
		t.Fatal(err)
	}
	if cred.APIVersion != kube.ExecAPIVersion || cred.Kind != "ExecCredential" || cred.Status.Token != "ya29.tok" || cred.Status.ExpirationTimestamp != exp.Format(time.RFC3339) {
		t.Fatalf("exec credential = %+v", cred)
	}
	if _, err := run(t, "kube", "list", "ghost"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("kube list unknown = %v", err)
	}
	if _, err := run(t, "kube", "add", "ghost", "--all"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("kube add unknown = %v", err)
	}
}

func TestPickClustersKeepsTheRequestedOrder(t *testing.T) {
	clusters := []kube.Cluster{
		{Name: "api", Location: "eu-west-1", Endpoint: "https://api.example", Cloud: core.CloudAWS},
		{Name: "batch", Location: "eu-west-1", Endpoint: "https://batch.example", Cloud: core.CloudAWS},
	}
	got, err := pickClusters(clusters, []string{"batch", "api"})
	if err != nil || len(got) != 2 || got[0].Name != "batch" || got[1].Name != "api" {
		t.Fatalf("picked = %+v, %v", got, err)
	}
	if _, err := pickClusters(clusters, []string{"api", "ghost"}); !errors.Is(err, core.ErrNotFound) || !strings.Contains(err.Error(), `cluster "ghost"`) {
		t.Fatalf("unknown cluster = %v", err)
	}
	out := clustersOut(clusters)
	if len(out) != 2 || out[0] != (clusterJSON{Name: "api", Location: "eu-west-1", Endpoint: "https://api.example", Cloud: core.CloudAWS}) {
		t.Fatalf("clusters json = %+v", out)
	}
	// No clusters is an empty list, not null, so scripts can range over it.
	if b, _ := json.Marshal(clustersOut(nil)); string(b) != "[]" {
		t.Fatalf("empty clusters json = %s", b)
	}
	table := capture(t, func() error { return printClusters(clusters) })
	rows := lines(table)
	if len(rows) != 3 || !strings.HasPrefix(rows[0], "NAME") || fields(rows[1])[0] != "api" || fields(rows[2])[2] != "https://batch.example" {
		t.Fatalf("table:\n%s", table)
	}
}

// capture returns what fn prints to stdout.
func capture(t *testing.T, fn func() error) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		done <- b.String()
	}()
	fnErr := fn()
	os.Stdout = old
	_ = w.Close()
	out := <-done
	_ = r.Close()
	if fnErr != nil {
		t.Fatal(fnErr)
	}
	return out
}

func TestExitCodeForInactiveAndAmbiguousSessions(t *testing.T) {
	// Neither a login nor a missing session: plain failure.
	if got := ExitCode(app.ErrSessionInactive); got != ExitError {
		t.Fatalf("ExitCode(inactive) = %d", got)
	}
	if got := ExitCode(app.ErrAmbiguous); got != ExitError {
		t.Fatalf("ExitCode(ambiguous) = %d", got)
	}
}
