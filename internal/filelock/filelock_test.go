package filelock

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestLockExcludesSecondHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "k.lock")
	unlock, err := Lock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}

	// A second open of the same file must wait, also in the same process.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := Lock(ctx, path); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second Lock while held = %v, want deadline exceeded", err)
	}

	got := make(chan error, 1)
	go func() {
		unlock2, err := Lock(context.Background(), path)
		if err == nil {
			unlock2()
		}
		got <- err
	}()
	select {
	case err := <-got:
		t.Fatalf("second Lock returned while held: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	unlock()
	select {
	case err := <-got:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second Lock did not get the lock after unlock")
	}
}
