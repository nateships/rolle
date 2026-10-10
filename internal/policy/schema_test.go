package policy

import (
	"encoding/json"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// schemaPath is the Jamf Pro custom schema that the docs site serves.
const schemaPath = "../../docs/public/mdm/com.getrolle.app.json"

type schemaProp struct {
	Type       string                `json:"type"`
	Title      string                `json:"title"`
	Enum       []string              `json:"enum"`
	Items      *schemaProp           `json:"items"`
	Properties map[string]schemaProp `json:"properties"`
}

// The schema must offer exactly the keys that the reader accepts, so a key
// added to one is not missing from the other.
func TestJamfSchemaMatchesReader(t *testing.T) {
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var schema schemaProp
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}

	var top []string
	for f := range reflect.TypeFor[profile]().Fields() {
		top = append(top, f.Tag.Get("plist"))
	}
	if got := slices.Sorted(maps.Keys(schema.Properties)); !slices.Equal(got, slices.Sorted(slices.Values(top))) {
		t.Errorf("top-level keys = %v, reader = %v", got, top)
	}

	want := slices.Sorted(maps.Keys(keys))
	settings := schema.Properties["Settings"]
	if got := slices.Sorted(maps.Keys(settings.Properties)); !slices.Equal(got, want) {
		t.Errorf("Settings keys = %v, reader = %v", got, want)
	}
	if locked := schema.Properties["LockedSettings"]; locked.Items == nil || !slices.Equal(slices.Sorted(slices.Values(locked.Items.Enum)), want) {
		t.Errorf("LockedSettings enum does not list the settings keys")
	}
	// Each setting's value must pass the reader's type check.
	for key, p := range settings.Properties {
		if p.Title == "" {
			t.Errorf("Settings.%s has no title", key)
		}
		var sample any
		switch p.Type {
		case "string":
			sample = "x"
			if len(p.Enum) > 0 {
				sample = p.Enum[0]
			}
		case "integer":
			sample = uint64(60)
		case "boolean":
			sample = true
		case "array":
			sample = []any{"gcp"}
		default:
			t.Errorf("Settings.%s has type %q", key, p.Type)
			continue
		}
		if _, _, ok := setting(key, sample); !ok {
			t.Errorf("Settings.%s: a %s value does not pass the reader", key, p.Type)
		}
	}

	in := schema.Properties["Integrations"]
	if in.Items == nil {
		t.Fatal("Integrations has no items")
	}
	fields := slices.Sorted(maps.Keys(in.Items.Properties))
	if !slices.Equal(fields, []string{"Alias", "Region", "StartURL", "TenantID", "Type"}) {
		t.Errorf("Integrations fields = %v", fields)
	}
	if got := in.Items.Properties["Type"].Enum; !slices.Equal(got, []string{"aws-sso", "azure"}) {
		t.Errorf("Integrations Type enum = %v", got)
	}
	if !strings.Contains(string(data), `"property_order"`) {
		t.Error("no property_order: Jamf Pro then lists the keys in any order")
	}
}

// The docs page lists every key that the reader accepts.
func TestDocsListEveryKey(t *testing.T) {
	data, err := os.ReadFile("../../docs/content/(reference)/mdm.mdx")
	if err != nil {
		t.Fatal(err)
	}
	for key := range keys {
		if !strings.Contains(string(data), "| `"+key+"` |") {
			t.Errorf("mdm.mdx has no row for %s", key)
		}
	}
	for _, key := range []string{"DisableUpdates", "Settings", "LockedSettings", "Integrations", "Type", "Alias", "StartURL", "Region", "TenantID"} {
		if !strings.Contains(string(data), "| `"+key+"` |") {
			t.Errorf("mdm.mdx has no row for %s", key)
		}
	}
}
