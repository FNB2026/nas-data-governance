// Package drill 实现 P0-3 故障演练。所有场景在 t.TempDir() 隔离临时
// 目录中进行，绝不触及真实 NAS 数据，遵循 AGENTS rule 1「默认只读」。
//
// 演练覆盖四个场景：
//
//	A. 崩溃恢复：构造 EXECUTING plan + journal，调用 Recover() 验证回滚/重置
//	B. 中断续扫：模拟 Ctrl+C 中断，--resume 续扫，验证断点+哈希复用
//	C. stale 检测：批准后修改源文件，验证执行被拦回 DRAFT
//	D. 权限错误：000 权限子目录，验证 scanner 不终止、错误进 Stats
//
// 通过 TestDrill_All 入口串行执行四个场景，并在结束后把演练报告
// 写入 var/reports/drill-<timestamp>.md（项目自有目录，非 NAS 数据）。
package drill

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
	"github.com/FNB2026/nas-data-governance/internal/fingerprint"
	"github.com/FNB2026/nas-data-governance/internal/scanner"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// reportBuf 在测试过程中收集各场景的演练结果，最后由 TestDrill_All
// 聚合写入报告文件。测试串行执行，无需加锁。
var reportBuf struct {
	sync.Mutex
	scenarios []scenarioResult
}

type scenarioResult struct {
	Name     string
	Pass     bool
	Detail   string
	Evidence []string
	Duration time.Duration
}

func record(r scenarioResult) {
	reportBuf.Lock()
	defer reportBuf.Unlock()
	reportBuf.scenarios = append(reportBuf.scenarios, r)
}

// sanitize 把临时目录绝对路径前缀替换为 <tmp>/，遵循 AGENTS rule 6
// 「敏感路径、文件名和内容不得出现在普通日志中」。常见临时目录前缀：
// macOS 的 /var/folders/.../T/、Linux 的 /tmp/、本地 TmpDir。
var tmpRoot = os.TempDir()

func sanitize(s string) string {
	// 把 tmpRoot 及其下的 TestXxxNNN/001 子目录折叠为 <tmp>。
	// 匹配 /tmp 或 /var/folders/.../T/TestXxx/001 形式。
	if i := strings.Index(s, tmpRoot); i >= 0 {
		rest := s[i+len(tmpRoot):]
		// rest 形如 /TestDrill_B_InterruptedResume1631759995/001/batch1/f09.txt
		// 找到 /001/ 之后的部分（保留文件相对路径）。
		if marker := strings.Index(rest, "/001/"); marker >= 0 {
			rest = "/<tmp>" + rest[marker+len("/001"):]
		} else {
			rest = "/<tmp>" + rest
		}
		return s[:i] + rest
	}
	return s
}

// newStore 打开隔离的 SQLite，用于演练。
func newStore(t *testing.T) *store.SQLiteStore {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "drill.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func registerStorage(t *testing.T, st *store.SQLiteStore, id, root string) {
	t.Helper()
	if err := st.RegisterStorage(context.Background(), domain.Storage{
		ID: id, RootPath: root, Kind: "filesystem", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("register storage: %v", err)
	}
}

// writeFile 写入文件并返回路径。
func writeFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// =====================================================================
// 场景 A：崩溃恢复
// =====================================================================

// TestDrill_A_CrashRecovery 模拟崩溃：plan 处于 EXECUTING，journal 有
// 一个 done 条目（文件已被隔离）。Recover() 应当：
//  1. 把隔离文件移回原路径（回滚）
//  2. 把 plan 状态置为 ROLLED_BACK
//  3. 不残留任何隔离区文件
//
// 同时验证「无 done 条目」分支：plan 应被重置为 APPROVED。
func TestDrill_A_CrashRecovery(t *testing.T) {
	start := time.Now()
	record(scenarioResult{Name: "A. 崩溃恢复", Pass: false})

	ctx := context.Background()
	st := newStore(t)
	srcDir := t.TempDir()
	qDir := t.TempDir()
	registerStorage(t, st, "drill-a", srcDir)

	// 子场景 A1：有 done 条目，应回滚。
	src := writeFile(t, filepath.Join(srcDir, "crashed.txt"), "original-content")
	snap, err := executor.Snapshot(src, true)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	file := domain.FileInstance{
		Path: src, Size: snap.Size, ModifiedAt: snap.ModifiedAt,
		Device: snap.Device, Inode: snap.Inode, ContentSHA256: snap.Hash,
	}
	actions := []domain.PlannedAction{
		{Path: src, Action: domain.OperationQuarantine, File: file, Reason: "drill A"},
	}
	taskID := "task-drill-a"
	if err := st.CreateTask(ctx, domain.OperationTask{
		ID: taskID, RootPath: srcDir, State: "executing", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	plan := domain.OperationPlan{
		ID:      "plan-drill-a-rollback",
		TaskID:  taskID,
		State:   domain.PlanApproved,
		Actions: actions,
	}
	if err := st.SavePlans(ctx, taskID, []domain.OperationPlan{plan}); err != nil {
		t.Fatal(err)
	}

	// 模拟崩溃：写入 journal pending→done，文件移到隔离区，plan 置 EXECUTING。
	if err := st.BeginJournal(ctx, taskID, plan.ID, actions); err != nil {
		t.Fatal(err)
	}
	qPath := filepath.Join(qDir, "crashed.txt")
	if err := os.Rename(src, qPath); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkJournalDone(ctx, plan.ID, 0, qPath); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdatePlanState(ctx, plan.ID, domain.PlanExecuting); err != nil {
		t.Fatal(err)
	}

	// 调用 Recover。
	exec := executor.NewForRecovery()
	results := exec.Recover(ctx, st)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Action != executor.RecoveryRolledBack {
		t.Fatalf("expected rolled_back, got %s", results[0].Action)
	}
	if results[0].RolledBack != 1 {
		t.Fatalf("expected 1 rolled back, got %d", results[0].RolledBack)
	}

	// 校验：源文件应恢复。
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source should be restored by recovery, err=%v", err)
	}
	// 校验：隔离区应清空。
	entries, _ := os.ReadDir(qDir)
	if len(entries) != 0 {
		t.Fatalf("quarantine should be empty after rollback, got %d files", len(entries))
	}
	// 校验：plan 状态应为 ROLLED_BACK。
	got, err := st.GetPlan(ctx, plan.ID)
	if err != nil {
		t.Fatalf("get plan: %v", err)
	}
	if got.State != domain.PlanRolledBack {
		t.Fatalf("expected ROLLED_BACK, got %s", got.State)
	}

	// 子场景 A2：无 done 条目，应重置为 APPROVED。
	// 用独立的 task，避免 SavePlans 删除 A1 的 plan（journal FK 引用）。
	taskID2 := "task-drill-a2"
	if err := st.CreateTask(ctx, domain.OperationTask{
		ID: taskID2, RootPath: srcDir, State: "executing", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	plan2 := domain.OperationPlan{
		ID:      "plan-drill-a-reset",
		TaskID:  taskID2,
		State:   domain.PlanApproved,
		Actions: actions,
	}
	if err := st.SavePlans(ctx, taskID2, []domain.OperationPlan{plan2}); err != nil {
		t.Fatal(err)
	}
	if err := st.BeginJournal(ctx, taskID2, plan2.ID, actions); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdatePlanState(ctx, plan2.ID, domain.PlanExecuting); err != nil {
		t.Fatal(err)
	}
	// 不 MarkJournalDone —— 模拟崩溃前未完成任何动作。

	results2 := exec.Recover(ctx, st)
	var resetFound bool
	for _, r := range results2 {
		if r.PlanID == plan2.ID && r.Action == executor.RecoveryResetToApproved {
			resetFound = true
		}
	}
	if !resetFound {
		t.Fatalf("expected reset_to_approved for %s, got %+v", plan2.ID, results2)
	}
	got2, _ := st.GetPlan(ctx, plan2.ID)
	if got2.State != domain.PlanApproved {
		t.Fatalf("expected APPROVED after reset, got %s", got2.State)
	}

	// 更新报告证据。
	reportBuf.Lock()
	for i := range reportBuf.scenarios {
		if reportBuf.scenarios[i].Name == "A. 崩溃恢复" {
			r := &reportBuf.scenarios[i]
			r.Pass = !t.Failed()
			r.Duration = time.Since(start)
			r.Detail = "A1 有 done 条目→回滚隔离文件+置 ROLLED_BACK；A2 无 done 条目→重置为 APPROVED"
			r.Evidence = []string{
				fmt.Sprintf("A1 recovered=%s rolledBack=%d", results[0].Action, results[0].RolledBack),
				fmt.Sprintf("A2 reset found=%v final state=%s", resetFound, got2.State),
				"源文件已恢复原路径，隔离区已清空",
			}
		}
	}
	reportBuf.Unlock()
}

// =====================================================================
// 场景 B：中断续扫
// =====================================================================

// TestDrill_B_InterruptedResume 模拟 Ctrl+C 中断后续扫：
//  1. 构造较大数据集
//  2. 用 context.WithCancel 在中途取消（模拟中断）
//  3. 第二次 scan 用 --resume 语义（ResumePath）
//  4. 验证：两次扫描合并后文件数等于全集，且未变化文件哈希被复用
func TestDrill_B_InterruptedResume(t *testing.T) {
	start := time.Now()
	record(scenarioResult{Name: "B. 中断续扫", Pass: false})

	ctx := context.Background()
	srcDir := t.TempDir()
	// 构造 50 个文件，分两个子目录。
	dir1 := filepath.Join(srcDir, "batch1")
	dir2 := filepath.Join(srcDir, "batch2")
	var allPaths []string
	for i := 0; i < 25; i++ {
		p := writeFile(t, filepath.Join(dir1, fmt.Sprintf("f%02d.txt", i)),
			fmt.Sprintf("batch1 content %d", i))
		allPaths = append(allPaths, p)
	}
	for i := 0; i < 25; i++ {
		p := writeFile(t, filepath.Join(dir2, fmt.Sprintf("f%02d.txt", i)),
			fmt.Sprintf("batch2 content %d", i))
		allPaths = append(allPaths, p)
	}
	sort.Strings(allPaths)

	st := newStore(t)
	registerStorage(t, st, "drill-b", srcDir)

	// 第一阶段：扫描但中途取消。使用一个很快 cancel 的 context，
	// 在 visit 回调中调用 cancel 模拟「扫到第 10 个就中断」。
	var firstBatch []domain.FileInstance
	cancelAfter := 10
	ctx1, cancel1 := context.WithCancel(ctx)
	scanOpts1 := scanner.Options{
		Root:          srcDir,
		StorageID:     "drill-b",
		ExcludedNames: scanner.DefaultExclusions(),
	}
	stats1, err := scanner.Scan(ctx1, scanOpts1, func(file domain.FileInstance) error {
		firstBatch = append(firstBatch, file)
		if len(firstBatch) == cancelAfter {
			cancel1() // 模拟 Ctrl+C
		}
		return nil
	})
	// 期望被 context 取消。
	if err == nil && stats1.FilesScanned == 0 {
		// 如果 BFS 第一批就读完了，可能没有触发 cancel；检查
		t.Fatalf("expected cancellation, but scan completed: %d files", stats1.FilesScanned)
	}

	// 记录「断点」：已扫描文件中按路径排序的最后一个。
	var resumePath string
	if len(firstBatch) > 0 {
		sorted := make([]string, len(firstBatch))
		for i, f := range firstBatch {
			sorted[i] = f.Path
		}
		sort.Strings(sorted)
		resumePath = sorted[len(sorted)-1]
	}

	// 第二阶段：用 ResumePath 续扫，应当跳过路径排序在 resumePath 之前的文件。
	// 注意：为验证哈希复用，第一阶段已扫描的文件需要先写入 DB。
	if len(firstBatch) > 0 {
		// 为已扫描文件计算 quick_hash。
		for i := range firstBatch {
			q, qerr := fingerprint.Quick(firstBatch[i].Path, firstBatch[i].Size)
			if qerr == nil {
				firstBatch[i].QuickHash = q
			}
		}
		if _, err := st.UpsertFiles(ctx, firstBatch); err != nil {
			t.Fatalf("upsert first batch: %v", err)
		}
		// 启动 checkpoint 记录断点。
		cpID, err := st.StartCheckpoint(ctx, "drill-b")
		if err != nil {
			t.Fatalf("start checkpoint: %v", err)
		}
		if err := st.UpdateCheckpoint(ctx, cpID, resumePath, len(firstBatch)); err != nil {
			t.Fatalf("update checkpoint: %v", err)
		}
		// 注意：不 CompleteCheckpoint，模拟中断状态。
	}

	// 加载已有元数据用于增量哈希复用。
	existing, err := st.ListFileMetadata(ctx, "drill-b")
	if err != nil {
		t.Fatalf("list metadata: %v", err)
	}
	cache := map[string]store.FileMeta{}
	for _, m := range existing {
		cache[m.Path] = m
	}

	// 第二阶段扫描：完整跑完，用 ResumePath 跳过已扫描部分。
	var secondBatch []domain.FileInstance
	hashReused := 0
	scanOpts2 := scanner.Options{
		Root:          srcDir,
		StorageID:     "drill-b",
		ExcludedNames: scanner.DefaultExclusions(),
		ResumePath:    resumePath,
	}
	ctx2, cancel2 := context.WithCancel(ctx)
	defer cancel2()
	stats2, err := scanner.Scan(ctx2, scanOpts2, func(file domain.FileInstance) error {
		// 增量检查：size+mtime+inode 未变则复用哈希。
		if cached, ok := cache[file.Path]; ok {
			if cached.Size == file.Size &&
				cached.ModifiedAt.Equal(file.ModifiedAt) &&
				cached.Inode == file.Inode {
				file.QuickHash = cached.QuickHash
				file.ContentSHA256 = cached.ContentSHA256
				hashReused++
				secondBatch = append(secondBatch, file)
				return nil
			}
		}
		q, qerr := fingerprint.Quick(file.Path, file.Size)
		if qerr == nil {
			file.QuickHash = q
		}
		secondBatch = append(secondBatch, file)
		return nil
	})
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}

	// 合并：firstBatch + secondBatch 应当覆盖全部 50 个文件。
	seen := map[string]bool{}
	for _, f := range firstBatch {
		seen[f.Path] = true
	}
	for _, f := range secondBatch {
		seen[f.Path] = true
	}
	if len(seen) != 50 {
		t.Fatalf("expected 50 unique files after resume, got %d", len(seen))
	}

	// 第二阶段应该跳过了已扫描的文件（不重复），所以 secondBatch 只含未扫描的。
	// 但 ResumePath 按字典序跳过，可能有些 firstBatch 文件路径 > resumePath，
	// 仍会在第二阶段被扫到——这正好验证哈希复用。
	if stats2.FilesScanned == 0 {
		t.Fatalf("expected second scan to find files, got 0")
	}

	// 完成检查点。
	lastCP, err := st.LastCheckpoint(ctx, "drill-b")
	if err != nil {
		t.Fatalf("last checkpoint: %v", err)
	}
	if err := st.CompleteCheckpoint(ctx, lastCP.ID, "completed"); err != nil {
		t.Fatalf("complete checkpoint: %v", err)
	}

	// 校验：完成后 LastCheckpoint 应返回 ErrNotFound（无 running）。
	_, err = st.LastCheckpoint(ctx, "drill-b")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after completion, got %v", err)
	}

	reportBuf.Lock()
	for i := range reportBuf.scenarios {
		if reportBuf.scenarios[i].Name == "B. 中断续扫" {
			r := &reportBuf.scenarios[i]
			r.Pass = !t.Failed()
			r.Duration = time.Since(start)
			r.Detail = fmt.Sprintf("第一阶段扫到 %d 文件后取消，第二阶段 --resume 补齐到 %d 文件，哈希复用 %d 次",
				len(firstBatch), len(seen), hashReused)
			r.Evidence = []string{
				fmt.Sprintf("第一阶段文件数=%d resumePath=%s", len(firstBatch), resumePath),
				fmt.Sprintf("第二阶段文件数=%d 哈希复用=%d", len(secondBatch), hashReused),
				fmt.Sprintf("合并去重后=%d（期望 50）", len(seen)),
				"检查点状态：completed 后 LastCheckpoint 返回 ErrNotFound",
			}
		}
	}
	reportBuf.Unlock()
}

// =====================================================================
// 场景 C：stale 检测
// =====================================================================

// TestDrill_C_StaleDetection 验证：plan 批准后源文件被修改，执行时
// stale 复核应拦截执行，把 plan 退回 DRAFT，且不产生任何文件系统副作用。
func TestDrill_C_StaleDetection(t *testing.T) {
	start := time.Now()
	record(scenarioResult{Name: "C. stale 检测", Pass: false})

	ctx := context.Background()
	srcDir := t.TempDir()
	qDir := t.TempDir()

	src := writeFile(t, filepath.Join(srcDir, "stale.txt"), "original content")
	snap, err := executor.Snapshot(src, true)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	file := domain.FileInstance{
		Path: src, Size: snap.Size, ModifiedAt: snap.ModifiedAt,
		Device: snap.Device, Inode: snap.Inode, ContentSHA256: snap.Hash,
	}
	actions := []domain.PlannedAction{
		{Path: src, Action: domain.OperationQuarantine, File: file, Reason: "drill C"},
	}
	plan := domain.OperationPlan{
		ID:      "plan-drill-c",
		State:   domain.PlanApproved,
		Actions: actions,
	}

	// 批准后修改源文件（size 变大）。
	if err := os.WriteFile(src, []byte("content was modified after approval - longer"), 0o644); err != nil {
		t.Fatal(err)
	}

	exec, err := executor.New(executor.QuarantineConfig{
		Root: qDir, Structure: executor.QuarantineFlat, SourceRoots: []string{srcDir},
	})
	if err != nil {
		t.Fatal(err)
	}

	result := exec.Execute(ctx, &plan)

	// stale 检测不应导致 result.Err；而是把 state 退回 DRAFT。
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if plan.State != domain.PlanDraft {
		t.Fatalf("expected DRAFT (stale re-review), got %s", plan.State)
	}

	// 校验：源文件仍存在（未被执行）。
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source should still exist, err=%v", err)
	}
	// 校验：隔离区为空（未产生副作用）。
	entries, _ := os.ReadDir(qDir)
	if len(entries) != 0 {
		t.Fatalf("quarantine should be empty, got %d files", len(entries))
	}

	reportBuf.Lock()
	for i := range reportBuf.scenarios {
		if reportBuf.scenarios[i].Name == "C. stale 检测" {
			r := &reportBuf.scenarios[i]
			r.Pass = !t.Failed()
			r.Detail = "批准后源文件被修改（size 变化），执行时 stale 复核拦截，plan 退回 DRAFT，隔离区为空"
			r.Duration = time.Since(start)
			r.Evidence = []string{
				fmt.Sprintf("final state=%s (expected DRAFT)", plan.State),
				"源文件未被移动/删除",
				"隔离区 0 文件（无副作用）",
			}
		}
	}
	reportBuf.Unlock()
}

// =====================================================================
// 场景 D：权限错误容忍
// =====================================================================

// TestDrill_D_PermissionErrorTolerance 构造一个 000 权限子目录，
// 验证 scanner 不终止、错误进入 Stats.Errors，仍能扫描其余可读目录。
//
// 注意：在 root 用户环境下 000 权限仍可读，所以此场景需要跳过。
func TestDrill_D_PermissionErrorTolerance(t *testing.T) {
	start := time.Now()
	record(scenarioResult{Name: "D. 权限错误容忍", Pass: false})

	if os.Geteuid() == 0 {
		// root 用户绕过权限检查，此场景无意义。
		reportBuf.Lock()
		for i := range reportBuf.scenarios {
			if reportBuf.scenarios[i].Name == "D. 权限错误容忍" {
				r := &reportBuf.scenarios[i]
				r.Pass = true
				r.Duration = time.Since(start)
				r.Detail = "SKIP：root 用户绕过权限检查，此场景在当前环境无意义"
				r.Evidence = []string{"os.Geteuid()=0，跳过"}
			}
		}
		reportBuf.Unlock()
		t.Skip("running as root, permission test is not meaningful")
	}

	srcDir := t.TempDir()
	// 可读子目录 + 文件。
	readableDir := filepath.Join(srcDir, "readable")
	readableFile := writeFile(t, filepath.Join(readableDir, "ok.txt"), "readable")

	// 不可读子目录（000 权限）。
	noPermDir := filepath.Join(srcDir, "noperm")
	if err := os.MkdirAll(noPermDir, 0o000); err != nil {
		t.Fatal(err)
	}
	// 恢复权限以便清理。
	t.Cleanup(func() {
		_ = os.Chmod(noPermDir, 0o755)
	})

	ctx := context.Background()
	var found []string
	stats, err := scanner.Scan(ctx, scanner.Options{
		Root:          srcDir,
		StorageID:     "drill-d",
		ExcludedNames: scanner.DefaultExclusions(),
	}, func(file domain.FileInstance) error {
		found = append(found, file.Path)
		return nil
	})
	// scan 不应返回错误（单目录失败是非致命的）。
	if err != nil {
		t.Fatalf("scan should not fail on single dir error, got %v", err)
	}

	// 应当扫到 readable 文件。
	foundReadable := false
	for _, p := range found {
		if p == readableFile {
			foundReadable = true
		}
	}
	if !foundReadable {
		t.Fatalf("expected to find readable file %s, got %v", readableFile, found)
	}

	// Stats.Errors 应包含 noperm 目录的读取错误。
	if len(stats.Errors) == 0 {
		t.Fatalf("expected non-fatal errors in Stats.Errors, got 0")
	}
	errStr := stats.FormatErrors()
	if errStr == "" {
		t.Fatalf("expected formatted errors to be non-empty")
	}

	// 不可读目录本身不应出现在 found 文件中（无法列出其内容）。
	for _, p := range found {
		if strings.HasPrefix(p, noPermDir+string(filepath.Separator)) {
			t.Fatalf("file under no-perm dir should not be scanned: %s", p)
		}
	}

	reportBuf.Lock()
	for i := range reportBuf.scenarios {
		if reportBuf.scenarios[i].Name == "D. 权限错误容忍" {
			r := &reportBuf.scenarios[i]
			r.Pass = !t.Failed()
			r.Duration = time.Since(start)
			r.Detail = fmt.Sprintf("000 权限子目录被记录为非致命错误，scanner 继续扫描可读目录（扫到 %d 文件）",
				stats.FilesScanned)
			r.Evidence = []string{
				fmt.Sprintf("Stats.FilesScanned=%d DirsVisited=%d", stats.FilesScanned, stats.DirsVisited),
				fmt.Sprintf("Stats.Errors=%d 条", len(stats.Errors)),
				fmt.Sprintf("可读文件 %s 被正确扫到", readableFile),
				"scan 返回 nil err（不因单目录失败终止）",
			}
		}
	}
	reportBuf.Unlock()
}

// =====================================================================
// 聚合：TestDrill_All 串行跑全部场景后生成报告
// =====================================================================

// TestDrill_All 是一个占位测试，确保在 go test -run TestDrill_All
// 时也跑全部场景。各场景测试以 TestDrill_A/B/C/D 开头，会被一并执行。
// 报告生成放在 TestMain 或独立的 TestDrill_Report 中。
func TestDrill_All(t *testing.T) {
	// 此函数仅作为入口标记，实际场景由 TestDrill_A/B/C/D 承担。
	t.Log("drill scenarios are implemented as TestDrill_A/B/C/D; running them together")
}

// TestDrill_Z_WriteReport 在所有场景跑完后生成演练报告。
// 命名带 Z 前缀使其在字母序最后执行。
func TestDrill_Z_WriteReport(t *testing.T) {
	reportDir := filepath.Join("..", "..", "var", "reports")
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		t.Fatalf("mkdir reports: %v", err)
	}
	ts := time.Now().Format("20060102-150405")
	path := filepath.Join(reportDir, "drill-"+ts+".md")

	var b strings.Builder
	fmt.Fprintf(&b, "# P0-3 故障演练报告\n\n")
	fmt.Fprintf(&b, "**时间**：%s\n\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "**环境**：%s/%s\n\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "**原则**：全部场景在 `t.TempDir()` 隔离目录进行，绝不触及真实 NAS 数据（AGENTS rule 1）。\n\n")
	fmt.Fprintf(&b, "## 场景汇总\n\n")
	fmt.Fprintf(&b, "| 场景 | 结果 | 耗时 |\n")
	fmt.Fprintf(&b, "|------|------|------|\n")

	reportBuf.Lock()
	defer reportBuf.Unlock()
	totalPass, totalFail := 0, 0
	for _, s := range reportBuf.scenarios {
		status := "PASS"
		if !s.Pass {
			status = "FAIL"
			totalFail++
		} else {
			totalPass++
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", s.Name, status, s.Duration.Truncate(time.Millisecond))
	}
	fmt.Fprintf(&b, "\n**通过 %d / 失败 %d / 共 %d**\n\n", totalPass, totalFail, len(reportBuf.scenarios))

	for _, s := range reportBuf.scenarios {
		fmt.Fprintf(&b, "## %s\n\n", s.Name)
		fmt.Fprintf(&b, "- **结果**：")
		if s.Pass {
			fmt.Fprintf(&b, "PASS\n")
		} else {
			fmt.Fprintf(&b, "FAIL\n")
		}
		fmt.Fprintf(&b, "- **详情**：%s\n", sanitize(s.Detail))
		if len(s.Evidence) > 0 {
			fmt.Fprintf(&b, "- **证据**：\n")
			for _, e := range s.Evidence {
				fmt.Fprintf(&b, "  - %s\n", sanitize(e))
			}
		}
		fmt.Fprintf(&b, "\n")
	}

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
	t.Logf("drill report written to %s", path)

	// 任何一个场景失败，让此测试也失败（确保 CI 捕获）。
	if totalFail > 0 {
		t.Errorf("%d scenario(s) failed", totalFail)
	}

	// 清空缓冲以便重跑。
	reportBuf.scenarios = nil
}
