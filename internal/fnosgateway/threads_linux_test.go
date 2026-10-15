//go:build linux && !cgo

package fnosgateway

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestAllThreadsNoNewPrivileges(t *testing.T) {
	if os.Getenv("NDG_NNP_TEST_CHILD") != "1" {
		c := exec.Command(os.Args[0], "-test.run=^TestAllThreadsNoNewPrivileges$")
		c.Env = append(os.Environ(), "NDG_NNP_TEST_CHILD=1")
		if out, e := c.CombinedOutput(); e != nil {
			t.Fatalf("NNP child failed: %v %s", e, out)
		}
		return
	}
	var ready sync.WaitGroup
	ready.Add(8)
	release := make(chan struct{})
	for range 8 {
		go func() { runtime.LockOSThread(); defer runtime.UnlockOSThread(); ready.Done(); <-release }()
	}
	ready.Wait()
	defer close(release)
	if e := LockPrivileges(); e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadDir("/proc/self/task")
	if e != nil || len(before) == 0 || len(before) > 128 {
		t.Fatal("cannot establish bounded thread baseline")
	}
	baseline := make(map[string]bool, len(before))
	for _, tid := range before {
		baseline[tid.Name()] = true
	}
	// Each locked worker remains alive. More workers than existing threads
	// forces at least one newly created OS thread rather than reusing an M.
	ready.Add(len(before) + 1)
	for range len(before) + 1 {
		go func() { runtime.LockOSThread(); defer runtime.UnlockOSThread(); ready.Done(); <-release }()
	}
	ready.Wait()
	tasks, e := os.ReadDir("/proc/self/task")
	if e != nil || len(tasks) < 8 {
		t.Fatal("thread setup failed")
	}
	newThread := false
	for _, tid := range tasks {
		if !baseline[tid.Name()] {
			newThread = true
		}
		b, e := os.ReadFile("/proc/self/task/" + tid.Name() + "/status")
		if e != nil {
			t.Fatal(e)
		}
		found := false
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "NoNewPrivs:") {
				found = strings.TrimSpace(strings.TrimPrefix(line, "NoNewPrivs:")) == "1"
			}
		}
		if !found {
			t.Fatal("thread did not inherit NoNewPrivs")
		}
	}
	if !newThread {
		t.Fatal("no new OS thread was observed after the privilege barrier")
	}
}
