package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/dircontext"
	"github.com/FNB2026/nas-data-governance/internal/domain"
	idx "github.com/FNB2026/nas-data-governance/internal/index"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

func runImportIndex(args []string) error {
	fs := flag.NewFlagSet("import-index", flag.ContinueOnError)
	indexPath := fs.String("index", "", "JSONL index produced by scan (required)")
	dbPath := fs.String("db", "", "SQLite database destination (required)")
	batchSize := fs.Int("batch-size", 1000, "records per SQLite transaction")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *indexPath == "" || *dbPath == "" {
		return fmt.Errorf("--index and --db are required")
	}
	if *batchSize < 1 || *batchSize > 10000 {
		return fmt.Errorf("--batch-size must be between 1 and 10000")
	}

	// Validate the complete stream before opening the destination database.
	// This prevents malformed trailing input from leaving a partial import.
	roots := map[string]string{}
	count := 0
	if err := idx.Walk(*indexPath, func(f domain.FileInstance) error {
		if f.StorageID == "" || f.Path == "" {
			return fmt.Errorf("storage_id and path are required")
		}
		dir := filepath.Dir(f.Path)
		if root, ok := roots[f.StorageID]; ok {
			roots[f.StorageID] = commonImportRoot(root, dir)
		} else {
			roots[f.StorageID] = dir
		}
		count++
		return nil
	}); err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("index contains no records")
	}

	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	for storageID, root := range roots {
		if err := st.RegisterStorage(ctx, domain.Storage{ID: storageID, RootPath: root, Kind: "imported-index", CreatedAt: time.Now().UTC()}); err != nil {
			return err
		}
	}

	batch := make([]domain.FileInstance, 0, *batchSize)
	imported := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		ids, err := st.UpsertFiles(ctx, batch)
		if err != nil {
			return err
		}
		contexts := make([]domain.DirectoryContext, len(batch))
		for i := range batch {
			contexts[i] = dircontext.Classify(batch[i].Path)
		}
		if err := st.SaveContexts(ctx, ids, contexts, dircontext.RuleVersion()); err != nil {
			return err
		}
		imported += len(batch)
		batch = batch[:0]
		return nil
	}
	if err := idx.Walk(*indexPath, func(f domain.FileInstance) error {
		batch = append(batch, f)
		if len(batch) >= *batchSize {
			return flush()
		}
		return nil
	}); err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	fmt.Printf("imported %d files from validated index into %s (%d storage(s); no NAS access)\n", imported, *dbPath, len(roots))
	return nil
}

func commonImportRoot(a, b string) string {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	for {
		if rel, err := filepath.Rel(a, b); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return a
		}
		parent := filepath.Dir(a)
		if parent == a {
			return parent
		}
		a = parent
	}
}
