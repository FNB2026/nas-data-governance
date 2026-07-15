// Package pathdiag performs private, read-only compatibility diagnostics for
// paths that were observed by a scanner but could not later be opened.
package pathdiag

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"golang.org/x/text/unicode/norm"
)

type Summary struct {
	Candidates               int `json:"candidates"`
	ExactNowExists           int `json:"exact_now_exists"`
	NFCVariantExists         int `json:"nfc_variant_exists"`
	NFDVariantExists         int `json:"nfd_variant_exists"`
	NormalizedSiblingMatches int `json:"normalized_sibling_matches"`
	ParentMissing            int `json:"parent_missing"`
	NoCurrentMatch           int `json:"no_current_match"`
	// ListableNotOpenable counts paths whose directory entry is visible
	// (Lstat succeeds) but the file cannot be opened. The classification records
	// the observed state without attributing it to a client, server, or protocol.
	ListableNotOpenable int `json:"listable_not_openable"`
	SafetyRejected      int `json:"safety_rejected"`
}

type Item struct {
	ID                     string `json:"id"`
	Path                   string `json:"path"`
	Classification         string `json:"classification"`
	ExactNowExists         bool   `json:"exact_now_exists"`
	NFCVariantExists       bool   `json:"nfc_variant_exists"`
	NFDVariantExists       bool   `json:"nfd_variant_exists"`
	NormalizedSiblingMatch bool   `json:"normalized_sibling_match"`
	// Openable reports whether the file can be opened read-only.
	// Distinct from ExactNowExists (which only checks Lstat) because some
	// filesystems list a directory entry but reject Open.
	Openable bool `json:"openable"`
	// NameHasDecomposedForm is true when the filename contains code
	// points that change under NFC normalization. It is diagnostic evidence,
	// not a root-cause conclusion.
	NameHasDecomposedForm bool     `json:"name_has_decomposed_form"`
	Evidence              []string `json:"evidence"`
}

type Report struct {
	GeneratedAt         time.Time `json:"generated_at"`
	ExecutionAuthorized bool      `json:"execution_authorized"`
	Summary             Summary   `json:"summary"`
	Items               []Item    `json:"items"`
	SafetyNotes         []string  `json:"safety_notes"`
}

func Build(root string, paths []string, now time.Time) (Report, error) {
	root, rootInfo, err := validateRoot(root)
	if err != nil {
		return Report{}, err
	}
	rootDevice := device(rootInfo)
	result := Report{
		GeneratedAt: now.UTC(), ExecutionAuthorized: false,
		SafetyNotes: []string{
			"只读取目录项和文件元数据，不打开文件内容",
			"为诊断可访问性会尝试以只读模式 Open 后立即关闭，不读取文件内容",
			"不跟随符号链接，不跨挂载点，不越过任务根目录",
			"Unicode 规范化匹配仅用于诊断，不自动重命名文件",
		},
	}
	unique := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if path != "." {
			unique[path] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(unique))
	for path := range unique {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	for _, path := range ordered {
		item := inspect(root, rootDevice, path)
		result.Items = append(result.Items, item)
		switch item.Classification {
		case "exact_now_exists":
			result.Summary.ExactNowExists++
		case "nfc_variant_exists":
			result.Summary.NFCVariantExists++
		case "nfd_variant_exists":
			result.Summary.NFDVariantExists++
		case "normalized_sibling_match":
			result.Summary.NormalizedSiblingMatches++
		case "parent_missing":
			result.Summary.ParentMissing++
		case "listable_not_openable":
			result.Summary.ListableNotOpenable++
		case "no_current_match":
			result.Summary.NoCurrentMatch++
		default:
			result.Summary.SafetyRejected++
		}
	}
	result.Summary.Candidates = len(result.Items)
	return result, nil
}

func inspect(root string, rootDevice uint64, path string) Item {
	item := Item{ID: pathID(path), Path: path}
	parent, status := validateParent(root, rootDevice, path)
	if status != "" {
		item.Classification = status
		item.Evidence = []string{"路径未通过只读安全边界检查"}
		return item
	}
	base := filepath.Base(path)
	item.NameHasDecomposedForm = hasDecomposedForm(base)
	lstatOK := existsSafe(path, rootDevice)
	openable, openErrno := openableSafe(path, rootDevice)
	item.Openable = openable
	// Both Lstat and Open succeed: the path is fully accessible now.
	if lstatOK && openable {
		item.ExactNowExists = true
		item.Classification = "exact_now_exists"
		item.Evidence = []string{"原路径当前可见，先前错误更符合瞬时变化或 SMB 缓存窗口"}
		return item
	}
	// Lstat succeeds but Open fails: the directory entry is visible but the file
	// cannot be opened. Preserve the observation without assigning responsibility
	// to the filesystem client, server, or Unicode handling layer.
	if lstatOK && !openable {
		item.Classification = "listable_not_openable"
		ev := []string{
			"目录项可见但 Open 失败，文件内容不可读取",
			fmt.Sprintf("Open 错误 errno=%d（仅诊断，不修改任何文件）", openErrno),
		}
		if item.NameHasDecomposedForm {
			ev = append(ev, "文件名包含会在 NFC 规范化下变化的分解态码点；该特征仅作相关性证据，不判定根因")
		}
		item.Evidence = ev
		return item
	}
	nfcBase := norm.NFC.String(base)
	nfdBase := norm.NFD.String(base)
	if nfcBase != base && existsSafe(filepath.Join(parent, nfcBase), rootDevice) {
		item.NFCVariantExists = true
		item.Classification = "nfc_variant_exists"
		item.Evidence = []string{"NFC 文件名变体当前存在，仅生成复核证据，不重命名"}
		return item
	}
	if nfdBase != base && existsSafe(filepath.Join(parent, nfdBase), rootDevice) {
		item.NFDVariantExists = true
		item.Classification = "nfd_variant_exists"
		item.Evidence = []string{"NFD 文件名变体当前存在，仅生成复核证据，不重命名"}
		return item
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		item.Classification = "parent_unavailable"
		item.Evidence = []string{"父目录当前不可读取"}
		return item
	}
	for _, entry := range entries {
		if entry.Name() != base && normalizedNamesMatch(entry.Name(), base) {
			item.NormalizedSiblingMatch = true
			item.Classification = "normalized_sibling_match"
			item.Evidence = []string{"父目录存在规范化后同名目录项，仅生成复核证据，不重命名"}
			return item
		}
	}
	item.Classification = "no_current_match"
	item.Evidence = []string{"原路径及 NFC/NFD 变体当前均不存在", "不能仅凭历史 no-such-file 错误判定规范化冲突"}
	return item
}

func validateRoot(root string) (string, fs.FileInfo, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", nil, fmt.Errorf("path diagnostic root is invalid")
	}
	info, err := os.Lstat(abs)
	if err != nil || !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("path diagnostic root must be an available real directory")
	}
	return filepath.Clean(abs), info, nil
}

func validateParent(root string, rootDevice uint64, path string) (string, string) {
	if !filepath.IsAbs(path) {
		return "", "outside_root"
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "outside_root"
	}
	parentRel := filepath.Dir(rel)
	current := root
	if parentRel != "." {
		for _, part := range strings.Split(parentRel, string(filepath.Separator)) {
			if part == "" || part == "." {
				continue
			}
			current = filepath.Join(current, part)
			info, err := os.Lstat(current)
			if os.IsNotExist(err) {
				return "", "parent_missing"
			}
			if err != nil || !info.IsDir() {
				return "", "parent_unavailable"
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				return "", "symlink_rejected"
			}
			if dev := device(info); rootDevice != 0 && dev != 0 && dev != rootDevice {
				return "", "cross_mount_rejected"
			}
		}
	}
	return current, ""
}

func existsSafe(path string, rootDevice uint64) bool {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&fs.ModeSymlink != 0 {
		return false
	}
	dev := device(info)
	return rootDevice == 0 || dev == 0 || dev == rootDevice
}

// openableSafe attempts a read-only Open of the path and immediately closes
// it. Returns (true, 0) on success. On failure returns (false, errno) where
// errno is the syscall.Errno underlying the *PathError, or 0 if not a
// syscall error. Never reads file content.
func openableSafe(path string, rootDevice uint64) (bool, int) {
	// Reject symlinks and cross-mount targets before Open: Open follows
	// symlinks, so we must guard explicitly to honor AGENTS rule 4.
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&fs.ModeSymlink != 0 {
		return false, 0
	}
	if dev := device(info); rootDevice != 0 && dev != 0 && dev != rootDevice {
		return false, 0
	}
	f, err := os.Open(path)
	if err != nil {
		if perr, ok := err.(*os.PathError); ok {
			if errno, ok := perr.Err.(syscall.Errno); ok {
				return false, int(errno)
			}
		}
		return false, 0
	}
	_ = f.Close()
	return true, 0
}

// hasDecomposedForm reports whether name changes under NFC normalization.
// The result is only a filename characteristic and does not attribute an
// accessibility failure to Unicode normalization.
func hasDecomposedForm(name string) bool {
	return norm.NFC.String(name) != name
}

func normalizedNamesMatch(a, b string) bool { return norm.NFC.String(a) == norm.NFC.String(b) }

func device(info fs.FileInfo) uint64 {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}
	return uint64(stat.Dev)
}

func pathID(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:])
}
