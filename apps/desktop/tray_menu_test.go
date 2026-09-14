package main

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nateships/rolle/internal/core"
)

// menuLabels lists a menu top to bottom. Separators show as "---" and a
// disabled item ends in " (off)".
func menuLabels(m *application.Menu) []string {
	var out []string
	for i := 0; ; i++ {
		it := m.ItemAt(i)
		if it == nil {
			return out
		}
		switch {
		case it.IsSeparator():
			out = append(out, "---")
		case !it.Enabled():
			out = append(out, it.Label()+" (off)")
		default:
			out = append(out, it.Label())
		}
	}
}

func submenu(t *testing.T, m *application.Menu, label string) *application.Menu {
	t.Helper()
	it := m.FindByLabel(label)
	if it == nil || !it.IsSubmenu() {
		t.Fatalf("no submenu %q in %v", label, menuLabels(m))
	}
	return it.GetSubmenu()
}

func TestNeedsInputOnlyForInactiveMFASessions(t *testing.T) {
	mfa := core.Session{Status: core.StatusInactive, AWS: &core.AWSSession{MFADevice: "arn:aws:iam::1:mfa/me"}}
	if !needsInput(mfa) {
		t.Fatal("inactive MFA session needs no input")
	}
	mfa.Status = core.StatusActive
	if needsInput(mfa) {
		t.Fatal("active MFA session needs input")
	}
	if needsInput(core.Session{AWS: &core.AWSSession{}}) || needsInput(core.Session{Azure: &core.AzureSession{}}) {
		t.Fatal("session without an MFA device needs input")
	}
}

func TestUntilFormatsRemainingTime(t *testing.T) {
	now := time.Now()
	cases := map[string]time.Duration{
		"expired": -time.Second,
		"2h 05m":  2*time.Hour + 5*time.Minute + 30*time.Second,
		"12m":     12*time.Minute + 30*time.Second,
		"<1m":     20 * time.Second,
	}
	for want, d := range cases {
		if got := until(now.Add(d)); got != want {
			t.Errorf("until(+%v) = %q, want %q", d, got, want)
		}
	}
}

func TestCountLabelAndTooltipWithoutExpiry(t *testing.T) {
	if countLabel(0) != "no active sessions" || countLabel(3) != "3 active sessions" {
		t.Fatal(countLabel(0), countLabel(3))
	}
	if got := tooltip([]core.Session{{Name: "a"}}); got != "rolle · 1 active session" {
		t.Fatal(got)
	}
}

func TestAddStartItemDisablesMFASessions(t *testing.T) {
	tr := &tray{}
	m := application.NewMenu()
	tr.addStartItem(m, core.Session{ID: "plain", Name: "plain", AWS: &core.AWSSession{}}, "plain")
	tr.addStartItem(m, core.Session{ID: "mfa", Name: "mfa", AWS: &core.AWSSession{MFADevice: "arn"}}, "mfa")
	want := []string{"plain", "mfa · needs MFA in the app (off)"}
	if got := menuLabels(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
}

func TestProviderMenusGroupAWSRolesByAccount(t *testing.T) {
	sessions := []core.Session{
		{ID: "ro", Name: "Acme Prod/ReadOnlyAccess", Kind: core.KindAWSSSORole, AWS: &core.AWSSession{AccountID: "1", RoleName: "ReadOnlyAccess"}},
		{ID: "admin", Name: "Acme Prod/AdministratorAccess", Kind: core.KindAWSSSORole, AWS: &core.AWSSession{AccountID: "1", RoleName: "AdministratorAccess"}},
		{ID: "dev", Name: "Acme Dev/PowerUserAccess", Kind: core.KindAWSSSORole, AWS: &core.AWSSession{AccountID: "2", RoleName: "PowerUserAccess"}},
		{ID: "iam", Name: "personal", Kind: core.KindAWSIAMUser, AWS: &core.AWSSession{}},
		{ID: "chain", Name: "chained", Kind: core.KindAWSAssumeRole, AWS: &core.AWSSession{}},
		{ID: "az", Name: "Contoso", Kind: core.KindAzure, Azure: &core.AzureSession{}},
		{ID: "gcp2", Name: "zeta", Kind: core.KindGCP, GCP: &core.GCPSession{}},
		{ID: "gcp1", Name: "alpha", Kind: core.KindGCP, GCP: &core.GCPSession{}},
	}
	w := &core.Workspace{Sessions: sessions}
	m := application.NewMenu()
	(&tray{}).addProviderMenus(m, w, sessions)

	if got, want := menuLabels(m), []string{"AWS · 5", "Azure · 1", "Google Cloud · 2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("top level = %v, want %v", got, want)
	}
	aws := submenu(t, m, "AWS · 5")
	if got, want := menuLabels(aws), []string{"Acme Dev", "Acme Prod", "---", "chained", "personal"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("aws = %v, want %v", got, want)
	}
	prod := submenu(t, aws, "Acme Prod")
	if got, want := menuLabels(prod), []string{"AdministratorAccess", "ReadOnlyAccess"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("account = %v, want %v", got, want)
	}
	if got, want := menuLabels(submenu(t, m, "Google Cloud · 2")), []string{"alpha", "zeta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("gcp = %v, want %v", got, want)
	}
	// Only Identity Center roles group under accounts; no separator without them.
	m = application.NewMenu()
	(&tray{}).addProviderMenus(m, w, sessions[3:5])
	if got, want := menuLabels(submenu(t, m, "AWS · 2")), []string{"chained", "personal"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("standalone only = %v, want %v", got, want)
	}
}

func TestPortalWarningsNameTheExpiringSignIn(t *testing.T) {
	now := time.Now()
	soon := now.Add(12 * time.Minute)
	later := now.Add(3 * time.Hour)
	w := &core.Workspace{
		Integrations: []core.Integration{
			{ID: "acme", Alias: "acme", AWSSSO: &core.AWSSSOIntegration{TokenExpires: &soon}},
			{ID: "globex", Alias: "globex", AWSSSO: &core.AWSSSOIntegration{TokenExpires: &later}},
			{ID: "idle", Alias: "idle", AWSSSO: &core.AWSSSOIntegration{TokenExpires: &soon}},
		},
		Sessions: []core.Session{
			{ID: "a", IntegrationID: "acme", Status: core.StatusActive},
			{ID: "g", IntegrationID: "globex", Status: core.StatusActive},
			{ID: "i", IntegrationID: "idle"},
		},
	}
	m := application.NewMenu()
	(&tray{}).addPortalWarnings(m, w, now)
	// Only the portal that is due and still has an active session.
	got := menuLabels(m)
	if len(got) != 1 || !strings.HasPrefix(got[0], "acme sign-in expires in 1") || !strings.HasSuffix(got[0], " · Sign in again") {
		t.Fatalf("warnings = %v", got)
	}
}

func TestFavoritesListActiveAndInactive(t *testing.T) {
	exp := time.Now().Add(30 * time.Minute)
	active := []core.Session{
		{ID: "admin", Name: "Acme Prod/AdministratorAccess", Kind: core.KindAWSSSORole, Status: core.StatusActive, Expires: &exp, Favorite: true, AWS: &core.AWSSession{Profile: "prod"}},
		{ID: "gcp", Name: "alpha", Kind: core.KindGCP, Status: core.StatusActive, GCP: &core.GCPSession{}},
	}
	inactive := []core.Session{
		{ID: "ro", Name: "AAA first by name", Kind: core.KindAWSSSORole, Favorite: true, AWS: &core.AWSSession{}},
		{ID: "az", Name: "Contoso", Kind: core.KindAzure, Azure: &core.AzureSession{}},
	}
	m := application.NewMenu()
	(&tray{}).addFavorites(m, active, inactive)

	// Active favorites lead, then the rest by name.
	got := menuLabels(m)
	if len(got) != 4 || got[0] != "Favorites (off)" || !strings.HasPrefix(got[1], "Acme Prod/AdministratorAccess · ") || got[2] != "AAA first by name" || got[3] != "---" {
		t.Fatalf("favorites = %v", got)
	}
	// The active favorite keeps its actions; the inactive one is a start item.
	if !m.ItemAt(1).IsSubmenu() || m.ItemAt(2).IsSubmenu() {
		t.Fatalf("active favorite has no submenu or inactive one has: %v", got)
	}

	// No favorite, no section.
	m = application.NewMenu()
	(&tray{}).addFavorites(m, active[1:], inactive[1:])
	if got := menuLabels(m); len(got) != 0 {
		t.Fatalf("menu without favorites = %v", got)
	}
}

func TestTagMenusListTaggedSessionsInSidebarOrder(t *testing.T) {
	exp := time.Now().Add(30 * time.Minute)
	active := []core.Session{
		{ID: "admin", Name: "Acme Prod/AdministratorAccess", Kind: core.KindAWSSSORole, Status: core.StatusActive, Expires: &exp, Tags: []string{"Production"}, AWS: &core.AWSSession{Profile: "prod"}},
	}
	inactive := []core.Session{
		{ID: "ro", Name: "Acme Prod/ReadOnlyAccess", Kind: core.KindAWSSSORole, Tags: []string{"Production", "Audit"}, AWS: &core.AWSSession{}},
		{ID: "gcp", Name: "alpha", Kind: core.KindGCP, GCP: &core.GCPSession{}},
	}
	w := &core.Workspace{Tags: []core.Tag{{Name: "Production"}, {Name: "Sandbox"}, {Name: "Audit"}}}
	m := application.NewMenu()
	(&tray{}).addTagMenus(m, w, active, inactive)

	// Sidebar order, no empty tag, one separator after the block.
	if got, want := menuLabels(m), []string{"Tags (off)", "Production · 2", "Audit · 1", "---"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("top level = %v, want %v", got, want)
	}
	prod := submenu(t, m, "Production · 2")
	got := menuLabels(prod)
	if len(got) != 2 || !strings.HasPrefix(got[0], "Acme Prod/AdministratorAccess · ") || got[1] != "Acme Prod/ReadOnlyAccess" {
		t.Fatalf("production = %v", got)
	}
	// The active session keeps its actions; the inactive one is a start item.
	if !prod.ItemAt(0).IsSubmenu() || prod.ItemAt(1).IsSubmenu() {
		t.Fatalf("active session has no submenu or inactive one has: %v", got)
	}

	// No tagged session, no block at all.
	m = application.NewMenu()
	(&tray{}).addTagMenus(m, w, nil, inactive[1:])
	if got := menuLabels(m); len(got) != 0 {
		t.Fatalf("menu without tagged sessions = %v", got)
	}
}

func TestSessionMenuOffersProfileCommandForAWSOnly(t *testing.T) {
	tr := &tray{}
	m := application.NewMenu()
	tr.addSessionMenu(m, core.Session{ID: "a", Name: "a", Kind: core.KindAWSSSORole, AWS: &core.AWSSession{}})
	want := []string{"Stop", "---", "Open console", "Open terminal", "Copy credentials as env", "Copy profile command"}
	if got := menuLabels(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("aws = %v, want %v", got, want)
	}
	m = application.NewMenu()
	tr.addSessionMenu(m, core.Session{ID: "b", Name: "b", Kind: core.KindAzure, Azure: &core.AzureSession{}})
	if got := menuLabels(m); !reflect.DeepEqual(got, want[:5]) {
		t.Fatalf("azure = %v, want %v", got, want[:5])
	}
}

func TestFooterKeepsOpenAndQuitReachable(t *testing.T) {
	m := application.NewMenu()
	(&tray{}).addFooter(m)
	want := []string{"---", "Open rolle", "Report a problem…", "Settings…", "Quit rolle"}
	if got := menuLabels(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("footer = %v, want %v", got, want)
	}
}

func TestSplitForMenuKeepsHiddenSessionsOutUnlessActive(t *testing.T) {
	sessions := []core.Session{
		{ID: "a", Name: "shown", Status: core.StatusInactive},
		{ID: "b", Name: "hidden", Status: core.StatusInactive, Hidden: true},
		{ID: "c", Name: "hidden but running", Status: core.StatusActive, Hidden: true},
	}
	active, inactive := splitForMenu(sessions)
	if len(active) != 1 || active[0].ID != "c" {
		t.Fatalf("active = %+v", active)
	}
	if len(inactive) != 1 || inactive[0].ID != "a" {
		t.Fatalf("inactive = %+v", inactive)
	}
}
