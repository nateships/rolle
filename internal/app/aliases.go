package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nateships/rolle/internal/core"
)

// AliasKind says what an alias renames.
type AliasKind string

const (
	// AliasAccount renames an Identity Center account, by account ID.
	AliasAccount AliasKind = "account"
	// AliasRole renames a permission set, by its name, in every account.
	AliasRole AliasKind = "role"
)

// aliases returns the workspace's alias tables, made when absent.
func aliases(w *core.Workspace) *core.Aliases {
	if w.Aliases == nil {
		w.Aliases = &core.Aliases{}
	}
	if w.Aliases.Accounts == nil {
		w.Aliases.Accounts = map[string]string{}
	}
	if w.Aliases.Roles == nil {
		w.Aliases.Roles = map[string]string{}
	}
	return w.Aliases
}

// accountLabel is what a session's account is called: its alias, else the
// name Identity Center gave it, else the ID.
func accountLabel(w *core.Workspace, a *core.AWSSession) string {
	if w.Aliases != nil {
		if v := w.Aliases.Accounts[a.AccountID]; v != "" {
			return v
		}
	}
	if a.AccountName != "" {
		return a.AccountName
	}
	return a.AccountID
}

// roleLabel is what a permission set is called: its alias, else its name.
func roleLabel(w *core.Workspace, role string) string {
	if w.Aliases != nil {
		if v := w.Aliases.Roles[role]; v != "" {
			return v
		}
	}
	return role
}

// ssoName is the name an Identity Center role session gets from its account
// and permission set, with aliases applied.
func ssoName(w *core.Workspace, a *core.AWSSession) string {
	return accountLabel(w, a) + "/" + roleLabel(w, a.RoleName)
}

// fillAccountNames records the Identity Center account name on sessions
// from before the field existed. The name is the prefix of the session
// name, which sync built as account/role.
func fillAccountNames(w *core.Workspace) {
	for i := range w.Sessions {
		s := &w.Sessions[i]
		if s.Kind != core.KindAWSSSORole || s.AWS == nil || s.AWS.AccountName != "" {
			continue
		}
		if acct, _, ok := strings.Cut(s.Name, "/"); ok && acct != "" {
			s.AWS.AccountName = acct
		} else {
			s.AWS.AccountName = s.AWS.AccountID
		}
	}
}

// SetAlias names an account or a permission set. An empty alias clears it.
// SetAlias renames every role session that still carries its built name. A
// session renamed by hand keeps its name. The alias must be unique among its
// kind, must not equal the name another key shows, and no resulting session
// name may collide with another.
func (s *Service) SetAlias(kind AliasKind, key, alias string) error {
	alias = strings.Join(strings.Fields(alias), " ")
	if strings.Contains(alias, "/") {
		return errors.New("an alias cannot contain /")
	}
	w, err := s.Load()
	if err != nil {
		return err
	}
	table, err := aliasTable(w, kind)
	if err != nil {
		return err
	}
	key, err = resolveAliasKey(w, kind, key)
	if err != nil {
		return err
	}
	for k, v := range table {
		if k != key && alias != "" && strings.EqualFold(v, alias) {
			return fmt.Errorf("%s alias %q is taken by %s", kind, alias, k)
		}
	}
	// A name another key already shows would make the label name two keys.
	for _, sess := range w.Sessions {
		if sess.Kind != core.KindAWSSSORole || sess.AWS == nil || alias == "" {
			continue
		}
		k, labels := aliasLabels(w, kind, sess.AWS)
		for _, l := range labels {
			if k != key && l != "" && strings.EqualFold(l, alias) {
				return fmt.Errorf("%s alias %q is taken by %s", kind, alias, k)
			}
		}
	}
	// Names before the change tell which sessions still carry a built name.
	before := map[string]string{}
	for _, sess := range w.Sessions {
		if sess.Kind == core.KindAWSSSORole && sess.AWS != nil {
			before[sess.ID] = ssoName(w, sess.AWS)
		}
	}
	if alias == "" {
		delete(table, key)
	} else {
		table[key] = alias
	}
	for i := range w.Sessions {
		sess := &w.Sessions[i]
		if built, ok := before[sess.ID]; !ok || sess.Name != built {
			continue
		}
		sess.Name = ssoName(w, sess.AWS)
	}
	seen := map[string]string{}
	for _, sess := range w.Sessions {
		if other, dup := seen[sess.Name]; dup {
			return fmt.Errorf("alias %q makes two sessions named %q (%s and %s)", alias, sess.Name, other, sess.ID)
		}
		seen[sess.Name] = sess.ID
	}
	// A profile does not depend on the name, so nothing on disk changes.
	return s.Save(w)
}

func aliasTable(w *core.Workspace, kind AliasKind) (map[string]string, error) {
	a := aliases(w)
	switch kind {
	case AliasAccount:
		return a.Accounts, nil
	case AliasRole:
		return a.Roles, nil
	}
	return nil, fmt.Errorf("alias kind %q: use account or role", kind)
}

// resolveAliasKey accepts an account ID, or the name or alias an account or
// permission set shows, and returns the key the alias table uses.
func resolveAliasKey(w *core.Workspace, kind AliasKind, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	var keys []string
	seen := map[string]bool{}
	for _, sess := range w.Sessions {
		if sess.Kind != core.KindAWSSSORole || sess.AWS == nil {
			continue
		}
		key, labels := aliasLabels(w, kind, sess.AWS)
		if seen[key] {
			continue
		}
		for _, l := range labels {
			if l != "" && strings.EqualFold(l, ref) {
				seen[key] = true
				keys = append(keys, key)
				break
			}
		}
	}
	switch len(keys) {
	case 1:
		return keys[0], nil
	case 0:
		return "", fmt.Errorf("%s %q: %w", kind, ref, core.ErrNotFound)
	}
	return "", fmt.Errorf("%s %q: %w", kind, ref, ErrAmbiguous)
}

// aliasLabels returns the alias table key of a session for kind and every
// name the key shows: the key itself, the Identity Center name, and the
// alias.
func aliasLabels(w *core.Workspace, kind AliasKind, a *core.AWSSession) (string, []string) {
	if kind == AliasAccount {
		return a.AccountID, []string{a.AccountID, a.AccountName, accountLabel(w, a)}
	}
	return a.RoleName, []string{a.RoleName, roleLabel(w, a.RoleName)}
}
