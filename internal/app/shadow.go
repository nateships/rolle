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
// shadows are absent. The map holds the shadows RemoveStaticProfile clears;
// a profile that another tool configures in the config file is not in it.
func (s *Service) ShadowedProfiles(w *core.Workspace) map[string]string {
	// One parse of the credentials file serves every session.
	profiles, path := awsconfig.StaticProfiles(s.AWSConfigPath)
	static := map[string]bool{}
	for _, p := range profiles {
		static[p.Name] = true
	}
	out := map[string]string{}
	for i := range w.Sessions {
		sess := &w.Sessions[i]
		if sess.Kind.Cloud() != core.CloudAWS {
			continue
		}
		if name := ProfileName(sess); static[name] {
			out[name] = awsconfig.Display(path)
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
// A profile whose access key a rolle session holds is marked imported.
func (s *Service) StaticProfiles() StaticKeys {
	profiles, path := awsconfig.StaticProfiles(s.AWSConfigPath)
	if len(profiles) == 0 {
		return StaticKeys{Path: awsconfig.Display(path), Profiles: []awsconfig.StaticProfile{}}
	}
	have := s.storedAccessKeyIDs()
	for i := range profiles {
		profiles[i].Imported = have[profiles[i].AccessKeyID]
	}
	return StaticKeys{Path: awsconfig.Display(path), Profiles: profiles}
}

// RemoveStaticProfile deletes the static keys of one section of the shared
// credentials file, the whole section. Other sections stay.
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

// ForeignProfile describes the settings of another tool in one profile of
// the AWS config file.
type ForeignProfile struct {
	// Path is the config file, in display form.
	Path string `json:"path"`
	// Keys are the lines that RemoveForeignKeys removes, in file order. The
	// list is empty when no other tool configures the profile.
	Keys []awsconfig.ForeignKey `json:"keys"`
}

// ForeignProfile lists the lines that another tool wrote for the profile
// called name. rolle does not start a session onto such a profile.
func (s *Service) ForeignProfile(name string) ForeignProfile {
	keys := awsconfig.ForeignKeys(s.AWSConfigPath, name)
	if keys == nil {
		keys = []awsconfig.ForeignKey{}
	}
	return ForeignProfile{Path: awsconfig.Display(s.AWSConfigPath), Keys: keys}
}

// RemoveForeignKeys deletes the settings of another tool from one profile of
// the AWS config file, so a session can start onto the profile. Other
// profiles stay.
func (s *Service) RemoveForeignKeys(name string) error {
	if name == "" {
		return fmt.Errorf("profile name is empty")
	}
	if err := awsconfig.RemoveForeignKeys(s.AWSConfigPath, name); err != nil {
		return err
	}
	// The workspace file did not change, but what the window shows did.
	s.notify()
	return nil
}
