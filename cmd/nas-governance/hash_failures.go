package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"nas-data-governance/internal/domain"
	"nas-data-governance/internal/fingerprint"
	idx "nas-data-governance/internal/index"
)

const privateFileMode = 0o600

type hashFailure struct {
	ID           string    `json:"id"`
	Stage        string    `json:"stage"`
	StorageID    string    `json:"storage_id"`
	Path         string    `json:"path"`
	Size         int64     `json:"size"`
	ModifiedAt   time.Time `json:"modified_at"`
	Device       uint64    `json:"device"`
	Inode        uint64    `json:"inode"`
	Attempts     int       `json:"attempts"`
	Status       string    `json:"status"`
	DiscoveredAt time.Time `json:"discovered_at"`
}

func newHashFailure(file domain.FileInstance, stage string, attempts int, status string) hashFailure {
	sum := sha256.Sum256([]byte(file.StorageID + "\x00" + file.Path))
	return hashFailure{
		ID: hex.EncodeToString(sum[:]), Stage: stage, StorageID: file.StorageID,
		Path: file.Path, Size: file.Size, ModifiedAt: file.ModifiedAt,
		Device: file.Device, Inode: file.Inode, Attempts: attempts,
		Status: status, DiscoveredAt: file.DiscoveredAt,
	}
}

func writeHashFailures(path string, failures []hashFailure) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, privateFileMode)
	if err != nil {
		return err
	}
	if err := f.Chmod(privateFileMode); err != nil {
		_ = f.Close()
		return err
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	for _, failure := range failures {
		if err := enc.Encode(failure); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func readHashFailures(path string) ([]hashFailure, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var failures []hashFailure
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 2*1024*1024)
	line := 0
	for s.Scan() {
		line++
		var failure hashFailure
		if err := json.Unmarshal(s.Bytes(), &failure); err != nil {
			return nil, fmt.Errorf("failure manifest line %d is invalid", line)
		}
		if failure.ID == "" || failure.Path == "" || (failure.Stage != "quick" && failure.Stage != "full") {
			return nil, fmt.Errorf("failure manifest line %d is incomplete", line)
		}
		failures = append(failures, failure)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return failures, nil
}

type hashFunc func(string, int64) (string, error)

func hashWithRetry(ctx context.Context, path string, size int64, attempts int, delay time.Duration, hash hashFunc) (string, int, error) {
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		value, err := hash(path, size)
		if err == nil {
			return value, attempt, nil
		}
		if attempt == attempts {
			return "", attempt, err
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", attempt, ctx.Err()
		case <-timer.C:
		}
	}
	return "", attempts, errors.New("hash attempts exhausted")
}

var quickHash hashFunc = fingerprint.Quick

var fullHash hashFunc = func(path string, _ int64) (string, error) {
	return fingerprint.Full(path)
}

func runRetryHashes(args []string) error {
	fsFlags := flag.NewFlagSet("retry-hashes", flag.ContinueOnError)
	rootArg := fsFlags.String("root", "", "approved task root containing every retry target")
	failuresPath := fsFlags.String("failures", "", "private hash failure manifest produced by scan")
	indexPath := fsFlags.String("index", "", "source JSONL index")
	outPath := fsFlags.String("out", "", "new JSONL index with recovered hashes")
	remainingPath := fsFlags.String("remaining-out", "", "private manifest for unresolved entries")
	attempts := fsFlags.Int("attempts", 3, "maximum read attempts per file")
	retryDelay := fsFlags.Duration("retry-delay", 250*time.Millisecond, "delay between read attempts")
	if err := fsFlags.Parse(args); err != nil {
		return err
	}
	if *rootArg == "" || *failuresPath == "" || *indexPath == "" || *outPath == "" {
		return fmt.Errorf("--root, --failures, --index and --out are required")
	}
	if *attempts < 1 || *attempts > 10 {
		return fmt.Errorf("--attempts must be between 1 and 10")
	}
	if *retryDelay < 0 || *retryDelay > 30*time.Second {
		return fmt.Errorf("--retry-delay must be between 0 and 30s")
	}
	if samePath(*indexPath, *outPath) {
		return fmt.Errorf("--out must differ from --index; retry never overwrites its source")
	}
	if *remainingPath == "" {
		*remainingPath = *outPath + ".hash-failures.jsonl"
	}

	root, rootInfo, err := validateRetryRoot(*rootArg)
	if err != nil {
		return err
	}
	rootDevice, _ := fileIdentity(rootInfo)
	failures, err := readHashFailures(*failuresPath)
	if err != nil {
		return err
	}
	files, err := idx.Read(*indexPath)
	if err != nil {
		return err
	}
	byPath := make(map[string]int, len(files))
	for i := range files {
		byPath[files[i].Path] = i
	}

	ctx := context.Background()
	remaining := make([]hashFailure, 0, len(failures))
	resolved := make(map[string]bool, len(failures))
	quickRecovered := make(map[int]hashFailure)
	for _, failure := range failures {
		indexAt, ok := byPath[failure.Path]
		if !ok {
			failure.Status = "index_record_missing"
			remaining = append(remaining, failure)
			continue
		}
		info, status := validateRetryTarget(root, rootDevice, failure)
		if status != "" {
			failure.Status = status
			remaining = append(remaining, failure)
			continue
		}
		var fn hashFunc = quickHash
		if failure.Stage == "full" {
			fn = fullHash
		}
		value, used, hashErr := hashWithRetry(ctx, failure.Path, info.Size(), *attempts, *retryDelay, fn)
		failure.Attempts += used
		if hashErr != nil {
			failure.Status = "hash_failed"
			remaining = append(remaining, failure)
			continue
		}
		if failure.Stage == "quick" {
			files[indexAt].QuickHash = value
			quickRecovered[indexAt] = failure
		} else {
			files[indexAt].ContentSHA256 = value
		}
		resolved[failure.ID] = true
	}

	// A recovered quick hash may place the file into a duplicate candidate
	// bucket. Complete the progressive fingerprint for that recovered file so
	// duplicate analysis does not silently omit it.
	quickCounts := make(map[string]int)
	for _, file := range files {
		if file.QuickHash != "" {
			quickCounts[fmt.Sprintf("%d:%s", file.Size, file.QuickHash)]++
		}
	}
	for indexAt, originalFailure := range quickRecovered {
		file := &files[indexAt]
		key := fmt.Sprintf("%d:%s", file.Size, file.QuickHash)
		if quickCounts[key] < 2 || file.ContentSHA256 != "" {
			continue
		}
		value, used, hashErr := hashWithRetry(ctx, file.Path, file.Size, *attempts, *retryDelay, fullHash)
		if hashErr == nil {
			file.ContentSHA256 = value
			continue
		}
		resolved[originalFailure.ID] = false
		fullFailure := newHashFailure(*file, "full", used, "hash_failed")
		fullFailure.Attempts += originalFailure.Attempts
		remaining = append(remaining, fullFailure)
	}

	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		return err
	}
	if err := idx.Write(*outPath, files); err != nil {
		return err
	}
	if err := writeHashFailures(*remainingPath, remaining); err != nil {
		return err
	}
	recovered := 0
	for _, ok := range resolved {
		if ok {
			recovered++
		}
	}
	fmt.Printf("retried %d entries: %d recovered, %d unresolved; source paths omitted (read-only source access)\n", len(failures), recovered, len(remaining))
	return nil
}

func samePath(a, b string) bool {
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	return errA == nil && errB == nil && filepath.Clean(aa) == filepath.Clean(bb)
}

func validateRetryRoot(path string) (string, fs.FileInfo, error) {
	root, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", nil, fmt.Errorf("retry root is unavailable")
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return "", nil, fmt.Errorf("retry root must be a real directory, not a symlink")
	}
	return filepath.Clean(root), info, nil
}

func validateRetryTarget(root string, rootDevice uint64, failure hashFailure) (fs.FileInfo, string) {
	path := filepath.Clean(failure.Path)
	if !filepath.IsAbs(path) {
		return nil, "outside_root"
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, "outside_root"
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, "missing"
			}
			return nil, "unavailable"
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, "symlink_rejected"
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, "unavailable"
	}
	if !info.Mode().IsRegular() {
		return nil, "not_regular_file"
	}
	device, inode := fileIdentity(info)
	if rootDevice != 0 && device != 0 && device != rootDevice {
		return nil, "cross_mount_rejected"
	}
	if info.Size() != failure.Size || !info.ModTime().Equal(failure.ModifiedAt) || inode != failure.Inode || (failure.Device != 0 && device != failure.Device) {
		return nil, "stale"
	}
	return info, ""
}

func fileIdentity(info fs.FileInfo) (uint64, uint64) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0
	}
	return uint64(stat.Dev), uint64(stat.Ino)
}
