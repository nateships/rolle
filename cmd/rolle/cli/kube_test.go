package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/kube"
)

const testKubeconfig = `apiVersion: v1
kind: Config
current-context: kops
clusters:
- name: kops
  cluster:
    server: https://api.kops.example
contexts:
- name: kops
  context:
    cluster: kops
    user: kops
users:
- name: kops
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: aws-iam-authenticator
      args: [token, -i, kops]
`

func TestKubeTokenRefusesAWSSession(t *testing.T) {
	testCLI(t)
	mustRun(t, "session", "add", "iam-user", "--name", "dev", "--region", "us-east-1", "--access-key-id", "AKIA", "--secret-access-key", "secret")
	_, err := run(t, "kube", "token", "dev")
	if err == nil || !strings.Contains(err.Error(), "AWS CLI") || !strings.Contains(err.Error(), `profile "default"`) {
		t.Fatalf("kube token on an AWS session = %v", err)
	}
	if _, err := run(t, "kube", "token", "ghost"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("kube token unknown = %v", err)
	}
	if _, err := run(t, "kube", "token"); err == nil {
		t.Fatal("kube token without a session accepted")
	}
}

func TestKubeAttach(t *testing.T) {
	s := testCLI(t)
	mustRun(t, "session", "add", "iam-user", "--name", "dev", "--region", "us-east-1", "--access-key-id", "AKIA", "--secret-access-key", "secret", "--profile", "work")
	w, _ := s.Load()
	w.Sessions = append(w.Sessions, core.Session{ID: "az1", Name: "Contoso Production", Kind: core.KindAzure, Status: core.StatusInactive, Azure: &core.AzureSession{SubscriptionID: "sub", TenantID: "ten-1"}})
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(testKubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}

	out := mustRun(t, "kube", "attach", "dev", "kops", "--kubeconfig", path)
	if strings.TrimSpace(out) != "context kops in "+path+" now authenticates through dev" {
		t.Fatalf("attach output:\n%s", out)
	}
	cfg, _ := os.ReadFile(path)
	if !strings.Contains(string(cfg), "name: AWS_PROFILE\n") || !strings.Contains(string(cfg), "value: work\n") || !strings.Contains(string(cfg), "command: aws-iam-authenticator\n") || !strings.Contains(string(cfg), "current-context: kops\n") {
		t.Fatalf("kubeconfig after attach:\n%s", cfg)
	}

	mustRun(t, "kube", "attach", "Contoso Production", "kops", "--kubeconfig", path)
	cfg, _ = os.ReadFile(path)
	if !strings.Contains(string(cfg), "command: rolle\n") || strings.Contains(string(cfg), "aws-iam-authenticator") || !strings.Contains(string(cfg), "- Contoso Production\n") {
		t.Fatalf("kubeconfig after azure attach:\n%s", cfg)
	}

	if _, err := run(t, "kube", "attach", "dev", "ghost", "--kubeconfig", path); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("attach unknown context = %v", err)
	}
	if _, err := run(t, "kube", "attach", "ghost", "kops", "--kubeconfig", path); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("attach unknown session = %v", err)
	}
	if _, err := run(t, "kube", "attach", "dev"); err == nil {
		t.Fatal("attach without a context accepted")
	}
}

func TestKubeEntriesGiveEachEKSClusterItsOwnUser(t *testing.T) {
	aws := &core.Session{Name: "dev", Kind: core.KindAWSIAMUser, AWS: &core.AWSSession{Profile: "work"}}
	clusters := []kube.Cluster{{Name: "api", Region: "eu-west-1"}, {Name: "batch", Region: "eu-west-1"}}
	got := kubeEntries(aws, clusters, "")
	if len(got) != 2 || got[0].User == got[1].User || got[0].User != "rolle:work@api" || got[1].Exec.Args[5] != "batch" {
		t.Fatalf("aws entries = %+v", got)
	}
	az := &core.Session{Name: "Contoso Production", Kind: core.KindAzure}
	got = kubeEntries(az, clusters, "ctx")
	if len(got) != 2 || got[0].User != got[1].User || got[0].User != "rolle:Contoso Production" || got[0].Context != "ctx" {
		t.Fatalf("azure entries = %+v", got)
	}
}

func TestKubeListAndAddNeedAnActiveSession(t *testing.T) {
	testCLI(t)
	mustRun(t, "session", "add", "iam-user", "--name", "dev", "--region", "us-east-1", "--access-key-id", "AKIA", "--secret-access-key", "secret")
	if _, err := run(t, "kube", "list", "dev"); !errors.Is(err, app.ErrSessionInactive) {
		t.Fatalf("kube list on an inactive session = %v", err)
	}
	if _, err := run(t, "kube", "add", "dev", "--all"); !errors.Is(err, app.ErrSessionInactive) {
		t.Fatalf("kube add on an inactive session = %v", err)
	}
}
