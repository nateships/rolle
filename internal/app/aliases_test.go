package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/nateships/rolle/internal/core"
)

func seedAliasWorkspace(t *testing.T) *Service {
	t.Helper()
	s := testService(t)
	in, err := s.AddAWSSSO("acme", portalURL, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	// One session predates the account name field; Load fills it from the name.
	for _, r := range []struct{ acct, name, role string }{
		{"111111111111", "acme-prod", "AWSAdministratorAccess"},
		{"111111111111", "acme-prod", "ReadOnly"},
		{"222222222222", "acme-dev", "AWSAdministratorAccess"},
	} {
		addSession(t, s, core.Session{ID: r.acct + "/" + r.role, Name: r.name + "/" + r.role, Kind: core.KindAWSSSORole, IntegrationID: in.ID, Status: core.StatusInactive,
			AWS: &core.AWSSession{AccountID: r.acct, RoleName: r.role}})
	}
	return s
}

func names(t *testing.T, s *Service) map[string]string {
	t.Helper()
	w, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, sess := range w.Sessions {
		out[sess.ID] = sess.Name
	}
	return out
}

func TestAliasesRenameBuiltNamesAndStick(t *testing.T) {
	s := seedAliasWorkspace(t)
	w, _ := s.Load()
	if got, _ := FindSession(w, "acme-prod/ReadOnly"); got.AWS.AccountName != "acme-prod" {
		t.Fatalf("account name not filled: %+v", got.AWS)
	}

	// An account alias renames every role of that account. The ID, the name, or the alias names the account.
	if err := s.SetAlias(AliasAccount, "111111111111", "Prod"); err != nil {
		t.Fatal(err)
	}
	got := names(t, s)
	if got["111111111111/AWSAdministratorAccess"] != "Prod/AWSAdministratorAccess" || got["111111111111/ReadOnly"] != "Prod/ReadOnly" || got["222222222222/AWSAdministratorAccess"] != "acme-dev/AWSAdministratorAccess" {
		t.Fatalf("after account alias: %v", got)
	}
	// A role alias applies in every account.
	if err := s.SetAlias(AliasRole, "AWSAdministratorAccess", "Admin"); err != nil {
		t.Fatal(err)
	}
	got = names(t, s)
	if got["111111111111/AWSAdministratorAccess"] != "Prod/Admin" || got["222222222222/AWSAdministratorAccess"] != "acme-dev/Admin" {
		t.Fatalf("after role alias: %v", got)
	}
	// A hand-renamed session keeps its name through alias changes.
	if err := s.RenameSession("Prod/ReadOnly", "read prod"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(AliasAccount, "prod", "Production"); err != nil {
		t.Fatal(err)
	}
	got = names(t, s)
	if got["111111111111/ReadOnly"] != "read prod" || got["111111111111/AWSAdministratorAccess"] != "Production/Admin" {
		t.Fatalf("after second account alias: %v", got)
	}
	// Sync builds new names with the aliases. Clearing restores the original names.
	w, _ = s.Load()
	if ssoName(w, &core.AWSSession{AccountID: "111111111111", RoleName: "AWSAdministratorAccess", AccountName: "acme-prod"}) != "Production/Admin" {
		t.Fatal("sync name does not use the aliases")
	}
	if err := s.SetAlias(AliasAccount, "111111111111", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(AliasRole, "Admin", ""); err != nil {
		t.Fatal(err)
	}
	got = names(t, s)
	if got["111111111111/AWSAdministratorAccess"] != "acme-prod/AWSAdministratorAccess" || got["222222222222/AWSAdministratorAccess"] != "acme-dev/AWSAdministratorAccess" {
		t.Fatalf("after clearing: %v", got)
	}
}

func TestAliasesCannotClash(t *testing.T) {
	s := seedAliasWorkspace(t)
	// The name of another account is refused: it would make two sessions share a name.
	if err := s.SetAlias(AliasAccount, "222222222222", "acme-prod"); err == nil || !strings.Contains(err.Error(), "taken") {
		t.Fatalf("colliding names: %v", err)
	}
	if err := s.SetAlias(AliasAccount, "111111111111", "prod"); err != nil {
		t.Fatal(err)
	}
	// The same alias on another account is refused, in any case.
	if err := s.SetAlias(AliasAccount, "222222222222", "Prod"); err == nil || !strings.Contains(err.Error(), "taken") {
		t.Fatalf("duplicate account alias: %v", err)
	}
	if err := s.SetAlias(AliasAccount, "222222222222", "a/b"); err == nil {
		t.Fatal("slash accepted")
	}
	if err := s.SetAlias(AliasAccount, "333333333333", "x"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("unknown account: %v", err)
	}
	if err := s.SetAlias(AliasKind("team"), "x", "y"); err == nil {
		t.Fatal("unknown kind accepted")
	}
	// A name another account or permission set shows is refused too, so the
	// raw name still names one key.
	if err := s.SetAlias(AliasAccount, "222222222222", "111111111111"); err == nil || !strings.Contains(err.Error(), "taken") {
		t.Fatalf("alias equal to another account ID: %v", err)
	}
	if err := s.SetAlias(AliasRole, "ReadOnly", "awsadministratoraccess"); err == nil || !strings.Contains(err.Error(), "taken") {
		t.Fatalf("alias equal to another permission set: %v", err)
	}
	if _, err := resolveAliasKey(mustLoad(t, s), AliasRole, "AWSAdministratorAccess"); err != nil {
		t.Fatalf("raw permission set name: %v", err)
	}
}

func TestAliasRefusedWhenItRebuildsAHandPickedName(t *testing.T) {
	s := seedAliasWorkspace(t)
	// The user gave a dev role the name an account alias would build for
	// the prod role. The alias is refused rather than making two sessions
	// share a name.
	if err := s.RenameSession("acme-dev/AWSAdministratorAccess", "Prod/AWSAdministratorAccess"); err != nil {
		t.Fatal(err)
	}
	err := s.SetAlias(AliasAccount, "111111111111", "Prod")
	if err == nil || !strings.Contains(err.Error(), "makes two sessions named") {
		t.Fatalf("clashing alias: %v", err)
	}
	if got := names(t, s); got["111111111111/AWSAdministratorAccess"] != "acme-prod/AWSAdministratorAccess" {
		t.Fatalf("refused alias changed a name: %v", got)
	}
}

func TestResolveAliasKeyRefusesToGuessBetweenAccounts(t *testing.T) {
	s := seedAliasWorkspace(t)
	w := mustLoad(t, s)
	// Two accounts with the same Identity Center name: the name alone
	// cannot pick one, the ID can.
	addSession(t, s, core.Session{ID: "333333333333/ReadOnly", Name: "acme-prod/ReadOnly (2)", Kind: core.KindAWSSSORole, IntegrationID: w.Integrations[0].ID, Status: core.StatusInactive,
		AWS: &core.AWSSession{AccountID: "333333333333", AccountName: "acme-prod", RoleName: "ReadOnly"}})
	w = mustLoad(t, s)
	if _, err := resolveAliasKey(w, AliasAccount, "acme-prod"); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("ambiguous account name: %v", err)
	}
	if key, err := resolveAliasKey(w, AliasAccount, " 333333333333 "); err != nil || key != "333333333333" {
		t.Fatalf("account by ID = %q, %v", key, err)
	}
	if err := s.SetAlias(AliasAccount, "acme-prod", "Prod"); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("SetAlias on an ambiguous name: %v", err)
	}
}

func mustLoad(t *testing.T, s *Service) *core.Workspace {
	t.Helper()
	w, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	return w
}
