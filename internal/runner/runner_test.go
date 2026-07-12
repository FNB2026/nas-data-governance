package runner

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestRunnerSerial verifies that workers=1 runs tasks sequentially.
func TestRunnerSerial(t *testing.T) {
	var counter int32
	r := New(1)
	for i := 0; i < 10; i++ {
		if err := r.Submit(context.Background(), func() error {
			current := atomic.AddInt32(&counter, 1)
			// In serial mode, counter should equal concurrent goroutines
			// at any time. Since workers=1, no two tasks overlap.
			if current > 1 {
				t.Errorf("serial mode should not overlap, counter=%d", current)
			}
			time.Sleep(time.Millisecond)
			atomic.AddInt32(&counter, -1)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if errs := r.Wait(); len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d", len(errs))
	}
}

// TestRunnerConcurrent verifies that workers=N allows N concurrent tasks.
func TestRunnerConcurrent(t *testing.T) {
	var peak int32
	r := New(4)
	var current int32
	for i := 0; i < 20; i++ {
		if err := r.Submit(context.Background(), func() error {
			c := atomic.AddInt32(&current, 1)
			// Track peak concurrency
			for {
				p := atomic.LoadInt32(&peak)
				if c <= p || atomic.CompareAndSwapInt32(&peak, p, c) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&current, -1)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	r.Wait()
	if peak < 2 {
		t.Fatalf("expected peak concurrency >= 2 with 4 workers, got %d", peak)
	}
	if peak > 4 {
		t.Fatalf("expected peak concurrency <= 4, got %d", peak)
	}
}

// TestRunnerCollectsErrors verifies that errors are collected and returned.
func TestRunnerCollectsErrors(t *testing.T) {
	r := New(2)
	for i := 0; i < 5; i++ {
		idx := i
		if err := r.Submit(context.Background(), func() error {
			if idx%2 == 0 {
				return errors.New("even error")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	errs := r.Wait()
	// Tasks 0, 2, 4 should produce errors
	if len(errs) != 3 {
		t.Fatalf("expected 3 errors (even-indexed tasks), got %d", len(errs))
	}
}

// TestRunnerContextCancellation verifies that Submit stops accepting new
// tasks when context is cancelled.
func TestRunnerContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := New(1)

	// Start a long-running task that blocks the single worker.
	r.Submit(context.Background(), func() error {
		time.Sleep(50 * time.Millisecond)
		return nil
	})

	// Cancel context immediately.
	cancel()

	// Subsequent Submit should fail with context error.
	err := r.Submit(ctx, func() error { return nil })
	if err == nil {
		t.Fatal("expected context cancellation error")
	}

	// Wait for the in-flight task to complete.
	r.Wait()
}

// TestRunnerCount verifies the submitted task count.
func TestRunnerCount(t *testing.T) {
	r := New(3)
	for i := 0; i < 7; i++ {
		r.Submit(context.Background(), func() error { return nil })
	}
	r.Wait()
	if r.Count() != 7 {
		t.Fatalf("expected count 7, got %d", r.Count())
	}
}

// TestRunHelper verifies the Run convenience function.
func TestRunHelper(t *testing.T) {
	var executed int32
	tasks := make([]func() error, 10)
	for i := range tasks {
		tasks[i] = func() error {
			atomic.AddInt32(&executed, 1)
			return nil
		}
	}
	errs := Run(context.Background(), 3, tasks)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %d", len(errs))
	}
	if executed != 10 {
		t.Fatalf("expected 10 executed, got %d", executed)
	}
}

// TestRunnerWorkersClampedTo1 verifies that workers <= 0 defaults to 1.
func TestRunnerWorkersClampedTo1(t *testing.T) {
	r := New(0)
	if cap(r.sem) != 1 {
		t.Fatalf("expected semaphore cap 1 for workers=0, got %d", cap(r.sem))
	}
	r = New(-1)
	if cap(r.sem) != 1 {
		t.Fatalf("expected semaphore cap 1 for workers=-1, got %d", cap(r.sem))
	}
}

// TestRunnerSingleFailureDoesNotBlockOthers verifies that a failing task
// doesn't prevent subsequent tasks from running.
func TestRunnerSingleFailureDoesNotBlockOthers(t *testing.T) {
	var completed int32
	r := New(1)
	for i := 0; i < 5; i++ {
		idx := i
		r.Submit(context.Background(), func() error {
			if idx == 2 {
				return errors.New("fail")
			}
			atomic.AddInt32(&completed, 1)
			return nil
		})
	}
	errs := r.Wait()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if completed != 4 {
		t.Fatalf("expected 4 successful tasks, got %d", completed)
	}
}
