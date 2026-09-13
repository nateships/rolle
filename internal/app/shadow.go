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

// ShadowedProfiles maps every AWS session's profile name to the file that
// shadows it. Profiles nothing shadows are absent.
func (s *Service) ShadowedProfiles() (map[string]string, error) {
	w, err := s.Load()
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for i := range w.Sessions {
		sess := &w.Sessions[i]
		if sess.Kind.Cloud() != core.CloudAWS {
			continue
		}
		name := ProfileName(sess)
		if _, seen := out[name]; seen {
			continue
		}
		if sh := s.ProfileShadow(name); sh != nil {
			out[name] = sh.Path
		}
	}
	return out, nil
}

// StaticKeys describes the shared credentials file's static keys.
type StaticKeys struct {
	// Path is the shared credentials file.
	Path string `json:"path"`
	// Profiles are the sections that hold static keys, in file order.
	Profiles []string `json:"profiles"`
}

// StaticProfiles lists the sections of the shared credentials file that hold
// static keys. Tools read those before any rolle profile of the same name.
func (s *Service) StaticProfiles() StaticKeys {
	profiles, path := awsconfig.StaticProfiles(s.AWSConfigPath)
	if profiles == nil {
		profiles = []string{}
	}
	return StaticKeys{Path: path, Profiles: profiles}
}

// RemoveStaticProfile deletes the static keys of one section of the shared
// credentials file. Other keys of the section and other sections stay.
func (s *Service) RemoveStaticProfile(name string) error {
	if name == "" {
		return fmt.Errorf("profile name is empty")
	}
	return awsconfig.RemoveStaticKeys(s.AWSConfigPath, name)
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
		return fmt.Errorf("profile %q in %s is configured by another tool; give the session another profile name", name, sh.Path)
	}
	return awsconfig.RemoveStaticKeys(s.AWSConfigPath, name)
}
