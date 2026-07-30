package project

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentProjectCreationClaimsUniqueDirectoriesAndPreservesRecent(t *testing.T) {
	base := filepath.Join(t.TempDir(), "app-support")
	source := filepath.Join(t.TempDir(), "产业资料库")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	baseFn := func() (string, error) { return base, nil }

	const count = 8
	created := make(chan *CreatedProject, count)
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Separate service values model independent desktop processes;
			// coordination must come from filesystem primitives, not a mutex.
			svc := NewService(Options{SupportBase: baseFn})
			item, err := svc.CreateFromSource(context.Background(), CreateInput{Name: "产业资料库", SourcePath: source})
			if err != nil {
				errs <- err
				return
			}
			if err := svc.RecordRecent(item.Name, item.DatabasePath); err != nil {
				_ = svc.RollbackProjectCreation(item)
				errs <- err
				return
			}
			created <- item
		}()
	}
	wg.Wait()
	close(errs)
	close(created)
	for err := range errs {
		t.Errorf("concurrent operation: %v", err)
	}
	if t.Failed() {
		return
	}

	ids := make(map[string]bool, count)
	paths := make(map[string]bool, count)
	for item := range created {
		ids[item.ProjectID] = true
		paths[item.DatabasePath] = true
		if err := item.Store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	}
	if len(ids) != count || len(paths) != count {
		t.Fatalf("unique IDs=%d paths=%d, want %d each", len(ids), len(paths), count)
	}

	svc := NewService(Options{SupportBase: baseFn})
	recent, err := svc.ListRecent()
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(recent) != count {
		t.Fatalf("recent entries=%d, want %d", len(recent), count)
	}
	for _, entry := range recent {
		if !paths[entry.Path] {
			t.Errorf("unexpected or lost recent path %q", entry.Path)
		}
	}
	temps, err := filepath.Glob(filepath.Join(base, ".recent.json.tmp-*"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(temps) != 0 {
		t.Errorf("temporary manifests left behind: %v", temps)
	}
}

func TestRollbackProjectCreationRemovesOnlyClaimedDirectory(t *testing.T) {
	base := filepath.Join(t.TempDir(), "app-support")
	source := t.TempDir()
	svc := NewService(Options{SupportBase: func() (string, error) { return base, nil }})
	created, err := svc.CreateFromSource(context.Background(), CreateInput{Name: "rollback", SourcePath: source})
	if err != nil {
		t.Fatalf("CreateFromSource: %v", err)
	}
	if err := svc.RollbackProjectCreation(created); err != nil {
		t.Fatalf("RollbackProjectCreation: %v", err)
	}
	if _, err := os.Lstat(created.ProjectDir); !os.IsNotExist(err) {
		t.Fatalf("claimed project directory remains: %v", err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("scan source was modified by rollback: %v", err)
	}
}
