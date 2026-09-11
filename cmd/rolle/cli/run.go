package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/browser"
	"github.com/nateships/rolle/internal/core"
)

func startCmd() *cobra.Command {
	var mfa string
	cmd := &cobra.Command{
		Use:   "start <session>",
		Short: "Start a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := svc.Start(cmd.Context(), args[0], app.StartOptions{MFACode: mfa})
			if err != nil {
				return err
			}
			w, err := svc.Load()
			if err != nil {
				return err
			}
			sess, err := app.FindSession(w, args[0])
			if err != nil {
				return err
			}
			until := ""
			if creds.Expiration != nil {
				until = " until " + creds.Expiration.Local().Format(time.Kitchen)
			}
			fmt.Printf("%s active%s\n", sess.Name, until)
			if sess.Kind.Cloud() == core.CloudAWS {
				fmt.Printf("AWS profile: %s\n", app.ProfileName(sess))
			} else {
				fmt.Printf("shell: eval \"$(rolle env %s)\"\n", sess.Name)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&mfa, "mfa-code", "", "one-time MFA code")
	return cmd
}

func stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop <session>",
		Short: "Stop a session",
		Args:  cobra.ExactArgs(1),
		RunE:  func(_ *cobra.Command, args []string) error { return svc.Stop(args[0]) },
	}
}

// credentialProcessOutput is the JSON shape the AWS SDKs expect from credential_process.
type credentialProcessOutput struct {
	Version         int    `json:"Version"`
	AccessKeyID     string `json:"AccessKeyId"`
	SecretAccessKey string `json:"SecretAccessKey"`
	SessionToken    string `json:"SessionToken,omitempty"`
	Expiration      string `json:"Expiration,omitempty"`
}

func credsCmd() *cobra.Command {
	var session string
	cmd := &cobra.Command{
		Use:    "creds",
		Short:  "Print credentials for credential_process",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, err := svc.Credentials(cmd.Context(), session)
			if err != nil {
				return err
			}
			out := credentialProcessOutput{Version: 1, AccessKeyID: creds.AccessKeyID, SecretAccessKey: creds.SecretAccessKey, SessionToken: creds.SessionToken}
			if creds.Expiration != nil {
				out.Expiration = creds.Expiration.UTC().Format(time.RFC3339)
			}
			return json.NewEncoder(os.Stdout).Encode(out)
		},
	}
	cmd.Flags().StringVar(&session, "session", "", "session ID")
	_ = cmd.MarkFlagRequired("session")
	return cmd
}

func envCmd() *cobra.Command {
	var powershell bool
	cmd := &cobra.Command{
		Use:   "env <session>",
		Short: "Print credentials as shell exports",
		Long:  "Print credentials as shell exports. Use with eval \"$(rolle env prod)\".",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := svc.Credentials(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			w, err := svc.Load()
			if err != nil {
				return err
			}
			sess, err := app.FindSession(w, args[0])
			if err != nil {
				return err
			}
			return printEnv(sess, creds, powershell)
		},
	}
	cmd.Flags().BoolVar(&powershell, "powershell", false, "emit PowerShell syntax")
	return cmd
}

func printEnv(sess *core.Session, creds core.Credentials, powershell bool) error {
	for _, kv := range app.EnvVars(sess, creds) {
		if kv[1] == "" {
			continue
		}
		if powershell {
			fmt.Printf("$env:%s = \"%s\"\n", kv[0], kv[1])
		} else {
			fmt.Printf("export %s=%q\n", kv[0], kv[1])
		}
	}
	return nil
}

func consoleCmd() *cobra.Command {
	var print bool
	cmd := &cobra.Command{
		Use:   "console <session>",
		Short: "Open the cloud console for a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			u, err := svc.ConsoleURLFor(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if print {
				fmt.Println(u)
				return nil
			}
			return browser.Open(u)
		},
	}
	cmd.Flags().BoolVar(&print, "print", false, "print the URL instead of opening it")
	return cmd
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show active sessions",
		RunE: func(_ *cobra.Command, _ []string) error {
			w, err := svc.Refresh()
			if err != nil {
				return err
			}
			var active []core.Session
			for _, s := range w.Sessions {
				if s.Status == core.StatusActive {
					active = append(active, s)
				}
			}
			if len(active) == 0 {
				fmt.Println("no active sessions")
				return nil
			}
			return printSessions(active)
		},
	}
}
