package app

import (
	"bytes"
	"encoding/json"
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

// applyPolicy puts the profile's settings into w. A value applies when the
// profile sets it and rolle has not applied that value before. Thus the
// user can change it, until the profile sets another value. A locked value
// applies every time. w.Managed records what applied. applyPolicy reports
// whether w changed.
func applyPolicy(w *core.Workspace, p policy.Policy) bool {
	before, _ := json.Marshal([]any{w.Settings, w.Managed})
	var record map[string]json.RawMessage
	if w.Managed != nil {
		record = w.Managed.Settings
	}
	apply := map[string]json.RawMessage{}
	for name, v := range p.Settings {
		if p.IsLocked(name) || !sameJSON(record[name], v) {
			apply[name] = v
		}
	}
	if len(apply) > 0 {
		st := overlay(w.EffectiveSettings(), apply)
		w.Settings = &st
	}
	// A key that left the profile leaves the record too, so the profile
	// can set it again later.
	w.Managed = nil
	if len(p.Settings) > 0 {
		w.Managed = &core.Managed{Settings: p.Settings}
	}
	after, _ := json.Marshal([]any{w.Settings, w.Managed})
	return !sameJSON(before, after)
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
