package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/crmne/hyprmoncfg/internal/writerlock"
)

func TestClaimWriterWaitsForDirectEditor(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	owner, err := writerlock.TryAcquire()
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	waiting := make(chan struct{}, 1)
	result := make(chan *writerlock.Lock, 1)
	errCh := make(chan error, 1)
	go func() {
		lock, err := claimWriterOwnership(ctx, func(string, ...any) { waiting <- struct{}{} })
		result <- lock
		errCh <- err
	}()
	select {
	case <-waiting:
	case err := <-errCh:
		t.Fatalf("daemon exited instead of waiting: %v", err)
	case <-ctx.Done():
		t.Fatal("daemon did not report writer contention")
	}
	select {
	case err := <-errCh:
		t.Fatalf("daemon exited while editor owned lock: %v", err)
	default:
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case lock := <-result:
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
		if lock == nil {
			t.Fatal("missing writer ownership")
		}
		defer lock.Close()
		if _, err := writerlock.TryAcquire(); !errors.Is(err, writerlock.ErrBusy) {
			t.Fatalf("writer exclusivity lost: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("daemon did not acquire released lock")
	}
}

func TestClaimWriterCanStopWhileEditorIsOpen(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	owner, err := writerlock.TryAcquire()
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waiting := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		lock, err := claimWriterOwnership(ctx, func(string, ...any) { waiting <- struct{}{} })
		if lock != nil {
			lock.Close()
		}
		done <- err
	}()
	select {
	case <-waiting:
	case err := <-done:
		t.Fatalf("daemon exited before cancellation: %v", err)
	case <-time.After(time.Second):
		t.Fatal("daemon did not wait")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("stop error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiting prevented shutdown")
	}
}
