package core

import (
	"testing"
	"time"
)

func TestKindCloud(t *testing.T) {
	cases := map[Kind]Cloud{
		KindAWSIAMUser:    CloudAWS,
		KindAWSAssumeRole: CloudAWS,
		KindAWSSSORole:    CloudAWS,
		KindAzure:         CloudAzure,
		KindGCP:           CloudGCP,
		Kind("bogus"):     "",
	}
	for k, want := range cases {
		if got := k.Cloud(); got != want {
			t.Errorf("%s.Cloud() = %q, want %q", k, got, want)
		}
	}
}

func TestCredentialsExpired(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	in10 := now.Add(10 * time.Minute)
	c := Credentials{Expiration: &in10}
	if c.Expired(now, time.Minute) {
		t.Fatal("fresh credentials reported expired")
	}
	if !c.Expired(now, 15*time.Minute) {
		t.Fatal("credentials inside skew window not reported expired")
	}
	if (Credentials{}).Expired(now, time.Hour) {
		t.Fatal("credentials without expiration reported expired")
	}
}

func TestRemoveIntegrationDropsSessions(t *testing.T) {
	w := Workspace{
		Integrations: []Integration{{ID: "i1"}, {ID: "i2"}},
		Sessions: []Session{
			{ID: "s1", IntegrationID: "i1"},
			{ID: "s2", IntegrationID: "i2"},
			{ID: "s3"},
		},
	}
	if err := w.RemoveIntegration("i1"); err != nil {
		t.Fatal(err)
	}
	if len(w.Integrations) != 1 || w.Integrations[0].ID != "i2" {
		t.Fatalf("integrations = %+v", w.Integrations)
	}
	if len(w.Sessions) != 2 {
		t.Fatalf("sessions = %+v", w.Sessions)
	}
	if err := w.RemoveIntegration("missing"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
