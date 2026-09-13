package kube

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/nateships/rolle/internal/core"
)

const existing = `apiVersion: v1
kind: Config
preferences:
  colors: true
current-context: old
clusters:
- name: old
  cluster:
    server: https://old.example
    insecure-skip-tls-verify: true
contexts:
- name: old
  context:
    cluster: old
    user: old-user
    namespace: kube-system
users:
- name: old-user
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: aws-iam-authenticator
      args: [token, -i, old]
      env:
      - name: AWS_PROFILE
        value: stale
      - name: AWS_REGION
        value: us-east-1
`

func writeKubeconfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readKubeconfig(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func names(doc map[string]any, key string) []string {
	var out []string
	for _, it := range doc[key].([]any) {
		out = append(out, it.(map[string]any)["name"].(string))
	}
	return out
}

func TestMergeKeepsUnknownFieldsAndReplacesByName(t *testing.T) {
	path := writeKubeconfig(t, existing)
	entries := []Entry{
		{Context: "old", Cluster: "old", Server: "https://new.example", CA: []byte("PEM"), User: "rolle:prod", Exec: AWSExec("eu-west-1", "old", "prod")},
		{Context: "fresh", Cluster: "fresh", Server: "https://fresh.example", User: "rolle:Contoso Production", Exec: RolleExec("Contoso Production")},
	}
	if err := Merge(path, entries, ""); err != nil {
		t.Fatal(err)
	}
	doc := readKubeconfig(t, path)
	if doc["current-context"] != "old" || doc["preferences"].(map[string]any)["colors"] != true {
		t.Fatalf("top-level fields changed: %v", doc)
	}
	if got := names(doc, "clusters"); !reflect.DeepEqual(got, []string{"old", "fresh"}) {
		t.Fatalf("clusters = %v", got)
	}
	if got := names(doc, "contexts"); !reflect.DeepEqual(got, []string{"old", "fresh"}) {
		t.Fatalf("contexts = %v", got)
	}
	if got := names(doc, "users"); !reflect.DeepEqual(got, []string{"old-user", "rolle:prod", "rolle:Contoso Production"}) {
		t.Fatalf("users = %v", got)
	}
	old := find(doc, "clusters", "old")["cluster"].(map[string]any)
	if old["server"] != "https://new.example" || old["certificate-authority-data"] != base64.StdEncoding.EncodeToString([]byte("PEM")) || old["insecure-skip-tls-verify"] != nil {
		t.Fatalf("old cluster = %v", old)
	}
	ctx := find(doc, "contexts", "old")["context"].(map[string]any)
	if ctx["user"] != "rolle:prod" || ctx["cluster"] != "old" {
		t.Fatalf("old context = %v", ctx)
	}
	exec := find(doc, "users", "rolle:Contoso Production")["user"].(map[string]any)["exec"].(map[string]any)
	if exec["command"] != "rolle" || exec["apiVersion"] != ExecAPIVersion || exec["interactiveMode"] != "Never" || exec["env"] != nil {
		t.Fatalf("rolle exec = %v", exec)
	}
	if got := exec["args"].([]any); len(got) != 3 || got[2] != "Contoso Production" {
		t.Fatalf("rolle args = %v", got)
	}
	aws := find(doc, "users", "rolle:prod")["user"].(map[string]any)["exec"].(map[string]any)
	if env := aws["env"].([]any); len(env) != 1 || env[0].(map[string]any)["value"] != "prod" {
		t.Fatalf("aws env = %v", aws["env"])
	}

	// A second merge with --use sets the current context and adds nothing.
	if err := Merge(path, entries[1:], "fresh"); err != nil {
		t.Fatal(err)
	}
	doc = readKubeconfig(t, path)
	if doc["current-context"] != "fresh" || len(doc["users"].([]any)) != 3 {
		t.Fatalf("after second merge: %v", doc)
	}
}

func TestMergeCreatesMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config")
	if err := Merge(path, []Entry{{Context: "c", Cluster: "c", Server: "https://c", User: "u", Exec: RolleExec("s")}}, ""); err != nil {
		t.Fatal(err)
	}
	doc := readKubeconfig(t, path)
	if doc["apiVersion"] != "v1" || doc["kind"] != "Config" || doc["current-context"] != nil || len(doc["contexts"].([]any)) != 1 {
		t.Fatalf("new kubeconfig = %v", doc)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(path)
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v, %v", st.Mode(), err)
		}
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".rolle-*")); len(left) != 0 {
		t.Fatalf("temp files left: %v", left)
	}
}

func TestAttachEnvSetsAndReplacesProfile(t *testing.T) {
	path := writeKubeconfig(t, existing)
	if err := AttachEnv(path, "old", "AWS_PROFILE", "prod"); err != nil {
		t.Fatal(err)
	}
	exec := find(readKubeconfig(t, path), "users", "old-user")["user"].(map[string]any)["exec"].(map[string]any)
	env := exec["env"].([]any)
	if len(env) != 2 || env[0].(map[string]any)["value"] != "prod" || env[1].(map[string]any)["name"] != "AWS_REGION" {
		t.Fatalf("env = %v", env)
	}
	if exec["command"] != "aws-iam-authenticator" {
		t.Fatalf("exec changed: %v", exec)
	}

	// A user without env gets one.
	noEnv := strings.Replace(existing, "      env:\n      - name: AWS_PROFILE\n        value: stale\n      - name: AWS_REGION\n        value: us-east-1\n", "", 1)
	path = writeKubeconfig(t, noEnv)
	if err := AttachEnv(path, "old", "AWS_PROFILE", "prod"); err != nil {
		t.Fatal(err)
	}
	exec = find(readKubeconfig(t, path), "users", "old-user")["user"].(map[string]any)["exec"].(map[string]any)
	if env := exec["env"].([]any); len(env) != 1 || env[0].(map[string]any)["value"] != "prod" {
		t.Fatalf("env = %v", env)
	}

	// A user without an exec plugin is refused.
	noExec := strings.Replace(existing, "    exec:", "    token: abc\n    unused:", 1)
	path = writeKubeconfig(t, noExec)
	if err := AttachEnv(path, "old", "AWS_PROFILE", "prod"); err == nil || !strings.Contains(err.Error(), "no exec plugin") {
		t.Fatalf("attach without exec = %v", err)
	}
}

func TestAttachExecReplacesPlugin(t *testing.T) {
	path := writeKubeconfig(t, existing)
	if err := AttachExec(path, "old", RolleExec("my-project")); err != nil {
		t.Fatal(err)
	}
	doc := readKubeconfig(t, path)
	exec := find(doc, "users", "old-user")["user"].(map[string]any)["exec"].(map[string]any)
	if exec["command"] != "rolle" || exec["env"] != nil {
		t.Fatalf("exec = %v", exec)
	}
	if ctx := find(doc, "contexts", "old")["context"].(map[string]any); ctx["namespace"] != "kube-system" {
		t.Fatalf("context changed: %v", ctx)
	}
}

func TestAttachMissingContextIsNotFound(t *testing.T) {
	path := writeKubeconfig(t, existing)
	if err := AttachEnv(path, "ghost", "AWS_PROFILE", "p"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("attach env = %v", err)
	}
	if err := AttachExec(path, "ghost", RolleExec("s")); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("attach exec = %v", err)
	}
}

func TestExecBuilders(t *testing.T) {
	aws := AWSExec("eu-west-1", "prod-1", "acme")
	if aws.Command != "aws" || aws.APIVersion != "client.authentication.k8s.io/v1beta1" || !reflect.DeepEqual(aws.Env, [][2]string{{"AWS_PROFILE", "acme"}}) {
		t.Fatalf("aws exec = %+v", aws)
	}
	if got := strings.Join(aws.Args, " "); got != "--region eu-west-1 eks get-token --cluster-name prod-1 --output json" {
		t.Fatalf("aws args = %q", got)
	}
	r := RolleExec("Contoso Production")
	if r.Command != "rolle" || r.APIVersion != ExecAPIVersion || !reflect.DeepEqual(r.Args, []string{"kube", "token", "Contoso Production"}) || r.Env != nil {
		t.Fatalf("rolle exec = %+v", r)
	}
}

func TestDefaultPath(t *testing.T) {
	t.Setenv("KUBECONFIG", strings.Join([]string{"/a/one", "/b/two"}, string(os.PathListSeparator)))
	if got, _ := DefaultPath(); got != "/a/one" {
		t.Fatalf("path = %q", got)
	}
	t.Setenv("KUBECONFIG", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if got, _ := DefaultPath(); got != filepath.Join(home, ".kube", "config") {
		t.Fatalf("path = %q", got)
	}
}
