package app

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/policy"
)

// withPolicy makes s read p as the organization's profile. The returned
// function replaces it.
func withPolicy(s *Service, p policy.Policy) func(policy.Policy) {
	s.Policy = func() policy.Policy { return p }
	return func(next policy.Policy) { p = next }
}

func managed(settings map[string]string, locked ...string) policy.Policy {
	p := policy.Policy{Settings: map[string]json.RawMessage{}, Locked: locked}
	for k, v := range settings {
		p.Settings[k] = json.RawMessage(v)
	}
	return p
}

func TestPolicyDefaultAppliesOnceAndUserCanChangeIt(t *testing.T) {
	s := testService(t)
	withPolicy(s, managed(map[string]string{"defaultRegion": `"eu-west-1"`, "assumeRoleMinutes": `240`}))
	st, err := s.Settings()
	if err != nil {
		t.Fatal(err)
	}
	if st.DefaultRegion != "eu-west-1" || st.AssumeRoleMinutes != 240 {
		t.Fatalf("managed defaults not applied: %+v", st)
	}
	// The first Load writes the defaults and the record of what it applied.
	data, err := os.ReadFile(s.WorkspacePath)
	if err != nil {
		t.Fatal(err)
	}
	var w core.Workspace
	if err := json.Unmarshal(data, &w); err != nil {
		t.Fatal(err)
	}
	if w.Settings == nil || w.Settings.DefaultRegion != "eu-west-1" || w.Managed == nil || string(w.Managed.Settings["defaultRegion"]) != `"eu-west-1"` {
		t.Fatalf("workspace on disk = %s", data)
	}

	st.DefaultRegion = "us-west-2"
	if _, err := s.UpdateSettings(st); err != nil {
		t.Fatal(err)
	}
	if st, _ = s.Settings(); st.DefaultRegion != "us-west-2" || st.AssumeRoleMinutes != 240 {
		t.Fatalf("user change lost: %+v", st)
	}
}

func TestPolicyNewValueOverridesUserChange(t *testing.T) {
	s := testService(t)
	set := withPolicy(s, managed(map[string]string{"proxyUrl": `"http://old:3128"`}))
	if _, err := s.UpdateSettings(core.Settings{ProxyURL: "http://mine:3128"}); err != nil {
		t.Fatal(err)
	}
	set(managed(map[string]string{"proxyUrl": `"http://new:3128"`}))
	if st, _ := s.Settings(); st.ProxyURL != "http://new:3128" {
		t.Fatalf("proxy = %q, want the new managed value", st.ProxyURL)
	}
}

func TestPolicyKeyRemovedThenAddedAppliesAgain(t *testing.T) {
	s := testService(t)
	set := withPolicy(s, managed(map[string]string{"terminal": `"iterm"`}))
	if _, err := s.Settings(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateSettings(core.Settings{Terminal: "warp"}); err != nil {
		t.Fatal(err)
	}
	set(policy.Policy{})
	if st, _ := s.Settings(); st.Terminal != "warp" {
		t.Fatalf("terminal = %q after the key left the profile", st.Terminal)
	}
	set(managed(map[string]string{"terminal": `"iterm"`}))
	if st, _ := s.Settings(); st.Terminal != "iterm" {
		t.Fatalf("terminal = %q after the key came back", st.Terminal)
	}
}

func TestPolicyLockedValueWins(t *testing.T) {
	s := testService(t)
	withPolicy(s, managed(map[string]string{"updateChannel": `""`, "terminal": `"iterm"`}, "updateChannel"))
	got, err := s.UpdateSettings(core.Settings{UpdateChannel: "beta", Terminal: "warp"})
	if err != nil {
		t.Fatal(err)
	}
	if got.UpdateChannel != "" || got.Terminal != "warp" {
		t.Fatalf("UpdateSettings = %+v", got)
	}
	if !slices.Equal(s.LockedSettings(), []string{"updateChannel"}) {
		t.Fatalf("LockedSettings = %v", s.LockedSettings())
	}
}

func TestPolicyValuesAreNormalized(t *testing.T) {
	s := testService(t)
	withPolicy(s, managed(map[string]string{"assumeRoleMinutes": `5`, "hiddenSections": `["gcp","nope"]`}))
	st, err := s.Settings()
	if err != nil {
		t.Fatal(err)
	}
	if st.AssumeRoleMinutes != 60 || !slices.Equal(st.HiddenSections, []string{"gcp"}) {
		t.Fatalf("settings = %+v", st)
	}
}

func TestNoPolicyWritesNothing(t *testing.T) {
	s := testService(t)
	withPolicy(s, policy.Policy{})
	if _, err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.WorkspacePath); !os.IsNotExist(err) {
		t.Fatalf("workspace written without a policy: %v", err)
	}
}

// A locked value comes back at Load even when the workspace file holds
// another value and records that the profile value applied already.
func TestPolicyLockedValueRestoredAtLoad(t *testing.T) {
	s := testService(t)
	w := &core.Workspace{
		Settings: &core.Settings{UpdateChannel: "beta"},
		Managed:  &core.Managed{Settings: map[string]json.RawMessage{"updateChannel": json.RawMessage(`""`)}},
	}
	if err := s.Save(w); err != nil {
		t.Fatal(err)
	}
	withPolicy(s, managed(map[string]string{"updateChannel": `""`}, "updateChannel"))
	if st, _ := s.Settings(); st.UpdateChannel != "" {
		t.Fatalf("update channel = %q, want the locked value", st.UpdateChannel)
	}
}

func acmeSSO(region string) policy.Integration {
	return policy.Integration{Type: "aws-sso", Alias: "acme", StartURL: "https://acme.awsapps.com/start", Region: region}
}

func TestPolicyIntegrationAddedOnceWithStableID(t *testing.T) {
	s := testService(t)
	withPolicy(s, policy.Policy{Integrations: []policy.Integration{
		acmeSSO("us-east-1"),
		{Type: "azure", Alias: "contoso", TenantID: "t-1"},
	}})
	w1, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	w2, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(w2.Integrations) != 2 || w1.Integrations[0].ID != w2.Integrations[0].ID {
		t.Fatalf("integrations: first %+v, second %+v", w1.Integrations, w2.Integrations)
	}
	sso, az := w2.Integrations[0], w2.Integrations[1]
	if sso.Alias != "acme" || sso.Cloud != core.CloudAWS || sso.AWSSSO.StartURL != "https://acme.awsapps.com/start" || sso.AWSSSO.Region != "us-east-1" {
		t.Errorf("aws-sso = %+v %+v", sso, sso.AWSSSO)
	}
	if az.Alias != "contoso" || az.Cloud != core.CloudAzure || az.Azure.TenantID != "t-1" {
		t.Errorf("azure = %+v %+v", az, az.Azure)
	}
}

func TestPolicyIntegrationRemovedByUserStaysRemoved(t *testing.T) {
	s := testService(t)
	set := withPolicy(s, policy.Policy{Integrations: []policy.Integration{acmeSSO("us-east-1")}})
	if _, err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveIntegration("acme"); err != nil {
		t.Fatal(err)
	}
	if w, _ := s.Load(); len(w.Integrations) != 0 {
		t.Fatalf("removed integration came back: %+v", w.Integrations)
	}
	// A new value in the profile applies again.
	set(policy.Policy{Integrations: []policy.Integration{acmeSSO("eu-west-1")}})
	if w, _ := s.Load(); len(w.Integrations) != 1 || w.Integrations[0].AWSSSO.Region != "eu-west-1" {
		t.Fatalf("integrations = %+v", w.Integrations)
	}
}

func TestPolicyIntegrationNewRegionUpdatesExisting(t *testing.T) {
	s := testService(t)
	set := withPolicy(s, policy.Policy{Integrations: []policy.Integration{acmeSSO("us-east-1")}})
	w, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	id := w.Integrations[0].ID
	if err := s.RenameIntegration("acme", "work"); err != nil {
		t.Fatal(err)
	}
	set(policy.Policy{Integrations: []policy.Integration{acmeSSO("eu-west-1")}})
	w, _ = s.Load()
	if len(w.Integrations) != 1 || w.Integrations[0].ID != id || w.Integrations[0].AWSSSO.Region != "eu-west-1" || w.Integrations[0].Alias != "work" {
		t.Fatalf("integrations = %+v", w.Integrations)
	}
}

func TestPolicyIntegrationMatchesUsersOwnAndAvoidsAliasClash(t *testing.T) {
	s := testService(t)
	if _, err := s.AddAWSSSO("acme", "https://ACME.awsapps.com/start/", "us-east-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddAzure("contoso", "other-tenant"); err != nil {
		t.Fatal(err)
	}
	withPolicy(s, policy.Policy{Integrations: []policy.Integration{
		acmeSSO("us-east-1"),
		{Type: "azure", Alias: "contoso", TenantID: "t-1"},
	}})
	w, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	var aliases []string
	for _, in := range w.Integrations {
		aliases = append(aliases, in.Alias)
	}
	if !slices.Equal(aliases, []string{"acme", "contoso", "contoso-2"}) {
		t.Fatalf("aliases = %v", aliases)
	}
}

func TestFreeAliasKeepsTheBase(t *testing.T) {
	w := &core.Workspace{Integrations: []core.Integration{{ID: "a", Alias: "team-1"}, {ID: "b", Alias: "team-1-2"}}}
	if got := freeAlias(w, "team-1"); got != "team-1-3" {
		t.Fatalf("freeAlias = %q", got)
	}
	if got := freeAlias(w, "new"); got != "new" {
		t.Fatalf("freeAlias = %q", got)
	}
}
