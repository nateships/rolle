package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/aws"
	"github.com/nateships/rolle/internal/awsconfig"
	"github.com/nateships/rolle/internal/browser"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/terminal"
)

func startCmd() *cobra.Command {
	var mfa string
	var noBrowser bool
	cmd := &cobra.Command{
		Use:   "start <session>",
		Short: "Start a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := svc.Start(cmd.Context(), args[0], app.StartOptions{MFACode: mfa})
			// A console login session signs in through the browser first.
			if errors.Is(err, aws.ErrLoginRequired) {
				if err = consoleLogin(cmd, args[0], noBrowser); err != nil {
					return err
				}
				creds, err = svc.Start(cmd.Context(), args[0], app.StartOptions{MFACode: mfa})
			}
			if errors.Is(err, awsconfig.ErrShadowed) {
				return fmt.Errorf("%w; tools read those first\nrolle session fix-profile %q removes them", err, args[0])
			}
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
			if jsonFlag {
				return writeJSON(sessionOut(w, *sess))
			}
			until := ""
			if creds.Expiration != nil {
				until = " until " + creds.Expiration.Local().Format(time.Kitchen)
			}
			fmt.Printf("%s active%s\n", sess.Name, until)
			if sess.Kind.Cloud() == core.CloudAWS {
				fmt.Printf("AWS profile: %s\n", app.ProfileName(sess))
			} else {
				fmt.Printf("shell: eval \"$(rolle env %q)\"\n", sess.Name)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&mfa, "mfa-code", "", "one-time MFA code")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "print the sign-in URL instead of opening a browser")
	return cmd
}

// consoleLogin runs the browser sign-in of a console login session. The
// prompts go to stderr so --json output stays clean.
func consoleLogin(cmd *cobra.Command, ref string, noBrowser bool) error {
	auth, err := svc.AWSLogin(cmd.Context(), ref)
	if err != nil {
		return err
	}
	if noBrowser {
		fmt.Fprintf(os.Stderr, "Open %s\n", auth.VerificationURI)
	} else {
		fmt.Fprintf(os.Stderr, "Approve the sign-in in your browser. If it did not open:\n%s\n", auth.VerificationURI)
		_ = browser.Open(auth.VerificationURI)
	}
	fmt.Fprintln(os.Stderr, "Waiting for approval...")
	if err := auth.Wait(cmd.Context()); err != nil {
		return err
	}
	_, err = svc.FinishAWSLogin(ref)
	return err
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

// tokenCmd prints the bearer token of an Azure or Google Cloud session, for
// tools that take one value: a mise template, a curl header, a kubeconfig.
func tokenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "token <session>",
		Short: "Print the bearer token of an Azure or Google Cloud session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := svc.Credentials(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if creds.Token == "" {
				return fmt.Errorf("%s has no bearer token; AWS sessions are profiles, use rolle env or --profile", args[0])
			}
			fmt.Println(creds.Token)
			return nil
		},
	}
}

func envCmd() *cobra.Command {
	var powershell bool
	cmd := &cobra.Command{
		Use:   "env <session>",
		Short: "Print credentials as shell exports",
		Long:  "Print credentials as shell exports. Use with eval \"$(rolle env prod)\".",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := svc.SessionEnv(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Print(terminal.EvalExports(env, powershell))
			return nil
		},
	}
	cmd.Flags().BoolVar(&powershell, "powershell", false, "emit PowerShell syntax")
	return cmd
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			w, err := svc.RefreshContext(cmd.Context())
			if err != nil {
				return err
			}
			var active []core.Session
			for _, s := range w.Sessions {
				if s.Status == core.StatusActive {
					active = append(active, s)
				}
			}
			if jsonFlag {
				return writeJSON(sessionsOut(w, active))
			}
			if len(active) == 0 {
				fmt.Println("no active sessions")
				return nil
			}
			return printSessions(active)
		},
	}
}

func resetCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Remove every session, integration, secret, and cached credential",
		RunE: func(_ *cobra.Command, _ []string) error {
			if !yes {
				fmt.Fprint(os.Stderr, "This removes all rolle sessions, integrations, keychain secrets, cached credentials, and rolle-owned AWS profiles. Continue? [y/N] ")
				var answer string
				_, _ = fmt.Scanln(&answer)
				if answer != "y" && answer != "Y" {
					return errors.New("aborted")
				}
			}
			if err := svc.ResetAll(); err != nil {
				return err
			}
			fmt.Println("rolle reset. The desktop app will show onboarding again.")
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask for confirmation")
	return cmd
}

func shellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell <session>",
		Short: "Open a new terminal window with the session's credentials ready",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return svc.OpenTerminal(cmd.Context(), args[0]) },
	}
}
