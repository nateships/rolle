package kube

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"

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

// AttachEnv sets one environment variable on the exec plugin of the user
// behind context. The user must run an exec plugin already.
func AttachEnv(path, context, name, value string) error {
	return updateUser(path, context, func(user map[string]any) error {
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
func AttachExec(path, context string, exec Exec) error {
	return updateUser(path, context, func(user map[string]any) error {
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

// updateUser loads the kubeconfig, finds the user of context, applies fn,
// and saves. A missing context is core.ErrNotFound.
func updateUser(path, context string, fn func(user map[string]any) error) error {
	doc, err := load(path)
	if err != nil {
		return err
	}
	ctx := find(doc, "contexts", context)
	if ctx == nil {
		return fmt.Errorf("context %q in %s: %w", context, path, core.ErrNotFound)
	}
	body, _ := ctx["context"].(map[string]any)
	name, _ := body["user"].(string)
	item := find(doc, "users", name)
	if item == nil {
		return fmt.Errorf("user %q of context %q in %s: %w", name, context, path, core.ErrNotFound)
	}
	user, ok := item["user"].(map[string]any)
	if !ok {
		user = map[string]any{}
		item["user"] = user
	}
	if err := fn(user); err != nil {
		return err
	}
	return save(path, doc)
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

// save writes doc through a temporary file in the same directory, so a
// failed write leaves the old kubeconfig in place.
func save(path string, doc map[string]any) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".rolle-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(buf.Bytes())
	if err == nil {
		err = tmp.Chmod(0o600)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
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
