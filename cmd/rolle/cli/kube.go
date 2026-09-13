package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/nateships/rolle/internal/app"
	"github.com/nateships/rolle/internal/core"
	"github.com/nateships/rolle/internal/kube"
)

func kubeCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "kube", Short: "Kubernetes contexts that authenticate through a session"}
	cmd.AddCommand(kubeListCmd(), kubeAddCmd(), kubeTokenCmd(), kubeAttachCmd())
	return cmd
}

// clusterJSON is the stable shape of a cluster on the command line.
type clusterJSON struct {
	Name     string     `json:"name"`
	Location string     `json:"location"`
	Endpoint string     `json:"endpoint"`
	Cloud    core.Cloud `json:"cloud"`
}

func clustersOut(clusters []kube.Cluster) []clusterJSON {
	out := make([]clusterJSON, 0, len(clusters))
	for _, c := range clusters {
		out = append(out, clusterJSON{Name: c.Name, Location: c.Location, Endpoint: c.Endpoint, Cloud: c.Cloud})
	}
	return out
}

func printClusters(clusters []kube.Cluster) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tLOCATION\tENDPOINT")
	for _, c := range clusters {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", c.Name, c.Location, c.Endpoint)
	}
	return tw.Flush()
}

func kubeListCmd() *cobra.Command {
	var region string
	cmd := &cobra.Command{
		Use:   "list <session>",
		Short: "List the managed clusters a session can reach",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusters, err := svc.KubeClusters(cmd.Context(), args[0], region)
			if err != nil {
				return err
			}
			if jsonFlag {
				return writeJSON(clustersOut(clusters))
			}
			return printClusters(clusters)
		},
	}
	cmd.Flags().StringVar(&region, "region", "", "AWS region to search (defaults to the session region)")
	return cmd
}

// entryJSON is what kube add reports for each context it writes.
type entryJSON struct {
	Context    string `json:"context"`
	Cluster    string `json:"cluster"`
	Server     string `json:"server"`
	User       string `json:"user"`
	Kubeconfig string `json:"kubeconfig"`
}

func kubeAddCmd() *cobra.Command {
	var all, use bool
	var region, path, context string
	cmd := &cobra.Command{
		Use:   "add <session> [cluster ...]",
		Short: "Write kubeconfig contexts for a session's clusters",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clusters, err := svc.KubeClusters(cmd.Context(), args[0], region)
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
			if len(args) == 1 && !all {
				if len(clusters) == 0 {
					return fmt.Errorf("%s reaches no clusters", sess.Name)
				}
				if err := printClusters(clusters); err != nil {
					return err
				}
				return errors.New("name one or more of these clusters, or pass --all")
			}
			if !all {
				clusters, err = pickClusters(clusters, args[1:])
				if err != nil {
					return err
				}
			}
			if (context != "" || use) && len(clusters) != 1 {
				return errors.New("--context and --use apply to one cluster")
			}
			if path == "" {
				if path, err = kube.DefaultPath(); err != nil {
					return err
				}
			}
			entries := make([]kube.Entry, 0, len(clusters))
			for _, c := range clusters {
				e := kube.Entry{Context: c.Name, Cluster: c.Name, Server: c.Endpoint, CA: c.CA}
				if context != "" {
					e.Context = context
				}
				if sess.Kind.Cloud() == core.CloudAWS {
					profile := app.ProfileName(sess)
					e.User, e.Exec = "rolle:"+profile, kube.AWSExec(c.Region, c.Name, profile)
				} else {
					e.User, e.Exec = "rolle:"+sess.Name, kube.RolleExec(sess.Name)
				}
				entries = append(entries, e)
			}
			current := ""
			if use {
				current = entries[0].Context
			}
			if err := kube.Merge(path, entries, current); err != nil {
				return err
			}
			if jsonFlag {
				out := make([]entryJSON, 0, len(entries))
				for _, e := range entries {
					out = append(out, entryJSON{Context: e.Context, Cluster: e.Cluster, Server: e.Server, User: e.User, Kubeconfig: path})
				}
				return writeJSON(out)
			}
			for _, e := range entries {
				fmt.Printf("context %s written to %s\n", e.Context, path)
			}
			fmt.Printf("next: kubectl --context %q get nodes\n", entries[0].Context)
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "add every cluster the session reaches")
	cmd.Flags().BoolVar(&use, "use", false, "make the context the current one")
	cmd.Flags().StringVar(&region, "region", "", "AWS region to search (defaults to the session region)")
	cmd.Flags().StringVar(&path, "kubeconfig", "", "kubeconfig to write (defaults to $KUBECONFIG, then ~/.kube/config)")
	cmd.Flags().StringVar(&context, "context", "", "context name (defaults to the cluster name)")
	return cmd
}

// pickClusters returns the clusters named in names, in that order.
func pickClusters(clusters []kube.Cluster, names []string) ([]kube.Cluster, error) {
	out := make([]kube.Cluster, 0, len(names))
	for _, name := range names {
		found := false
		for _, c := range clusters {
			if c.Name == name {
				out = append(out, c)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("cluster %q: %w", name, core.ErrNotFound)
		}
	}
	return out, nil
}

// execCredential is the JSON shape kubectl expects from an exec plugin.
type execCredential struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Status     struct {
		Token               string `json:"token"`
		ExpirationTimestamp string `json:"expirationTimestamp,omitempty"`
	} `json:"status"`
}

func kubeTokenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "token <session>",
		Short: "Print an ExecCredential for kubectl (used by the contexts kube add writes)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := svc.KubeToken(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			out := execCredential{APIVersion: kube.ExecAPIVersion, Kind: "ExecCredential"}
			out.Status.Token = creds.Token
			if creds.Expiration != nil {
				out.Status.ExpirationTimestamp = creds.Expiration.UTC().Format(time.RFC3339)
			}
			return json.NewEncoder(os.Stdout).Encode(out)
		},
	}
}

func kubeAttachCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "attach <session> <context>",
		Short: "Make an existing kubeconfig context authenticate through a session",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			w, err := svc.Load()
			if err != nil {
				return err
			}
			sess, err := app.FindSession(w, args[0])
			if err != nil {
				return err
			}
			if path == "" {
				if path, err = kube.DefaultPath(); err != nil {
					return err
				}
			}
			if sess.Kind.Cloud() == core.CloudAWS {
				err = kube.AttachEnv(path, args[1], "AWS_PROFILE", app.ProfileName(sess))
			} else {
				err = kube.AttachExec(path, args[1], kube.RolleExec(sess.Name))
			}
			if err != nil {
				return err
			}
			fmt.Printf("context %s in %s now authenticates through %s\n", args[1], path, sess.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "kubeconfig", "", "kubeconfig to edit (defaults to $KUBECONFIG, then ~/.kube/config)")
	return cmd
}
