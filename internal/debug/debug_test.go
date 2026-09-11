package debug

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestLogfOnlyWhenEnabled(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil)
	enabled.Store(false)
	Logf("test", "hidden %d", 1)
	if buf.Len() != 0 {
		t.Fatalf("logged while disabled: %q", buf.String())
	}
	Enable()
	Logf("test", "shown %d", 2)
	if !strings.Contains(buf.String(), "[rolle:test] shown 2") {
		t.Fatalf("log = %q", buf.String())
	}
}
