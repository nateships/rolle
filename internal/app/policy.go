package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/policy"
)

// policy returns the organization's profile. It is empty when s has no
// reader.
func (s *Service) policy() policy.Policy {
	if s.Policy == nil {
		return policy.Policy{}
	}
	return s.Policy()
}

// LockedSettings lists the JSON names of the settings that the
// organization's profile locks. The user cannot change them.
func (s *Service) LockedSettings() []string {
	p := s.policy()
	var names []string
	for name := range p.Settings {
		if p.IsLocked(name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// applyPolicy puts the profile into w. A setting or an integration applies
// when the profile sets a value that rolle has not applied before. Thus the
// user can change or remove it, until the profile sets another value. A
// locked setting applies every time. w.Managed records what applied.
// applyPolicy reports whether w changed.
func applyPolicy(w *core.Workspace, p policy.Policy) bool {
	before, _ := json.Marshal([]any{w.Settings, w.Managed, w.Integrations})
	var record core.Managed
	if w.Managed != nil {
		record = *w.Managed
	}
	apply := map[string]json.RawMessage{}
	for name, v := range p.Settings {
		if p.IsLocked(name) || !sameJSON(record.Settings[name], v) {
			apply[name] = v
		}
	}
	if len(apply) > 0 {
		st := overlay(w.EffectiveSettings(), apply)
		w.Settings = &st
	}
	applied := applyIntegrations(w, p.Integrations, record.Integrations)
	// A key that left the profile leaves the record too, so the profile
	// can set it again later.
	w.Managed = nil
	if len(p.Settings) > 0 || len(applied) > 0 {
		w.Managed = &core.Managed{Settings: p.Settings, Integrations: applied}
	}
	after, _ := json.Marshal([]any{w.Settings, w.Managed, w.Integrations})
	return !sameJSON(before, after)
}

// applyIntegrations adds each profile integration that rolle has not
// applied in this form before. If the workspace has the portal or tenant
// already, from the user or from an earlier profile, it updates the region
// and keeps the rest. A new integration gets a free alias: the profile's,
// or that alias with -2, -3, and so on. The result is the new record.
func applyIntegrations(w *core.Workspace, list []policy.Integration, record map[string]json.RawMessage) map[string]json.RawMessage {
	var applied map[string]json.RawMessage
	for _, in := range list {
		key := in.Key()
		v, err := json.Marshal(in)
		if err != nil {
			continue
		}
		if applied == nil {
			applied = map[string]json.RawMessage{}
		}
		applied[key] = v
		if sameJSON(record[key], v) {
			continue
		}
		if i := slices.IndexFunc(w.Integrations, func(x core.Integration) bool { return policy.Key(x) == key }); i >= 0 {
			if sso := w.Integrations[i].AWSSSO; sso != nil {
				sso.Region = in.Region
			}
			continue
		}
		next := core.Integration{ID: newID(), Alias: freeAlias(w, in.Alias)}
		if in.Type == "azure" {
			next.Cloud, next.Azure = core.CloudAzure, &core.AzureIntegration{TenantID: in.TenantID}
		} else {
			next.Cloud, next.AWSSSO = core.CloudAWS, &core.AWSSSOIntegration{StartURL: in.StartURL, Region: in.Region}
		}
		w.Integrations = append(w.Integrations, next)
	}
	return applied
}

// freeAlias returns alias, or alias with the first suffix -2, -3, and so on
// that no integration uses.
func freeAlias(w *core.Workspace, base string) string {
	alias := base
	for n := 2; checkAlias(w, alias, "") != nil; n++ {
		alias = fmt.Sprintf("%s-%d", base, n)
	}
	return alias
}

// lockSettings puts the profile's locked values into st.
func lockSettings(st core.Settings, p policy.Policy) core.Settings {
	locked := map[string]json.RawMessage{}
	for name, v := range p.Settings {
		if p.IsLocked(name) {
			locked[name] = v
		}
	}
	if len(locked) == 0 {
		return st
	}
	return overlay(st, locked)
}

// overlay sets the fields of st named in vals and normalizes the result, so
// a profile cannot set a value that the settings screen refuses. The policy
// package checked the types already; on an error, st stays as it was.
func overlay(st core.Settings, vals map[string]json.RawMessage) core.Settings {
	data, err := json.Marshal(st)
	if err != nil {
		return st
	}
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &m); err != nil {
		return st
	}
	for k, v := range vals {
		m[k] = v
	}
	data, err = json.Marshal(m)
	if err != nil {
		return st
	}
	var out core.Settings
	if err := json.Unmarshal(data, &out); err != nil {
		return st
	}
	return out.Normalize()
}

// sameJSON compares two JSON values without regard to spacing. The
// workspace file is indented, so a value read back from it has other
// spacing than the profile's.
func sameJSON(a, b []byte) bool {
	var ca, cb bytes.Buffer
	if json.Compact(&ca, a) != nil || json.Compact(&cb, b) != nil {
		return bytes.Equal(a, b)
	}
	return bytes.Equal(ca.Bytes(), cb.Bytes())
}
