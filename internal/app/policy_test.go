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
