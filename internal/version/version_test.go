package version

import (
	"regexp"
	"testing"
)

// The default marks the build as development so the updater stays off. Release
// builds replace it with -ldflags.
func TestDefaultVersionIsDevSemver(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+-dev$`).MatchString(Version) {
		t.Fatalf("Version = %q, want MAJOR.MINOR.PATCH-dev", Version)
	}
}
