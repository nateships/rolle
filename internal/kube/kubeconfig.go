package kube

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/nateships/rolle/internal/atomicfile"
	"github.com/nateships/rolle/internal/core"
)

// Entry is one context with its cluster and user.
type Entry struct {
	Context string
	Cluster string
	Server  string
	// CA is the PEM certificate of the API server.
	CA   []byte
	User string
	Exec Exec
}

// Merge writes entries into the kubeconfig at path. A cluster or user with
// the same name replaces the old one; a context with the same name keeps its
// other fields, such as the namespace. Other entries and unknown fields
// stay. A missing file becomes a new kubeconfig. current, when set, becomes
// current-context.
func Merge(path string, entries []Entry, current string) error {
	doc, err := load(path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		cluster := map[string]any{"server": e.Server}
		if len(e.CA) > 0 {
			cluster["certificate-authority-data"] = base64.StdEncoding.EncodeToString(e.CA)
		}
		upsert(doc, "clusters", e.Cluster, cluster)
		context := map[string]any{}
		if old := find(doc, "contexts", e.Context); old != nil {
			if body, ok := old["context"].(map[string]any); ok {
				context = body
			}
		}
		context["cluster"], context["user"] = e.Cluster, e.User
		upsert(doc, "contexts", e.Context, context)
		upsert(doc, "users", e.User, map[string]any{"exec": e.Exec.spec()})
	}
	if current != "" {
		doc["current-context"] = current
	}
	return save(path, doc)
}

// Attached tells where attach found the context and which user it changed.
type Attached struct {
	// ContextPath is the file that holds the context.
	ContextPath string
	User        string
	// UserPath is the file that holds the user. Attach writes only this file.
	UserPath string
}

// AttachEnv sets one environment variable on the exec plugin of the user
// behind context. The user must run an exec plugin already.
func AttachEnv(paths []string, context, name, value string) (Attached, error) {
	return updateUser(paths, context, func(user map[string]any) error {
		exec, ok := user["exec"].(map[string]any)
		if !ok {
			return fmt.Errorf("the user of context %q has no exec plugin; attach works with a plugin that reads AWS credentials, such as aws-iam-authenticator", context)
		}
		env, _ := exec["env"].([]any)
		exec["env"] = upsertList(env, name, map[string]any{"name": name, "value": value})
		return nil
	})
}

// AttachExec replaces the exec plugin of the user behind context.
func AttachExec(paths []string, context string, exec Exec) (Attached, error) {
	return updateUser(paths, context, func(user map[string]any) error {
		user["exec"] = exec.spec()
		return nil
	})
}

// spec returns the exec block as kubeconfig fields.
func (e Exec) spec() map[string]any {
	args := make([]any, 0, len(e.Args))
	for _, a := range e.Args {
		args = append(args, a)
	}
	spec := map[string]any{"apiVersion": e.APIVersion, "command": e.Command, "args": args, "interactiveMode": "Never"}
	if len(e.Env) > 0 {
		env := make([]any, 0, len(e.Env))
		for _, kv := range e.Env {
			env = append(env, map[string]any{"name": kv[0], "value": kv[1]})
		}
		spec["env"] = env
	}
	return spec
}

// updateUser loads the kubeconfigs, finds the user of context, applies fn,
// and saves the file that holds the user. Like kubectl, the first file with
// a name wins, and the context and its user can be in different files. A
// missing context or user is core.ErrNotFound.
func updateUser(paths []string, context string, fn func(user map[string]any) error) (Attached, error) {
	docs := make([]map[string]any, len(paths))
	for i, p := range paths {
		doc, err := load(p)
		if err != nil {
			return Attached{}, err
		}
		docs[i] = doc
	}
	searched := strings.Join(paths, ", ")
	var ctx map[string]any
	var at Attached
	for i, doc := range docs {
		if ctx = find(doc, "contexts", context); ctx != nil {
			at.ContextPath = paths[i]
			break
		}
	}
	if ctx == nil {
		return Attached{}, fmt.Errorf("context %q in %s: %w", context, searched, core.ErrNotFound)
	}
	body, _ := ctx["context"].(map[string]any)
	name, _ := body["user"].(string)
	at.User = name
	for i, doc := range docs {
		item := find(doc, "users", name)
		if item == nil {
			continue
		}
		user, ok := item["user"].(map[string]any)
		if !ok {
			user = map[string]any{}
			item["user"] = user
		}
		if err := fn(user); err != nil {
			return Attached{}, err
		}
		at.UserPath = paths[i]
		return at, save(paths[i], doc)
	}
	return Attached{}, fmt.Errorf("user %q of context %q in %s: %w", name, context, searched, core.ErrNotFound)
}

// load parses the kubeconfig at path into generic maps, so every field it
// does not know survives a write. A missing file is an empty kubeconfig.
func load(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{"apiVersion": "v1", "kind": "Config"}, nil
	}
	if err != nil {
		return nil, err
	}
	doc := map[string]any{}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	if doc["apiVersion"] == nil {
		doc["apiVersion"] = "v1"
	}
	if doc["kind"] == nil {
		doc["kind"] = "Config"
	}
	return doc, nil
}

// save writes doc atomically, so a failed write leaves the old kubeconfig in
// place. A symlinked kubeconfig stays a link.
func save(path string, doc map[string]any) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	return atomicfile.Write(path, buf.Bytes(), 0o600)
}

// find returns the named item of a kubeconfig list, or nil.
func find(doc map[string]any, key, name string) map[string]any {
	list, _ := doc[key].([]any)
	for _, it := range list {
		if m, ok := it.(map[string]any); ok && m["name"] == name {
			return m
		}
	}
	return nil
}

// upsert replaces or appends a named item in a kubeconfig list. The list
// key is the singular of the list name: clusters hold a cluster.
func upsert(doc map[string]any, key, name string, body map[string]any) {
	list, _ := doc[key].([]any)
	doc[key] = upsertList(list, name, map[string]any{"name": name, key[:len(key)-1]: body})
}

// upsertList replaces the item named name in list, or appends item.
func upsertList(list []any, name string, item map[string]any) []any {
	for i, it := range list {
		if m, ok := it.(map[string]any); ok && m["name"] == name {
			list[i] = item
			return list
		}
	}
	return append(list, item)
}
