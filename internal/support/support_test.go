package support

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/nateships/rolle/internal/core"
)

func TestRedact(t *testing.T) {
	in := "acct 123456789012 arn:aws:iam::210987654321:role/Admin nate@example.com https://acme.awsapps.com/start tenant 0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b key AKIAIOSFODNN7EXAMPLE token=abc.def secret: xyz"
	got := Redact(in)
	for _, leak := range []string{"123456789012", "210987654321", "nate@example.com", "acme.awsapps", "0f1e2d3c-4b5a", "AKIAIOSFODNN7EXAMPLE", "abc.def", "xyz"} {
		if strings.Contains(got, leak) {
			t.Errorf("Redact left %q in %q", leak, got)
		}
	}
	for _, keep := range []string{"role/Admin", "https://<org>.awsapps.com/start", "<account>", "<email>", "<guid>", "<access-key-id>"} {
		if !strings.Contains(got, keep) {
			t.Errorf("Redact lost %q in %q", keep, got)
		}
	}
}

func TestWriteBundle(t *testing.T) {
	ws := &core.Workspace{Sessions: []core.Session{{ID: "s1", Name: "Prod/Admin", AWS: &core.AWSSession{AccountID: "123456789012", RoleName: "Admin"}}}}
	var buf bytes.Buffer
	err := Write(&buf, Inputs{Version: "0.1.1", App: "cli", Workspace: ws, Log: []string{"[rolle:session] start Prod 123456789012"}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range r.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		files[f.Name] = string(b)
	}
	for _, name := range []string{"info.json", "workspace.json", "log.txt", "README.txt"} {
		if _, ok := files[name]; !ok {
			t.Errorf("bundle lacks %s", name)
		}
	}
	if strings.Contains(files["workspace.json"], "123456789012") || strings.Contains(files["log.txt"], "123456789012") {
		t.Fatal("account id leaked into the bundle")
	}
	if !strings.Contains(files["workspace.json"], `"roleName": "Admin"`) {
		t.Fatalf("workspace.json lost the role name: %s", files["workspace.json"])
	}
}
