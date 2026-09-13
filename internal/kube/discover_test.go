package kube

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"

	"github.com/nateships/rolle/internal/core"
)

// rewriteTransport sends every request to a test server, whatever host the
// code under test asked for.
type rewriteTransport struct{ target *url.URL }

func (r *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = r.target.Scheme
	req.URL.Host = r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func testClient(t *testing.T, handler http.HandlerFunc) *http.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Transport: &rewriteTransport{target: target}}
}

var caPEM = base64.StdEncoding.EncodeToString([]byte("-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----\n"))

func TestListGKE(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" || r.URL.Path != "/v1/projects/my-project/locations/-/clusters" {
			t.Errorf("unexpected request %s %v", r.URL, r.Header)
		}
		fmt.Fprintf(w, `{"clusters":[{"name":"web","location":"europe-west1","endpoint":"35.1.2.3","masterAuth":{"clusterCaCertificate":%q}}]}`, caPEM)
	})
	got, err := ListGKE(context.Background(), client, "tok", "my-project")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "web" || got[0].Location != "europe-west1" || got[0].Endpoint != "https://35.1.2.3" || got[0].Cloud != core.CloudGCP || !strings.HasPrefix(string(got[0].CA), "-----BEGIN CERTIFICATE-----") {
		t.Fatalf("clusters = %+v", got)
	}
	denied := testClient(t, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "nope", http.StatusForbidden) })
	if _, err := ListGKE(context.Background(), denied, "tok", "my-project"); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("denied = %v", err)
	}
}

func TestListAKS(t *testing.T) {
	const id = "/subscriptions/sub-1/resourceGroups/rg/providers/Microsoft.ContainerService/managedClusters/prod"
	kubeconfig := fmt.Sprintf("apiVersion: v1\nclusters:\n- name: prod\n  cluster:\n    server: https://prod.hcp.eastus.azmk8s.io:443\n    certificate-authority-data: %s\nusers:\n- name: u\n  user:\n    token: secret\n", caPEM)
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" || r.URL.Query().Get("api-version") != aksAPIVersion {
			t.Errorf("unexpected request %s %v", r.URL, r.Header)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/subscriptions/sub-1/providers/Microsoft.ContainerService/managedClusters" && r.URL.Query().Get("skiptoken") == "":
			fmt.Fprintf(w, `{"value":[{"id":%q,"name":"prod","location":"eastus"}],"nextLink":"https://management.azure.com/subscriptions/sub-1/providers/Microsoft.ContainerService/managedClusters?api-version=%s&skiptoken=n2"}`, id, aksAPIVersion)
		case r.Method == http.MethodGet && r.URL.Query().Get("skiptoken") == "n2":
			fmt.Fprint(w, `{"value":[]}`)
		case r.Method == http.MethodPost && r.URL.Path == id+"/listClusterUserCredential":
			fmt.Fprintf(w, `{"kubeconfigs":[{"name":"clusterUser","value":%q}]}`, base64.StdEncoding.EncodeToString([]byte(kubeconfig)))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	got, err := ListAKS(context.Background(), client, "tok", "sub-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "prod" || got[0].Location != "eastus" || got[0].Endpoint != "https://prod.hcp.eastus.azmk8s.io:443" || got[0].Cloud != core.CloudAzure || !strings.HasPrefix(string(got[0].CA), "-----BEGIN CERTIFICATE-----") {
		t.Fatalf("clusters = %+v", got)
	}
}

func TestListEKS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=AKIA/") {
			t.Errorf("request not signed with the session key: %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.URL.Path == "/clusters" && r.URL.Query().Get("nextToken") == "":
			fmt.Fprint(w, `{"clusters":["one"],"nextToken":"n2"}`)
		case r.URL.Path == "/clusters":
			fmt.Fprint(w, `{"clusters":["two"]}`)
		case strings.HasPrefix(r.URL.Path, "/clusters/"):
			name := strings.TrimPrefix(r.URL.Path, "/clusters/")
			fmt.Fprintf(w, `{"cluster":{"name":%q,"endpoint":"https://%s.eks.amazonaws.com","certificateAuthority":{"data":%q}}}`, name, name, caPEM)
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	creds := core.Credentials{AccessKeyID: "AKIA", SecretAccessKey: "secret", SessionToken: "tok"}
	got, err := ListEKS(context.Background(), creds, "eu-west-1", func(o *eks.Options) {
		o.BaseEndpoint = aws.String(srv.URL)
		o.RetryMaxAttempts = 1
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "one" || got[1].Name != "two" || got[1].Endpoint != "https://two.eks.amazonaws.com" || got[0].Region != "eu-west-1" || got[0].Location != "eu-west-1" || got[0].Cloud != core.CloudAWS || !strings.HasPrefix(string(got[0].CA), "-----BEGIN CERTIFICATE-----") {
		t.Fatalf("clusters = %+v", got)
	}
}
