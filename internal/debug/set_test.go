package debug

import "testing"

func TestSetHonoursEnvironment(t *testing.T) {
	prev := enabled.Load()
	t.Cleanup(func() { enabled.Store(prev) })
	cases := []struct {
		env  string
		on   bool
		want bool
	}{
		{"", false, false},
		{"", true, true},
		{"0", false, false},
		{"0", true, true},
		{"false", false, false},
		{"1", false, true},
		{"true", false, true},
		{"yes", false, true},
	}
	for _, tc := range cases {
		t.Run("ROLLE_DEBUG="+tc.env, func(t *testing.T) {
			t.Setenv("ROLLE_DEBUG", tc.env)
			enabled.Store(!tc.want)
			Set(tc.on)
			if got := Enabled(); got != tc.want {
				t.Fatalf("Set(%v) with ROLLE_DEBUG=%q: Enabled = %v, want %v", tc.on, tc.env, got, tc.want)
			}
		})
	}
}

func TestEnableIsSticky(t *testing.T) {
	prev := enabled.Load()
	t.Cleanup(func() { enabled.Store(prev) })
	enabled.Store(false)
	Enable()
	if !Enabled() {
		t.Fatal("Enable did not turn debug on")
	}
	Enable()
	if !Enabled() {
		t.Fatal("second Enable turned debug off")
	}
}
