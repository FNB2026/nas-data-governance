//go:build linux

// quarantine-candidate-fixture seeds only a new CI-owned synthetic project.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/dircontext"
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

func check(err error) {
	if err != nil {
		panic("synthetic fixture preparation failed")
	}
}

func main() {
	if os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("RUNNER_ENVIRONMENT") != "github-hosted" || len(os.Args) != 2 {
		panic("disposable GitHub runner required")
	}
	root := filepath.Clean(os.Args[1])
	if filepath.Dir(root) != "/tmp" || !strings.HasPrefix(filepath.Base(root), "ndg-quarantine-ci-") {
		panic("new synthetic fixture root required")
	}
	db := filepath.Join(root, "state", "project.db")
	if _, err := os.Lstat(db); !os.IsNotExist(err) {
		panic("new synthetic database required")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, db)
	check(err)
	defer st.Close()
	raw := []byte("synthetic delete fixture\n")
	sum := sha256.Sum256(raw)
	target := filepath.Join(root, "quarantine", "synthetic.txt")
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	check(err)
	_, err = f.Write(raw)
	check(err)
	check(f.Close())
	now := time.Now().UTC()
	task := domain.OperationTask{ID: "synthetic-task", RootPath: "/quarantine", State: "completed", CreatedAt: now}
	plan := domain.OperationPlan{ID: "synthetic-quarantine", State: domain.PlanVerified, Actions: []domain.PlannedAction{{
		Path: "/quarantine/original", Action: domain.OperationQuarantine, TargetPath: "/quarantine/synthetic.txt",
		File: domain.FileInstance{Size: int64(len(raw)), ContentSHA256: hex.EncodeToString(sum[:])},
	}}}
	check(st.CreateTask(ctx, task))
	check(st.SavePlans(ctx, task.ID, []domain.OperationPlan{plan}))
	check(st.BeginJournal(ctx, task.ID, plan.ID, plan.Actions))
	check(st.MarkJournalDone(ctx, plan.ID, 0, "/quarantine/synthetic.txt"))
	items, err := st.RegisterQuarantinesFromJournal(ctx, plan.ID, now.Add(-31*24*time.Hour), now.Add(-24*time.Hour))
	check(err)
	if len(items) != 1 {
		panic("synthetic item not registered")
	}
	fmt.Println(items[0].ID)
	seedSource(ctx, st, root)
}

func seedSource(ctx context.Context, st *store.SQLiteStore, root string) {
	const storageID = "synthetic-local"
	check(st.RegisterStorage(ctx, domain.Storage{ID: storageID, Kind: "local", RootPath: "/source", CreatedAt: time.Now().UTC()}))
	gen, err := st.StartCheckpoint(ctx, storageID)
	check(err)
	var files []domain.FileInstance
	for _, group := range []string{"one", "two"} {
		for _, name := range []string{group + "-remove.txt", group + "-keeper.txt"} {
			raw := []byte("synthetic source group " + group + "\n")
			sum := sha256.Sum256(raw)
			p := filepath.Join(root, "source", name)
			f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			check(err)
			_, err = f.Write(raw)
			check(err)
			info, err := f.Stat()
			check(err)
			check(f.Close())
			identity := info.Sys().(*syscall.Stat_t)
			files = append(files, domain.FileInstance{
				StorageID: storageID, Path: filepath.Join("/source", name), Name: name, Size: info.Size(), Mode: uint32(info.Mode()),
				Device: uint64(identity.Dev), Inode: identity.Ino, Physical: domain.PhysicalIdentity{Reliable: true, LinkCount: 1},
				ContentSHA256: hex.EncodeToString(sum[:]), ModifiedAt: info.ModTime(), DiscoveredAt: time.Now().UTC(),
			})
		}
	}
	ids, err := st.UpsertFiles(ctx, files)
	check(err)
	for _, id := range ids {
		check(st.SaveContext(ctx, id, domain.DirectoryContext{Role: domain.RoleTemporary, PrivacyLevel: "normal"}, dircontext.RuleVersion()))
		check(st.SaveFormat(ctx, id, domain.FormatInfo{Format: "text", Category: domain.CategoryDocument}))
		fmt.Println(id)
	}
	check(st.CompleteCheckpoint(ctx, gen, "completed"))
}
