package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
	"github.com/FNB2026/nas-data-governance/internal/format"
	"github.com/FNB2026/nas-data-governance/internal/formatdiag"
	"github.com/FNB2026/nas-data-governance/internal/governancediag"
	idx "github.com/FNB2026/nas-data-governance/internal/index"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

func runNextSteps(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("next-steps requires subcommand: hold-critical|media-tiering|reconcile-coverage|prepare-cold-copy|prepare-extension-renames")
	}
	switch args[0] {
	case "hold-critical":
		return runHoldCritical(args[1:])
	case "media-tiering":
		return runMediaTiering(args[1:])
	case "reconcile-coverage":
		return runReconcileCoverage(args[1:])
	case "prepare-cold-copy":
		return runPrepareColdCopy(args[1:])
	case "prepare-extension-renames":
		return runPrepareExtensionRenames(args[1:])
	default:
		return fmt.Errorf("unknown next-steps subcommand")
	}
}

func runPrepareExtensionRenames(args []string) error {
	fs := flag.NewFlagSet("next-steps prepare-extension-renames", flag.ContinueOnError)
	dbPath := fs.String("db", "", "analysis SQLite database")
	reviewPath := fs.String("review", "", "private format review input")
	sourceRoot := fs.String("source-root", "", "exact source root")
	out := fs.String("out", "./var/extension-rename-draft.json", "private DRAFT RENAME plan")
	taskID := fs.String("task-id", "wave-02-extension-renames", "stable execution wave ID")
	detected := fs.String("detected", "", "optional detected format filter, for example jpeg")
	maxItems := fs.Int("max-items", 100, "maximum candidates in this wave")
	maxBytes := fs.Int64("max-bytes", 512<<20, "maximum total bytes in this wave")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *reviewPath == "" || *sourceRoot == "" {
		return fmt.Errorf("--db, --review and --source-root are required")
	}
	if *taskID == "" || *maxItems < 1 || *maxItems > 1000 || *maxBytes < 1 {
		return fmt.Errorf("task ID and bounded positive wave limits are required")
	}
	sourceAbs, err := filepath.Abs(*sourceRoot)
	if err != nil {
		return err
	}
	if err := validateRealSourceRoot(sourceAbs); err != nil {
		return err
	}
	data, err := os.ReadFile(*reviewPath)
	if err != nil {
		return err
	}
	var review formatdiag.Report
	if err := json.Unmarshal(data, &review); err != nil {
		return fmt.Errorf("decode format review: %w", err)
	}
	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	files, err := st.ListFiles(ctx, "")
	if err != nil {
		return err
	}
	plans, bytes, err := buildExtensionRenamePlans(review.ExtensionMismatches, files, sourceAbs, *taskID, *detected, *maxItems, *maxBytes)
	if err != nil {
		return err
	}
	if err := writePrivateJSON(*out, plans); err != nil {
		return err
	}
	fmt.Printf("prepared %d high-risk DRAFT RENAME plan(s), %d bytes; source files unchanged and execution unauthorized\n", len(plans), bytes)
	return nil
}

const coldReviewTier = "COLD_STORAGE_REVIEW"

func runPrepareColdCopy(args []string) error {
	fs := flag.NewFlagSet("next-steps prepare-cold-copy", flag.ContinueOnError)
	dbPath := fs.String("db", "", "analysis SQLite database")
	tieringPath := fs.String("tiering", "", "private media tiering input")
	sourceRoot := fs.String("source-root", "", "exact read-only source root")
	targetRoot := fs.String("target-root", "", "future managed copy target (must not exist for preparation)")
	out := fs.String("out", "./var/cold-copy-draft.json", "private DRAFT COPY plan")
	taskID := fs.String("task-id", "wave-01-cold-copy", "stable execution wave ID")
	maxItems := fs.Int("max-items", 8, "maximum candidates in this wave")
	maxBytes := fs.Int64("max-bytes", 8<<30, "maximum total bytes in this wave")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *tieringPath == "" || *sourceRoot == "" || *targetRoot == "" {
		return fmt.Errorf("--db, --tiering, --source-root and --target-root are required")
	}
	if *taskID == "" || *maxItems < 1 || *maxItems > 100 || *maxBytes < 1 {
		return fmt.Errorf("task ID and bounded positive wave limits are required")
	}

	sourceAbs, err := filepath.Abs(*sourceRoot)
	if err != nil {
		return err
	}
	targetAbs, err := filepath.Abs(*targetRoot)
	if err != nil {
		return err
	}
	if err := validateColdCopyRoots(sourceAbs, targetAbs); err != nil {
		return err
	}

	data, err := os.ReadFile(*tieringPath)
	if err != nil {
		return err
	}
	var tiering mediaTieringPlan
	if err := json.Unmarshal(data, &tiering); err != nil {
		return fmt.Errorf("decode media tiering: %w", err)
	}
	if tiering.ExecutionAuthorized {
		return fmt.Errorf("tiering input must remain advisory and unauthorized")
	}

	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	files, err := st.ListFiles(ctx, "")
	if err != nil {
		return err
	}
	plans, bytes, err := buildColdCopyPlans(tiering, files, sourceAbs, targetAbs, *taskID, *maxItems, *maxBytes)
	if err != nil {
		return err
	}
	if err := writePrivateJSON(*out, plans); err != nil {
		return err
	}
	fmt.Printf("prepared %d high-risk DRAFT COPY plan(s), %d bytes; source files unchanged and execution unauthorized\n", len(plans), bytes)
	return nil
}

func validateColdCopyRoots(sourceRoot, targetRoot string) error {
	if !filepath.IsAbs(sourceRoot) || !filepath.IsAbs(targetRoot) {
		return fmt.Errorf("source and target roots must be absolute")
	}
	if err := validateRealSourceRoot(sourceRoot); err != nil {
		return err
	}
	if withinPath(sourceRoot, targetRoot) || withinPath(targetRoot, sourceRoot) {
		return fmt.Errorf("source and target roots must not overlap")
	}
	if _, err := os.Lstat(targetRoot); err == nil {
		return fmt.Errorf("target root must not exist during DRAFT preparation")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect target root: %w", err)
	}
	return nil
}

func validateRealSourceRoot(sourceRoot string) error {
	info, err := os.Lstat(sourceRoot)
	if err != nil {
		return fmt.Errorf("inspect source root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("source root must be a real directory")
	}
	return nil
}

func buildExtensionRenamePlans(mismatches []formatdiag.ExtensionMismatch, files []domain.FileInstance, sourceRoot, taskID, detected string, maxItems int, maxBytes int64) ([]domain.OperationPlan, int64, error) {
	active := make(map[string]domain.FileInstance, len(files))
	for _, file := range files {
		active[file.StorageID+"\x00"+filepath.Clean(file.Path)] = file
	}
	candidates := make([]formatdiag.ExtensionMismatch, 0, len(mismatches))
	for _, item := range mismatches {
		if canonicalExtension(item.Detected) == "" || (detected != "" && item.Detected != detected) {
			continue
		}
		candidates = append(candidates, item)
	}
	sort.Slice(candidates, func(i, j int) bool {
		pi, pj := extensionRenamePriority(candidates[i]), extensionRenamePriority(candidates[j])
		if pi != pj {
			return pi < pj
		}
		if candidates[i].Size != candidates[j].Size {
			return candidates[i].Size < candidates[j].Size
		}
		return candidates[i].Path < candidates[j].Path
	})
	plans := make([]domain.OperationPlan, 0, min(maxItems, len(candidates)))
	var total int64
	for _, item := range candidates {
		if len(plans) >= maxItems {
			break
		}
		if !filepath.IsAbs(item.Path) || !withinPath(sourceRoot, item.Path) {
			return nil, 0, fmt.Errorf("extension candidate is outside the approved source root")
		}
		if item.Size > maxBytes-total {
			continue
		}
		file, ok := active[item.StorageID+"\x00"+filepath.Clean(item.Path)]
		if !ok || file.Size != item.Size {
			return nil, 0, fmt.Errorf("extension candidate is not active or changed since analysis")
		}
		info, err := os.Lstat(item.Path)
		if err != nil {
			return nil, 0, fmt.Errorf("inspect extension candidate: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, 0, fmt.Errorf("extension candidate must be a regular non-symlink file")
		}
		current, err := format.Analyze(item.Path)
		if err != nil || current.Format != item.Detected {
			return nil, 0, fmt.Errorf("extension candidate format evidence is stale")
		}
		targetPath := strings.TrimSuffix(filepath.Clean(item.Path), filepath.Ext(item.Path)) + canonicalExtension(item.Detected)
		if targetPath == filepath.Clean(item.Path) {
			return nil, 0, fmt.Errorf("extension candidate already has canonical extension")
		}
		if _, err := os.Lstat(targetPath); err == nil {
			return nil, 0, fmt.Errorf("extension rename target already exists")
		} else if !os.IsNotExist(err) {
			return nil, 0, fmt.Errorf("inspect extension rename target: %w", err)
		}
		snap, err := executor.Snapshot(item.Path, true)
		if err != nil || snap.Size != item.Size {
			return nil, 0, fmt.Errorf("extension candidate changed during DRAFT preparation")
		}
		file.Path, file.Size, file.Mode = filepath.Clean(item.Path), snap.Size, uint32(info.Mode())
		file.ModifiedAt, file.Device, file.Inode, file.IsSymlink, file.ContentSHA256 = snap.ModifiedAt, snap.Device, snap.Inode, false, snap.Hash
		ordinal := len(plans) + 1
		planID := fmt.Sprintf("%s-%03d", taskID, ordinal)
		plans = append(plans, domain.OperationPlan{
			ID: planID, TaskID: taskID, State: domain.PlanDraft, ContentSHA256: snap.Hash, Size: snap.Size, Risk: domain.RiskHigh,
			Actions:  []domain.PlannedAction{{Path: file.Path, Action: domain.OperationRename, TargetPath: targetPath, File: file, Reason: "verified file-header extension correction; rename only"}},
			Evidence: []string{"current header re-detected before DRAFT creation", "full SHA-256 and filesystem identity captured", "target collision check passed", "RENAME only; independent approval required"},
		})
		total += snap.Size
	}
	if len(plans) == 0 {
		return nil, 0, fmt.Errorf("no extension corrections fit the bounded wave")
	}
	return plans, total, nil
}

func canonicalExtension(format string) string {
	switch strings.ToLower(format) {
	case "jpeg":
		return ".jpg"
	case "png":
		return ".png"
	case "gif":
		return ".gif"
	case "bmp":
		return ".bmp"
	case "tiff":
		return ".tiff"
	case "webp":
		return ".webp"
	case "heic":
		return ".heic"
	case "pdf":
		return ".pdf"
	case "mp4":
		return ".mp4"
	case "mov":
		return ".mov"
	case "m4v":
		return ".m4v"
	case "m4a":
		return ".m4a"
	case "mpeg":
		return ".mpeg"
	case "aac":
		return ".aac"
	case "mp3":
		return ".mp3"
	case "doc":
		return ".doc"
	case "xls":
		return ".xls"
	case "ppt":
		return ".ppt"
	case "docx":
		return ".docx"
	case "xlsx":
		return ".xlsx"
	case "pptx":
		return ".pptx"
	default:
		return ""
	}
}

func extensionRenamePriority(item formatdiag.ExtensionMismatch) int {
	if item.Extension == ".pdf" && item.Detected == "jpeg" {
		return 0
	}
	if item.Detected == "jpeg" || item.Detected == "png" || item.Detected == "gif" || item.Detected == "bmp" || item.Detected == "tiff" {
		return 1
	}
	return 2
}

func buildColdCopyPlans(tiering mediaTieringPlan, files []domain.FileInstance, sourceRoot, targetRoot, taskID string, maxItems int, maxBytes int64) ([]domain.OperationPlan, int64, error) {
	active := make(map[string]domain.FileInstance, len(files))
	for _, file := range files {
		active[file.StorageID+"\x00"+filepath.Clean(file.Path)] = file
	}
	candidates := make([]mediaTierItem, 0)
	for _, item := range tiering.Items {
		if item.Tier == coldReviewTier {
			candidates = append(candidates, item)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Size != candidates[j].Size {
			return candidates[i].Size < candidates[j].Size
		}
		return candidates[i].Path < candidates[j].Path
	})

	plans := make([]domain.OperationPlan, 0, min(maxItems, len(candidates)))
	var total int64
	for _, item := range candidates {
		if len(plans) >= maxItems {
			break
		}
		if err := validateColdCandidate(item, sourceRoot); err != nil {
			return nil, 0, err
		}
		if item.Size > maxBytes-total {
			continue
		}
		file, ok := active[item.StorageID+"\x00"+filepath.Clean(item.Path)]
		if !ok {
			return nil, 0, fmt.Errorf("cold candidate is not active in the analysis database")
		}
		info, err := os.Lstat(item.Path)
		if err != nil {
			return nil, 0, fmt.Errorf("inspect cold candidate: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, 0, fmt.Errorf("cold candidate must be a regular non-symlink file")
		}
		snap, err := executor.Snapshot(item.Path, true)
		if err != nil {
			return nil, 0, fmt.Errorf("snapshot cold candidate: %w", err)
		}
		if snap.Size != item.Size || snap.Size != file.Size {
			return nil, 0, fmt.Errorf("cold candidate changed since analysis")
		}
		file.Path = filepath.Clean(item.Path)
		file.Size = snap.Size
		file.Mode = uint32(info.Mode())
		file.ModifiedAt = snap.ModifiedAt
		file.Device = snap.Device
		file.Inode = snap.Inode
		file.IsSymlink = false
		file.ContentSHA256 = snap.Hash

		ordinal := len(plans) + 1
		shortHash := snap.Hash
		if len(shortHash) > 20 {
			shortHash = shortHash[:20]
		}
		planID := fmt.Sprintf("%s-%03d-%s", taskID, ordinal, shortHash)
		targetPath := filepath.Join(targetRoot, planID+coldCopyExtension(item.Format))
		plans = append(plans, domain.OperationPlan{
			ID: planID, TaskID: taskID, State: domain.PlanDraft,
			ContentSHA256: snap.Hash, Size: snap.Size, Risk: domain.RiskHigh,
			Actions: []domain.PlannedAction{{
				Path: file.Path, Action: domain.OperationCopy,
				Reason:     "bounded cold-storage review copy; source remains unchanged",
				TargetPath: targetPath, Context: item.Context, File: file,
			}},
			Evidence: []string{
				"selected from COLD_STORAGE_REVIEW advisory tier",
				"no protection, business anchor, or file relation detected",
				"full SHA-256 and filesystem identity captured at DRAFT preparation",
				"COPY only; source removal is not authorized",
			},
		})
		total += snap.Size
	}
	if len(plans) == 0 {
		return nil, 0, fmt.Errorf("no cold-review candidates fit the bounded wave")
	}
	return plans, total, nil
}

func validateColdCandidate(item mediaTierItem, sourceRoot string) error {
	if item.Tier != coldReviewTier || !filepath.IsAbs(item.Path) || !withinPath(sourceRoot, item.Path) {
		return fmt.Errorf("cold candidate is outside the approved source root")
	}
	ctx := item.Context
	if ctx.Protected || ctx.BusinessAnchor != "" || item.RelationCount != 0 ||
		ctx.Role == domain.RoleRaw || ctx.Role == domain.RoleFormalArchive ||
		ctx.Role == domain.RoleProjectWork || ctx.Role == domain.RoleBackup ||
		ctx.Role == domain.RoleSensitive || ctx.Role == domain.RoleSystem {
		return fmt.Errorf("cold candidate no longer satisfies protection and context gates")
	}
	return nil
}

func withinPath(root, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func coldCopyExtension(format string) string {
	switch strings.ToLower(format) {
	case "mp4":
		return ".mp4"
	case "mov":
		return ".mov"
	case "mpeg", "mpeg1", "mpeg2":
		return ".mpeg"
	default:
		return ".bin"
	}
}

func runReconcileCoverage(args []string) error {
	fs := flag.NewFlagSet("next-steps reconcile-coverage", flag.ContinueOnError)
	dbPath := fs.String("db", "", "analysis SQLite database")
	indexPath := fs.String("index", "", "latest partial scan index")
	storageID := fs.String("storage-id", "", "exact storage ID")
	out := fs.String("out", "./var/coverage-reconciliation.json", "private aggregate reconciliation receipt")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *indexPath == "" || *storageID == "" {
		return fmt.Errorf("--db, --index and --storage-id are required")
	}
	files, err := idx.Read(*indexPath)
	if err != nil {
		return err
	}
	seen := make([]string, 0, len(files))
	for _, file := range files {
		if file.StorageID == *storageID {
			seen = append(seen, file.Path)
		}
	}
	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	unavailable, err := st.MarkFilesUnavailable(ctx, *storageID, seen)
	if err != nil {
		return err
	}
	receipt := map[string]any{
		"generated_at": time.Now().UTC(), "mode": "partial_scan_fail_closed",
		"seen_files": len(seen), "unavailable_marked": unavailable,
		"source_files_modified": 0, "execution_authorized": false,
	}
	if err := writePrivateJSON(*out, receipt); err != nil {
		return err
	}
	fmt.Printf("reconciled partial coverage: %d seen, %d unavailable; source files unchanged\n", len(seen), unavailable)
	return nil
}

type criticalHoldItem struct {
	PlanID        string           `json:"plan_id"`
	OriginalState domain.PlanState `json:"original_state"`
	Risk          domain.RiskLevel `json:"risk"`
	Size          int64            `json:"size"`
	Status        string           `json:"status"`
	Reason        string           `json:"reason"`
}

type criticalHoldRegister struct {
	GeneratedAt         time.Time          `json:"generated_at"`
	ExecutionAuthorized bool               `json:"execution_authorized"`
	ApprovalGate        string             `json:"approval_gate"`
	Count               int                `json:"count"`
	Bytes               int64              `json:"bytes"`
	Items               []criticalHoldItem `json:"items"`
}

func runHoldCritical(args []string) error {
	fs := flag.NewFlagSet("next-steps hold-critical", flag.ContinueOnError)
	planPath := fs.String("plan", "", "private DRAFT plan input")
	out := fs.String("out", "./var/critical-hold-register.json", "private HOLD register")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *planPath == "" {
		return fmt.Errorf("--plan is required")
	}
	plans, err := readPlans(*planPath)
	if err != nil {
		return err
	}
	register := buildCriticalHoldRegister(plans, time.Now().UTC())
	if err := writePrivateJSON(*out, register); err != nil {
		return err
	}
	fmt.Printf("registered %d critical plan(s) as HOLD; approval gate enforced, no filesystem operations\n", register.Count)
	return nil
}

func buildCriticalHoldRegister(plans []domain.OperationPlan, now time.Time) criticalHoldRegister {
	register := criticalHoldRegister{
		GeneratedAt: now.UTC(), ExecutionAuthorized: false,
		ApprovalGate: "critical plans are rejected by ordinary approve; independent hold release is not implemented",
	}
	for _, plan := range plans {
		if plan.Risk != domain.RiskCritical {
			continue
		}
		register.Items = append(register.Items, criticalHoldItem{
			PlanID: plan.ID, OriginalState: plan.State, Risk: plan.Risk, Size: plan.Size,
			Status: "HOLD", Reason: "directory duty, business anchor, protection rule, and backup domain require independent review",
		})
		register.Count++
		register.Bytes += plan.Size
	}
	return register
}

type mediaTierItem struct {
	StorageID        string                  `json:"storage_id"`
	Path             string                  `json:"path"`
	Size             int64                   `json:"size"`
	Format           string                  `json:"format"`
	Context          domain.DirectoryContext `json:"context"`
	RelationCount    int                     `json:"relation_count"`
	Tier             string                  `json:"tier"`
	ProxyRecommended bool                    `json:"proxy_recommended"`
	Reason           string                  `json:"reason"`
}

type mediaTieringPlan struct {
	GeneratedAt         time.Time        `json:"generated_at"`
	ExecutionAuthorized bool             `json:"execution_authorized"`
	SourceGeneratedAt   time.Time        `json:"source_generated_at"`
	Summary             map[string]int   `json:"summary"`
	BytesByTier         map[string]int64 `json:"bytes_by_tier"`
	Items               []mediaTierItem  `json:"items"`
	SafetyNotes         []string         `json:"safety_notes"`
}

func runMediaTiering(args []string) error {
	fs := flag.NewFlagSet("next-steps media-tiering", flag.ContinueOnError)
	reviewPath := fs.String("review", "", "private governance review input")
	out := fs.String("out", "./var/media-tiering-draft.json", "private advisory tiering output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *reviewPath == "" {
		return fmt.Errorf("--review is required")
	}
	data, err := os.ReadFile(*reviewPath)
	if err != nil {
		return err
	}
	var review governancediag.Report
	if err := json.Unmarshal(data, &review); err != nil {
		return fmt.Errorf("decode governance review: %w", err)
	}
	plan := buildMediaTieringPlan(review, time.Now().UTC())
	if err := writePrivateJSON(*out, plan); err != nil {
		return err
	}
	fmt.Printf("created advisory media tiering for %d item(s); no move, archive, or delete operations\n", len(plan.Items))
	return nil
}

func buildMediaTieringPlan(review governancediag.Report, now time.Time) mediaTieringPlan {
	plan := mediaTieringPlan{
		GeneratedAt: now.UTC(), SourceGeneratedAt: review.GeneratedAt, ExecutionAuthorized: false,
		Summary: map[string]int{}, BytesByTier: map[string]int64{},
		SafetyNotes: []string{
			"This is an advisory storage-tier review, not an execution plan.",
			"Protected, anchored, related, and project media never enter an automatic cold-storage action.",
			"Cold-storage candidates still require access-frequency, recovery-SLA, and backup-domain review.",
		},
	}
	for _, item := range review.LargeMediaReviews {
		tier, reason := mediaTier(item)
		out := mediaTierItem{
			StorageID: item.StorageID, Path: item.Path, Size: item.Size, Format: item.Format.Format,
			Context: item.Context, RelationCount: item.RelationCount, Tier: tier,
			ProxyRecommended: item.Format.Category == domain.CategoryVideo && item.Size >= 1<<30,
			Reason:           reason,
		}
		plan.Items = append(plan.Items, out)
		plan.Summary[tier]++
		plan.BytesByTier[tier] += item.Size
	}
	return plan
}

func mediaTier(item governancediag.LargeMediaReview) (string, string) {
	ctx := item.Context
	if ctx.Protected || ctx.Role == domain.RoleRaw || ctx.Role == domain.RoleFormalArchive ||
		ctx.Role == domain.RoleBackup || ctx.Role == domain.RoleSensitive || ctx.Role == domain.RoleSystem {
		return "ONLINE_PROTECTED", "protection or authoritative directory duty takes precedence"
	}
	if ctx.Role == domain.RoleProjectWork || ctx.BusinessAnchor != "" || item.RelationCount > 0 {
		return "PROJECT_ARCHIVE_REVIEW", "project context, business anchor, or media relation requires coordinated archiving"
	}
	return "COLD_STORAGE_REVIEW", "large media without a detected protection, business anchor, or relation; human SLA review required"
}
