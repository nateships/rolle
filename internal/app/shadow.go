package app

import (
	"fmt"

	"github.com/nateships/rolle/internal/awsconfig"
	"github.com/nateships/rolle/internal/core"
)

// ProfileShadow reports what keeps tools from using the rolle profile called
// name. Nil when nothing does.
func (s *Service) ProfileShadow(name string) *awsconfig.Shadow {
	return awsconfig.Shadowed(s.AWSConfigPath, name)
}

// ShadowedProfiles maps every AWS session's profile name to the file, in
// display form, whose static keys tools read instead of it. Profiles nothing
// shadows are absent.
func (s *Service) ShadowedProfiles(w *core.Workspace) map[string]string {
	out := map[string]string{}
	checked := map[string]bool{}
	for i := range w.Sessions {
		sess := &w.Sessions[i]
		if sess.Kind.Cloud() != core.CloudAWS {
			continue
		}
		name := ProfileName(sess)
		if checked[name] {
			continue
		}
		checked[name] = true
		if sh := s.ProfileShadow(name); sh != nil {
			out[name] = awsconfig.Display(sh.Path)
		}
	}
	return out
}

// StaticKeys describes the shared credentials file's static keys.
type StaticKeys struct {
	// Path is the shared credentials file, in display form.
	Path string `json:"path"`
	// Profiles hold static keys, in file order, with the values masked.
	Profiles []awsconfig.StaticProfile `json:"profiles"`
}

// StaticProfiles lists the profiles of the shared credentials file that hold
// static keys. Tools read those before any rolle profile of the same name.
func (s *Service) StaticProfiles() StaticKeys {
	profiles, path := awsconfig.StaticProfiles(s.AWSConfigPath)
	if profiles == nil {
		profiles = []awsconfig.StaticProfile{}
	}
	return StaticKeys{Path: awsconfig.Display(path), Profiles: profiles}
}

// RemoveStaticProfile deletes the static keys of one section of the shared
// credentials file. Other keys of the section and other sections stay.
func (s *Service) RemoveStaticProfile(name string) error {
	if name == "" {
		return fmt.Errorf("profile name is empty")
	}
	if err := awsconfig.RemoveStaticKeys(s.AWSConfigPath, name); err != nil {
		return err
	}
	// The workspace file did not change, but what the window shows did.
	s.notify()
	return nil
}

// FixProfile removes the static keys that shadow the session's profile from
// the shared credentials file. A profile that another tool configures in the
// config file cannot be fixed this way; the session needs another name.
func (s *Service) FixProfile(ref string) error {
	w, err := s.Load()
	if err != nil {
		return err
	}
	sess, err := FindSession(w, ref)
	if err != nil {
		return err
	}
	name := ProfileName(sess)
	sh := s.ProfileShadow(name)
	if sh == nil {
		return nil
	}
	if !sh.Fixable {
		return fmt.Errorf("another tool configures profile %q in %s; use another profile name", name, awsconfig.Display(sh.Path))
	}
	return awsconfig.RemoveStaticKeys(s.AWSConfigPath, name)
}
