package core

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func ids(sessions []Session) []string {
	out := make([]string, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, s.ID)
	}
	return out
}

func TestSessionLookupReturnsPointerIntoWorkspace(t *testing.T) {
	w := Workspace{Sessions: []Session{{ID: "s1", Name: "one"}, {ID: "s2", Name: "two"}}}
	s, err := w.Session("s2")
	if err != nil || s.Name != "two" {
		t.Fatalf("Session = %+v, %v", s, err)
	}
	s.Name = "renamed"
	if w.Sessions[1].Name != "renamed" {
		t.Fatal("returned session is a copy, not a pointer into the workspace")
	}
	if s, err := w.Session("missing"); !errors.Is(err, ErrNotFound) || s != nil {
		t.Fatalf("missing session = %+v, %v", s, err)
	}
	if _, err := (&Workspace{}).Session("s1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty workspace = %v", err)
	}
}

func TestIntegrationLookupReturnsPointerIntoWorkspace(t *testing.T) {
	w := Workspace{Integrations: []Integration{{ID: "i1"}, {ID: "i2", Alias: "acme"}}}
	i, err := w.Integration("i2")
	if err != nil || i.Alias != "acme" {
		t.Fatalf("Integration = %+v, %v", i, err)
	}
	i.Alias = "renamed"
	if w.Integrations[1].Alias != "renamed" {
		t.Fatal("returned integration is a copy, not a pointer into the workspace")
	}
	if i, err := w.Integration("missing"); !errors.Is(err, ErrNotFound) || i != nil {
		t.Fatalf("missing integration = %+v, %v", i, err)
	}
}

func TestRemoveSession(t *testing.T) {
	cases := []struct {
		name    string
		have    []string
		remove  string
		want    []string
		wantErr error
	}{
		{"middle keeps order", []string{"a", "b", "c"}, "b", []string{"a", "c"}, nil},
		{"first", []string{"a", "b", "c"}, "a", []string{"b", "c"}, nil},
		{"last", []string{"a", "b", "c"}, "c", []string{"a", "b"}, nil},
		{"only", []string{"a"}, "a", []string{}, nil},
		{"missing leaves workspace unchanged", []string{"a", "b"}, "z", []string{"a", "b"}, ErrNotFound},
		{"empty workspace", nil, "a", nil, ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var w Workspace
			for _, id := range tc.have {
				w.Sessions = append(w.Sessions, Session{ID: id})
			}
			err := w.RemoveSession(tc.remove)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got := ids(w.Sessions); len(got) != len(tc.want) || (len(got) > 0 && !reflect.DeepEqual(got, tc.want)) {
				t.Fatalf("sessions = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRemoveSourceSessionKeepsDependents(t *testing.T) {
	// Removing a source session is not cascaded: the chained session stays and
	// keeps its now-dangling SourceSessionID for the caller to resolve.
	w := Workspace{Sessions: []Session{
		{ID: "src", Kind: KindAWSIAMUser},
		{ID: "chain", Kind: KindAWSAssumeRole, AWS: &AWSSession{SourceSessionID: "src"}},
	}}
	if err := w.RemoveSession("src"); err != nil {
		t.Fatal(err)
	}
	if len(w.Sessions) != 1 || w.Sessions[0].ID != "chain" || w.Sessions[0].AWS.SourceSessionID != "src" {
		t.Fatalf("sessions = %+v", w.Sessions)
	}
}

func TestRemoveIntegrationKeepsOrderOfRemaining(t *testing.T) {
	w := Workspace{
		Integrations: []Integration{{ID: "i1"}, {ID: "i2"}, {ID: "i3"}},
		Sessions: []Session{
			{ID: "a", IntegrationID: "i1"},
			{ID: "b", IntegrationID: "i2"},
			{ID: "c", IntegrationID: "i1"},
			{ID: "d"},
		},
	}
	if err := w.RemoveIntegration("i1"); err != nil {
		t.Fatal(err)
	}
	if got := ids(w.Sessions); !reflect.DeepEqual(got, []string{"b", "d"}) {
		t.Fatalf("sessions = %v", got)
	}
	if len(w.Integrations) != 2 || w.Integrations[0].ID != "i2" || w.Integrations[1].ID != "i3" {
		t.Fatalf("integrations = %+v", w.Integrations)
	}
}

func TestDefaultSettings(t *testing.T) {
	want := Settings{Theme: "system", DefaultRegion: "us-east-1", AssumeRoleMinutes: 60, HideOnClose: true}
	if got := DefaultSettings(); !reflect.DeepEqual(got, want) {
		t.Fatalf("DefaultSettings = %+v, want %+v", got, want)
	}
}

func TestSettingsNormalizeTable(t *testing.T) {
	cases := []struct {
		name string
		in   Settings
		want Settings
	}{
		{"zero value takes defaults except booleans", Settings{}, Settings{Theme: "system", DefaultRegion: "us-east-1", AssumeRoleMinutes: 60}},
		{"light theme kept", Settings{Theme: "light"}, Settings{Theme: "light", DefaultRegion: "us-east-1", AssumeRoleMinutes: 60}},
		{"dark theme kept", Settings{Theme: "dark"}, Settings{Theme: "dark", DefaultRegion: "us-east-1", AssumeRoleMinutes: 60}},
		{"theme is case sensitive", Settings{Theme: "Dark"}, Settings{Theme: "system", DefaultRegion: "us-east-1", AssumeRoleMinutes: 60}},
		{"minimum duration kept", Settings{AssumeRoleMinutes: 15}, Settings{Theme: "system", DefaultRegion: "us-east-1", AssumeRoleMinutes: 15}},
		{"below minimum resets to default", Settings{AssumeRoleMinutes: 14}, Settings{Theme: "system", DefaultRegion: "us-east-1", AssumeRoleMinutes: 60}},
		{"maximum duration kept", Settings{AssumeRoleMinutes: 720}, Settings{Theme: "system", DefaultRegion: "us-east-1", AssumeRoleMinutes: 720}},
		{"above maximum clamps", Settings{AssumeRoleMinutes: 721}, Settings{Theme: "system", DefaultRegion: "us-east-1", AssumeRoleMinutes: 720}},
		{"region kept", Settings{DefaultRegion: "eu-west-1"}, Settings{Theme: "system", DefaultRegion: "eu-west-1", AssumeRoleMinutes: 60}},
		{"flags and terminal untouched", Settings{HideOnClose: true, VerboseLogging: true, AutoUpdateOff: true, Terminal: "iterm"},
			Settings{Theme: "system", DefaultRegion: "us-east-1", AssumeRoleMinutes: 60, HideOnClose: true, VerboseLogging: true, AutoUpdateOff: true, Terminal: "iterm"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.Normalize(); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Normalize(%+v) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestEffectiveSettings(t *testing.T) {
	if got := (&Workspace{}).EffectiveSettings(); !reflect.DeepEqual(got, DefaultSettings()) {
		t.Fatalf("nil settings = %+v", got)
	}
	stored := &Settings{Theme: "sepia", AssumeRoleMinutes: 1}
	w := Workspace{Settings: stored}
	got := w.EffectiveSettings()
	if got.Theme != "system" || got.AssumeRoleMinutes != 60 || got.DefaultRegion != "us-east-1" {
		t.Fatalf("effective = %+v", got)
	}
	if stored.Theme != "sepia" || stored.AssumeRoleMinutes != 1 {
		t.Fatal("EffectiveSettings must not mutate the stored settings")
	}
}

func TestCredentialsExpiredAtBoundary(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	atNow := now
	justAfter := now.Add(time.Nanosecond)
	in5 := now.Add(5 * time.Minute)
	cases := []struct {
		name string
		exp  *time.Time
		skew time.Duration
		want bool
	}{
		{"nil expiration", nil, time.Hour, false},
		{"expires now", &atNow, 0, true},
		{"expires one nanosecond later", &justAfter, 0, false},
		{"skew reaches expiry exactly", &in5, 5 * time.Minute, true},
		{"skew stops short of expiry", &in5, 5*time.Minute - time.Nanosecond, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := (Credentials{Expiration: tc.exp}).Expired(now, tc.skew); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Expired = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSessionJSONOmitsEmptyOptionalFields(t *testing.T) {
	data, err := json.Marshal(Session{ID: "s1", Name: "n", Kind: KindAzure, Status: StatusInactive})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"region", "integrationId", "favorite", "expires", "aws", "azure", "gcp"} {
		if _, ok := got[k]; ok {
			t.Errorf("%q should be omitted when empty: %s", k, data)
		}
	}
	for _, k := range []string{"id", "name", "kind", "status"} {
		if _, ok := got[k]; !ok {
			t.Errorf("%q missing: %s", k, data)
		}
	}
}
