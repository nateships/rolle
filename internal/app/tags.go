package app

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/nateships/rolle/internal/core"
)

// maxTagLength bounds a tag name so the sidebar stays readable.
const maxTagLength = 40

// ErrTagExists is returned when a tag with the same name, in any case,
// already exists.
var ErrTagExists = errors.New("tag exists")

// hexColor matches a #rrggbb value.
var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// iconName matches a Lucide icon name: lower-case words joined by hyphens.
var iconName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// normalizeTag checks a tag: the name trimmed with inner whitespace
// collapsed, the color a #rrggbb value, and the icon a Lucide icon name.
func normalizeTag(t core.Tag) (core.Tag, error) {
	t.Name = strings.Join(strings.Fields(t.Name), " ")
	if t.Name == "" {
		return t, errors.New("tag name is empty")
	}
	if utf8.RuneCountInString(t.Name) > maxTagLength {
		return t, fmt.Errorf("tag name is longer than %d characters", maxTagLength)
	}
	if t.Color != "" && !hexColor.MatchString(t.Color) {
		return t, fmt.Errorf("tag color %q is not a #rrggbb value", t.Color)
	}
	t.Color = strings.ToLower(t.Color)
	t.Icon = strings.TrimSpace(t.Icon)
	if t.Icon != "" && (len(t.Icon) > 50 || !iconName.MatchString(t.Icon)) {
		return t, fmt.Errorf("tag icon %q is not a Lucide icon name such as shield", t.Icon)
	}
	return t, nil
}

// findTag returns the index of name in tags, matching case-insensitively.
func findTag(tags []core.Tag, name string) int {
	return slices.IndexFunc(tags, func(t core.Tag) bool { return strings.EqualFold(t.Name, name) })
}

// findName returns the index of name in names, matching case-insensitively.
func findName(names []string, name string) int {
	return slices.IndexFunc(names, func(n string) bool { return strings.EqualFold(n, name) })
}

// FindTag returns the tag called name, matching case-insensitively.
func FindTag(w *core.Workspace, name string) (*core.Tag, error) {
	i := findTag(w.Tags, name)
	if i < 0 {
		return nil, fmt.Errorf("tag %s: %w", name, core.ErrNotFound)
	}
	return &w.Tags[i], nil
}

// AddTag creates a tag. Tags are user-defined groups in the sidebar; a
// session carries any number of them.
func (s *Service) AddTag(t core.Tag) error {
	t, err := normalizeTag(t)
	if err != nil {
		return err
	}
	w, err := s.Load()
	if err != nil {
		return err
	}
	if findTag(w.Tags, t.Name) >= 0 {
		return fmt.Errorf("%s: %w", t.Name, ErrTagExists)
	}
	w.Tags = append(w.Tags, t)
	return s.Save(w)
}

// UpdateTag changes a tag's name, color, or icon. A new name follows
// through to every session that carries the tag.
func (s *Service) UpdateTag(name string, t core.Tag) error {
	t, err := normalizeTag(t)
	if err != nil {
		return err
	}
	w, err := s.Load()
	if err != nil {
		return err
	}
	i := findTag(w.Tags, name)
	if i < 0 {
		return fmt.Errorf("tag %s: %w", name, core.ErrNotFound)
	}
	if j := findTag(w.Tags, t.Name); j >= 0 && j != i {
		return fmt.Errorf("%s: %w", t.Name, ErrTagExists)
	}
	old := w.Tags[i].Name
	w.Tags[i] = t
	if old != t.Name {
		for k := range w.Sessions {
			if n := findName(w.Sessions[k].Tags, old); n >= 0 {
				w.Sessions[k].Tags[n] = t.Name
			}
		}
	}
	return s.Save(w)
}

// RemoveTag deletes a tag and takes it off every session.
func (s *Service) RemoveTag(name string) error {
	w, err := s.Load()
	if err != nil {
		return err
	}
	i := findTag(w.Tags, name)
	if i < 0 {
		return fmt.Errorf("tag %s: %w", name, core.ErrNotFound)
	}
	w.Tags = slices.Delete(w.Tags, i, i+1)
	for k := range w.Sessions {
		if n := findName(w.Sessions[k].Tags, name); n >= 0 {
			w.Sessions[k].Tags = slices.Delete(w.Sessions[k].Tags, n, n+1)
		}
	}
	return s.Save(w)
}

// MoveTag puts a tag at index in the sidebar order.
func (s *Service) MoveTag(name string, index int) error {
	w, err := s.Load()
	if err != nil {
		return err
	}
	i := findTag(w.Tags, name)
	if i < 0 {
		return fmt.Errorf("tag %s: %w", name, core.ErrNotFound)
	}
	index = max(0, min(index, len(w.Tags)-1))
	if index == i {
		return nil
	}
	tag := w.Tags[i]
	rest := slices.Delete(slices.Clone(w.Tags), i, i+1)
	w.Tags = slices.Insert(rest, index, tag)
	return s.Save(w)
}

// SetSessionTag adds a tag to a session or takes it off. The tag must exist.
func (s *Service) SetSessionTag(ref, tag string, on bool) error {
	w, err := s.Load()
	if err != nil {
		return err
	}
	i := findTag(w.Tags, tag)
	if i < 0 {
		return fmt.Errorf("tag %s: %w", tag, core.ErrNotFound)
	}
	tag = w.Tags[i].Name
	sess, err := FindSession(w, ref)
	if err != nil {
		return err
	}
	n := findName(sess.Tags, tag)
	switch {
	case on && n < 0:
		sess.Tags = append(sess.Tags, tag)
	case !on && n >= 0:
		sess.Tags = slices.Delete(sess.Tags, n, n+1)
	default:
		return nil
	}
	return s.Save(w)
}
