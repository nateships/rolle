package cli

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/core"
)

// jsonFlag switches list, status, and start to machine-readable output.
var jsonFlag bool

// Exit codes. Scripts and agents branch on them instead of parsing text.
const (
	ExitError         = 1
	ExitLoginRequired = 3
	ExitNotFound      = 4
)

// ExitCode maps a command error to the process exit code.
func ExitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case app.LoginRequired(err):
		return ExitLoginRequired
	case errors.Is(err, core.ErrNotFound):
		return ExitNotFound
	}
	return ExitError
}

// sessionJSON is the stable shape of a session on the command line. It
// carries no credentials: a script uses the profile name and lets the SDK
// fetch them through credential_process.
type sessionJSON struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Kind        core.Kind  `json:"kind"`
	Cloud       core.Cloud `json:"cloud"`
	Status      string     `json:"status"`
	Region      string     `json:"region,omitempty"`
	Profile     string     `json:"profile,omitempty"`
	AccountID   string     `json:"accountId,omitempty"`
	RoleName    string     `json:"roleName,omitempty"`
	Integration string     `json:"integration,omitempty"`
	Expires     *time.Time `json:"expires,omitempty"`
	Favorite    bool       `json:"favorite,omitempty"`
	Hidden      bool       `json:"hidden,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
}

func sessionOut(w *core.Workspace, s core.Session) sessionJSON {
	out := sessionJSON{
		ID: s.ID, Name: s.Name, Kind: s.Kind, Cloud: s.Kind.Cloud(), Status: string(s.Status),
		Region: s.Region, Expires: s.Expires, Favorite: s.Favorite, Hidden: s.Hidden, Tags: s.Tags,
	}
	if s.Kind.Cloud() == core.CloudAWS {
		out.Profile = app.ProfileName(&s)
	}
	if s.AWS != nil {
		out.AccountID, out.RoleName = s.AWS.AccountID, s.AWS.RoleName
	}
	if in, err := w.Integration(s.IntegrationID); err == nil {
		out.Integration = in.Alias
	}
	return out
}

func sessionsOut(w *core.Workspace, sessions []core.Session) []sessionJSON {
	out := make([]sessionJSON, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, sessionOut(w, s))
	}
	return out
}

// integrationJSON is the stable shape of an identity source.
type integrationJSON struct {
	ID       string     `json:"id"`
	Alias    string     `json:"alias"`
	Cloud    core.Cloud `json:"cloud"`
	SignedIn bool       `json:"signedIn"`
	Account  string     `json:"account,omitempty"`
	Expires  *time.Time `json:"expires,omitempty"`
}

func integrationOut(in core.Integration) integrationJSON {
	out := integrationJSON{ID: in.ID, Alias: in.Alias, Cloud: in.Cloud}
	switch {
	case in.AWSSSO != nil:
		out.SignedIn, out.Expires = in.AWSSSO.TokenExpires != nil, in.AWSSSO.TokenExpires
	case in.Azure != nil:
		out.SignedIn, out.Account = in.Azure.Account != "", in.Azure.Account
	case in.GCP != nil:
		out.SignedIn, out.Account = in.GCP.Account != "", in.GCP.Account
	}
	return out
}

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
