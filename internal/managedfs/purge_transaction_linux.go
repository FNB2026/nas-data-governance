//go:build linux

package managedfs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"reflect"
	"strings"

	"golang.org/x/sys/unix"
)

const purgeStage = ".ndg-purge-stage"

func purgeStat(fd int) (unix.Statx_t, error) {
	var st unix.Statx_t
	const mask = unix.STATX_BASIC_STATS | unix.STATX_MNT_ID_UNIQUE
	if err := unix.Statx(fd, "", unix.AT_EMPTY_PATH, mask, &st); err != nil || st.Mask&mask != mask {
		return st, ErrUnsupported
	}
	return st, nil
}

func purgeIdentity(st unix.Statx_t) PurgeIdentity {
	return PurgeIdentity{Device: unix.Mkdev(st.Dev_major, st.Dev_minor), Inode: st.Ino, Mount: st.Mnt_id}
}

func purgeDirIdentity(fd int) (PurgeIdentity, error) {
	st, err := purgeStat(fd)
	return purgeIdentity(st), err
}

func purgeFileFD(directory int, name string) (int, error) {
	// openat2 rejects O_NONBLOCK when combined with O_PATH.
	fd, err := unix.Openat2(directory, name, &unix.OpenHow{
		Flags:   unix.O_PATH | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV,
	})
	if err != nil {
		return -1, ErrRejected
	}
	return fd, nil
}

func purgeRelative(relative string) bool {
	if !fs.ValidPath(relative) || relative == "." || strings.ContainsAny(relative, "\x00\r\n\\") {
		return false
	}
	for part := range strings.SplitSeq(relative, "/") {
		if part == purgeStage {
			return false
		}
	}
	return true
}

func samePurgeFile(a, b unix.Statx_t) bool {
	return purgeIdentity(a) == purgeIdentity(b) && a.Mode == b.Mode && a.Nlink == b.Nlink &&
		a.Uid == b.Uid && a.Gid == b.Gid && a.Size == b.Size && a.Mtime == b.Mtime && a.Ctime == b.Ctime
}

func purgeFile(ctx context.Context, fd int, relative string) (PurgeTarget, error) {
	st, err := purgeStat(fd)
	if err != nil {
		return PurgeTarget{}, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Size > uint64(maxObjectBytes) {
		return PurgeTarget{}, ErrRejected
	}
	// A kernel FD link opens the already confined regular inode. The source
	// pathname is never re-resolved for the content read.
	content, err := os.Open(fmt.Sprintf("/proc/self/fd/%d", fd))
	if err != nil {
		return PurgeTarget{}, ErrRejected
	}
	defer content.Close()
	before, err := purgeStat(int(content.Fd()))
	if err != nil || !samePurgeFile(before, st) || before.Uid != uint32(os.Geteuid()) ||
		before.Gid != uint32(os.Getegid()) || before.Mode&0022 != 0 || !aclAbsent(int(content.Fd())) {
		return PurgeTarget{}, ErrRejected
	}
	h := sha256.New()
	reader := io.LimitReader(content, int64(st.Size)+1)
	buffer := make([]byte, 64<<10)
	var count uint64
	for {
		if ctx.Err() != nil {
			return PurgeTarget{}, ErrCancelled
		}
		n, err := reader.Read(buffer)
		count += uint64(n)
		_, _ = h.Write(buffer[:n])
		if err == io.EOF {
			break
		}
		if err != nil || n == 0 {
			return PurgeTarget{}, ErrRejected
		}
	}
	after, err := purgeStat(int(content.Fd()))
	if err != nil || !samePurgeFile(after, before) || count != st.Size {
		return PurgeTarget{}, ErrRejected
	}
	return PurgeTarget{
		Relative: relative, File: purgeIdentity(st), Size: st.Size, Mode: st.Mode, Links: st.Nlink,
		MTimeSec: st.Mtime.Sec, MTimeNsec: st.Mtime.Nsec, CTimeSec: st.Ctime.Sec, CTimeNsec: st.Ctime.Nsec,
		SHA256: hex.EncodeToString(h.Sum(nil)),
	}, nil
}

// The exclusive Worker account is the only non-administrator that can
// mutate the namespace. A descriptor alone does not prevent a writable
// parent directory from being renamed between validation and mutation.
func openPurgeParent(root int, relative string) (int, error) {
	current, err := rooted(root, ".", unix.O_RDONLY|unix.O_DIRECTORY)
	if err != nil {
		return -1, err
	}
	if _, err := directory(current, true); err != nil {
		unix.Close(current)
		return -1, err
	}
	parent := path.Dir(relative)
	if parent == "." {
		return current, nil
	}
	for part := range strings.SplitSeq(parent, "/") {
		next, err := rooted(current, part, unix.O_RDONLY|unix.O_DIRECTORY)
		unix.Close(current)
		if err != nil {
			return -1, err
		}
		if _, err := directory(next, true); err != nil {
			unix.Close(next)
			return -1, err
		}
		current = next
	}
	return current, nil
}

func observeUnderRoot(ctx context.Context, fd int, relative string) (PurgeTarget, error) {
	if !purgeRelative(relative) {
		return PurgeTarget{}, ErrRejected
	}
	parent, err := openPurgeParent(fd, relative)
	if err != nil {
		return PurgeTarget{}, err
	}
	defer unix.Close(parent)
	file, err := purgeFileFD(fd, relative)
	if err != nil {
		return PurgeTarget{}, err
	}
	defer unix.Close(file)
	result, err := purgeFile(ctx, file, relative)
	if err != nil {
		return PurgeTarget{}, err
	}
	result.Root, err = purgeDirIdentity(fd)
	if err != nil {
		return PurgeTarget{}, err
	}
	result.Parent, err = purgeDirIdentity(parent)
	if err != nil || result.Root.Mount != result.Parent.Mount || result.Root.Mount != result.File.Mount {
		return PurgeTarget{}, ErrRejected
	}
	bound, err := purgeFileFD(parent, path.Base(relative))
	if err != nil {
		return PurgeTarget{}, err
	}
	defer unix.Close(bound)
	st, err := purgeStat(bound)
	if err != nil || purgeIdentity(st) != result.File {
		return PurgeTarget{}, ErrRejected
	}
	return result, nil
}

func observePurgeTarget(ctx context.Context, root, relative string) (PurgeTarget, error) {
	fd, err := absolute(root)
	if err != nil {
		return PurgeTarget{}, err
	}
	defer unix.Close(fd)
	return observeUnderRoot(ctx, fd, relative)
}

func validatePurgeBinding(plan PurgeTransaction, root, stage, parent int) error {
	current, err := absolute(plan.RootPath)
	if err != nil {
		return ErrRejected
	}
	defer unix.Close(current)
	currentID, err := purgeDirIdentity(current)
	if err != nil || currentID != plan.Target.Root {
		return ErrRejected
	}
	for _, binding := range []struct {
		name string
		fd   int
	}{{purgeStage, stage}, {path.Dir(plan.Target.Relative), parent}} {
		current, err := rooted(root, binding.name, unix.O_RDONLY|unix.O_DIRECTORY)
		if err != nil {
			return ErrRejected
		}
		observed, err := purgeDirIdentity(current)
		unix.Close(current)
		expected, fdErr := purgeDirIdentity(binding.fd)
		if err != nil || fdErr != nil || observed != expected {
			return ErrRejected
		}
	}
	return nil
}

func purgeEntryAbsent(dir int, name string) bool {
	var st unix.Stat_t
	return errors.Is(unix.Fstatat(dir, name, &st, unix.AT_SYMLINK_NOFOLLOW), unix.ENOENT)
}

func purgeKeeper(ctx context.Context, root int, plan PurgeTransaction) error {
	if plan.Keeper == nil {
		return nil
	}
	keeper, err := observeUnderRoot(ctx, root, plan.Keeper.Relative)
	if err != nil || keeper != *plan.Keeper || keeper.File == plan.Target.File ||
		keeper.SHA256 != plan.Target.SHA256 || keeper.Size != plan.Target.Size {
		return ErrRejected
	}
	return nil
}

func purgeJournal(stage int, name string, record any) error {
	fd, err := createAt(stage, name)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), "purge journal")
	if err := json.NewEncoder(f).Encode(record); err != nil {
		f.Close()
		return err
	}
	err = errors.Join(f.Sync(), f.Close())
	if err != nil {
		return err
	}
	return unix.Fsync(stage)
}

type purgeDirectorySet struct {
	root, stage, parent int
}

func (d purgeDirectorySet) close() {
	for _, fd := range []int{d.parent, d.stage, d.root} {
		if fd >= 0 {
			unix.Close(fd)
		}
	}
}

func openPurgeDirectories(plan PurgeTransaction) (purgeDirectorySet, error) {
	d := purgeDirectorySet{-1, -1, -1}
	if os.Geteuid() == 0 || os.Getegid() == 0 || !opaque(plan.ID) || !opaque(plan.ApprovalSHA256) ||
		!purgeRelative(plan.Target.Relative) {
		return d, ErrRejected
	}
	opened := false
	defer func() {
		if !opened {
			d.close()
		}
	}()
	var err error
	d.root, err = absolute(plan.RootPath)
	if err != nil {
		return d, err
	}

	d.stage, err = rooted(d.root, purgeStage, unix.O_RDONLY|unix.O_DIRECTORY)
	if err != nil {
		return d, err
	}
	if _, err := directory(d.stage, true); err != nil {
		return d, err
	}
	if err := unix.Flock(d.stage, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return d, ErrBusy
	}
	d.parent, err = openPurgeParent(d.root, plan.Target.Relative)
	if err != nil {
		return d, err
	}
	parentID, err := purgeDirIdentity(d.parent)
	if err != nil || parentID != plan.Target.Parent || validatePurgeBinding(plan, d.root, d.stage, d.parent) != nil {
		return d, ErrRejected
	}
	opened = true
	return d, nil
}

func executePurgeTransaction(ctx context.Context, plan PurgeTransaction, fault func(string) error) (PurgeReceipt, error) {
	empty := PurgeReceipt{}
	d, err := openPurgeDirectories(plan)
	if err != nil {
		return empty, err
	}
	defer d.close()
	root, stage, parent := d.root, d.stage, d.parent
	if !purgeEntryAbsent(stage, "pending") {
		return empty, ErrRecoveryRequired
	}
	if !purgeEntryAbsent(stage, "receipt-"+plan.ID) || !purgeEntryAbsent(stage, plan.ID) {
		return empty, ErrRejected
	}
	current, err := observeUnderRoot(ctx, root, plan.Target.Relative)
	if err != nil || current != plan.Target || purgeKeeper(ctx, root, plan) != nil {
		return empty, ErrRejected
	}
	if err := purgeJournal(stage, "pending", plan); err != nil {
		return empty, ErrRecoveryRequired
	}
	// The journal is durable before the first namespace operation. Any later
	// ambiguity keeps the installation blocked with its evidence intact.
	fail := func(phase string) bool {
		return ctx.Err() != nil || (fault != nil && fault(phase) != nil)
	}
	if fail("pending") || validatePurgeBinding(plan, root, stage, parent) != nil ||
		unix.Renameat2(parent, path.Base(plan.Target.Relative), stage, plan.ID, unix.RENAME_NOREPLACE) != nil ||
		fail("renamed") || unix.Fsync(parent) != nil || fail("parent_synced") ||
		unix.Fsync(stage) != nil || fail("staged") {
		return empty, ErrRecoveryRequired
	}
	staged, err := purgeFileFD(stage, plan.ID)
	if err != nil {
		return empty, ErrRecoveryRequired
	}
	defer unix.Close(staged)
	actual, err := purgeFile(ctx, staged, plan.Target.Relative)
	// A rename can update ctime. All other evidence remains bound to the plan;
	// the new ctime is retained in the commit record and checked before unlink.
	actual.Root, actual.Parent = current.Root, current.Parent
	comparable := actual
	comparable.CTimeSec, comparable.CTimeNsec = current.CTimeSec, current.CTimeNsec
	if err != nil || comparable != current || validatePurgeBinding(plan, root, stage, parent) != nil ||
		purgeKeeper(ctx, root, plan) != nil {
		return empty, ErrRecoveryRequired
	}
	marker := "commit-" + plan.ID
	if purgeJournal(stage, marker, actual) != nil || fail("commit_pending") {
		return empty, ErrRecoveryRequired
	}
	verified, err := purgeFileFD(stage, plan.ID)
	if err != nil {
		return empty, ErrRecoveryRequired
	}
	confirmed, err := purgeFile(ctx, verified, plan.Target.Relative)
	unix.Close(verified)
	confirmed.Root, confirmed.Parent = current.Root, current.Parent
	if err != nil || confirmed != actual || validatePurgeBinding(plan, root, stage, parent) != nil ||
		purgeKeeper(ctx, root, plan) != nil || unix.Unlinkat(stage, plan.ID, 0) != nil ||
		fail("unlinked_unsynced") || unix.Fsync(stage) != nil || fail("unlinked") {
		return empty, ErrRecoveryRequired
	}
	receipt := PurgeReceipt{ID: plan.ID, State: "PURGED"}
	if purgeJournal(stage, "receipt-"+plan.ID, receipt) != nil || fail("receipt_synced") ||
		unix.Unlinkat(stage, "pending", 0) != nil || fail("pending_removed") || unix.Fsync(stage) != nil {
		return empty, ErrRecoveryRequired
	}
	return receipt, nil
}

func readPurgeRecord(dir int, name string, record any) error {
	fd, err := rooted(dir, name, unix.O_RDONLY)
	if err != nil {
		return ErrRecoveryRequired
	}
	f := os.NewFile(uintptr(fd), "purge journal")
	defer f.Close()
	st, err := regular(fd)
	if err != nil || st.Size > 16384 {
		return ErrRecoveryRequired
	}
	raw, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil || json.Unmarshal(raw, record) != nil || f.Sync() != nil {
		return ErrRecoveryRequired
	}
	return nil
}

// ReconcilePurgeTransaction is operator requested. It never performs an
// unlink of user content. It only reconciles complete, attributable proof.
func reconcilePurgeTransaction(ctx context.Context, plan PurgeTransaction) (PurgeReceipt, error) {
	empty := PurgeReceipt{}
	d, err := openPurgeDirectories(plan)
	if err != nil {
		return empty, ErrRecoveryRequired
	}
	stage := d.stage
	receiptName := "receipt-" + plan.ID
	commitName := "commit-" + plan.ID
	name := path.Base(plan.Target.Relative)
	pending := !purgeEntryAbsent(stage, "pending")
	receiptExists := !purgeEntryAbsent(stage, receiptName)
	if pending && !receiptExists {
		d.close()
		// This helper reacquires the stage flock and rechecks all bindings.
		return recoverPurgeTransaction(ctx, plan)
	}
	defer d.close()
	if ctx.Err() != nil || !purgeEntryAbsent(stage, plan.ID) || validatePurgeBinding(plan, d.root, stage, d.parent) != nil {
		return empty, ErrRecoveryRequired
	}
	if pending {
		var recorded PurgeTransaction
		if readPurgeRecord(stage, "pending", &recorded) != nil || !reflect.DeepEqual(recorded, plan) {
			return empty, ErrRecoveryRequired
		}
	}
	var receipt PurgeReceipt
	if receiptExists {
		if readPurgeRecord(stage, receiptName, &receipt) != nil || receipt.ID != plan.ID {
			return empty, ErrRecoveryRequired
		}
	} else {
		receipt = PurgeReceipt{ID: plan.ID, State: "ROLLED_BACK"}
	}
	switch receipt.State {
	case "PURGED":
		var commit PurgeTarget
		if readPurgeRecord(stage, commitName, &commit) != nil || !purgeEntryAbsent(d.parent, name) {
			return empty, ErrRecoveryRequired
		}
		commit.CTimeSec, commit.CTimeNsec = plan.Target.CTimeSec, plan.Target.CTimeNsec
		if commit != plan.Target || purgeKeeper(ctx, d.root, plan) != nil {
			return empty, ErrRecoveryRequired
		}
	case "ROLLED_BACK":
		if !purgeEntryAbsent(stage, commitName) {
			return empty, ErrRecoveryRequired
		}
		current, err := observeUnderRoot(ctx, d.root, plan.Target.Relative)
		if receiptExists {
			current.CTimeSec, current.CTimeNsec = plan.Target.CTimeSec, plan.Target.CTimeNsec
		}
		if err != nil || current != plan.Target {
			return empty, ErrRecoveryRequired
		}
	default:
		return empty, ErrRecoveryRequired
	}
	if ctx.Err() != nil || validatePurgeBinding(plan, d.root, stage, d.parent) != nil {
		return empty, ErrRecoveryRequired
	}
	if !receiptExists && purgeJournal(stage, receiptName, receipt) != nil {
		return empty, ErrRecoveryRequired
	}
	if pending && unix.Unlinkat(stage, "pending", 0) != nil {
		return empty, ErrRecoveryRequired
	}
	if err := unix.Fsync(stage); err != nil {
		return empty, ErrRecoveryRequired
	}
	return receipt, nil
}

// recoverPurgeTransaction can restore only a provably staged object before
// commit authorization. Ambiguous or post-commit states keep their lock.
func recoverPurgeTransaction(ctx context.Context, plan PurgeTransaction) (PurgeReceipt, error) {
	empty := PurgeReceipt{}
	d, err := openPurgeDirectories(plan)
	if err != nil {
		return empty, err
	}
	defer d.close()
	stage := d.stage
	if !purgeEntryAbsent(stage, "commit-"+plan.ID) || !purgeEntryAbsent(stage, "receipt-"+plan.ID) {
		return empty, ErrRecoveryRequired
	}
	expected, err := json.Marshal(plan)
	if err != nil {
		return empty, ErrRecoveryRequired
	}
	fd, err := rooted(stage, "pending", unix.O_RDONLY)
	if err != nil {
		return empty, ErrRecoveryRequired
	}
	journal := os.NewFile(uintptr(fd), "purge journal")
	defer journal.Close()
	if _, err := regular(fd); err != nil {
		return empty, ErrRecoveryRequired
	}
	expected = append(expected, '\n')
	record, err := io.ReadAll(io.LimitReader(journal, int64(len(expected)+1)))
	if err != nil || !bytes.Equal(record, expected) || ctx.Err() != nil {
		return empty, ErrRecoveryRequired
	}
	name := path.Base(plan.Target.Relative)
	if purgeEntryAbsent(stage, plan.ID) {
		current, err := observeUnderRoot(ctx, d.root, plan.Target.Relative)
		if err != nil || current != plan.Target {
			return empty, ErrRecoveryRequired
		}
	} else {
		staged, err := purgeFileFD(stage, plan.ID)
		if err != nil {
			return empty, ErrRecoveryRequired
		}
		current, err := purgeFile(ctx, staged, plan.Target.Relative)
		unix.Close(staged)
		current.Root, current.Parent = plan.Target.Root, plan.Target.Parent
		current.CTimeSec, current.CTimeNsec = plan.Target.CTimeSec, plan.Target.CTimeNsec
		if err != nil || current != plan.Target || validatePurgeBinding(plan, d.root, stage, d.parent) != nil ||
			unix.Renameat2(stage, plan.ID, d.parent, name, unix.RENAME_NOREPLACE) != nil ||
			unix.Fsync(d.parent) != nil || unix.Fsync(stage) != nil {
			return empty, ErrRecoveryRequired
		}
	}
	receipt := PurgeReceipt{ID: plan.ID, State: "ROLLED_BACK"}
	if purgeJournal(stage, "receipt-"+plan.ID, receipt) != nil ||
		unix.Unlinkat(stage, "pending", 0) != nil || unix.Fsync(stage) != nil {
		return empty, ErrRecoveryRequired
	}
	return receipt, nil
}

func dryRunPurgeTransaction(ctx context.Context, plan PurgeTransaction) error {
	d, err := openPurgeDirectories(plan)
	if err != nil {
		return err
	}
	defer d.close()
	if !purgeEntryAbsent(d.stage, "pending") {
		return ErrRecoveryRequired
	}
	if !purgeEntryAbsent(d.stage, "receipt-"+plan.ID) || !purgeEntryAbsent(d.stage, plan.ID) {
		return ErrRejected
	}
	observed, err := observeUnderRoot(ctx, d.root, plan.Target.Relative)
	if err != nil || observed != plan.Target || purgeKeeper(ctx, d.root, plan) != nil {
		return ErrRejected
	}
	return nil
}
