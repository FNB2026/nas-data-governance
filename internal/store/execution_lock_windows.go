//go:build windows

package store

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func (s *SQLiteStore) AcquireExecutionLock() (func(), error) {
	path, e := windows.UTF16PtrFromString(s.path + ".execution.lock")
	if e != nil {
		return nil, errors.New("store: cannot acquire execution lock")
	}
	handle, e := windows.CreateFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_ALWAYS, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		return nil, errors.New("store: cannot acquire execution lock")
	}
	f := os.NewFile(uintptr(handle), "execution lock")
	var info windows.ByHandleFileInformation
	if e = windows.GetFileInformationByHandle(handle, &info); e != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		_ = f.Close()
		return nil, errors.New("store: unsafe execution lock")
	}
	ov := new(windows.Overlapped)
	if e = windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ov); e != nil {
		_ = f.Close()
		return nil, errors.New("store: execution or recovery already active")
	}
	return func() { _ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ov); _ = f.Close() }, nil
}

// ExecutionLockHeld probes whether a writer currently holds the
// execution/recovery lock, without acquiring it. It never blocks and never
// persists a lock. Three outcomes are distinguished and callers MUST treat
// them differently:
//
//	(false, nil)  — confirmed free: the lock file does not exist (acquisition
//	                always creates it) or the probe took and released the lock;
//	(true, nil)   — confirmed held: the probe was refused with a lock violation;
//	(false, err)  — unknown: the lock file exists but is not accessible. The
//	                caller must refuse, never proceed as if the lock were free.
func (s *SQLiteStore) ExecutionLockHeld() (bool, error) {
	path, e := windows.UTF16PtrFromString(s.path + ".execution.lock")
	if e != nil {
		return false, errors.New("store: execution lock state unknown")
	}
	handle, e := windows.CreateFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if e != nil {
		if errors.Is(e, windows.ERROR_FILE_NOT_FOUND) || errors.Is(e, windows.ERROR_PATH_NOT_FOUND) {
			// No lock file: acquisition creates one before taking the lock,
			// so no writer can hold the lock.
			return false, nil
		}
		return false, errors.New("store: execution lock state unknown")
	}
	f := os.NewFile(uintptr(handle), "execution lock probe")
	defer f.Close()
	ov := new(windows.Overlapped)
	if e := windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, ov); e != nil {
		if errors.Is(e, windows.ERROR_LOCK_VIOLATION) {
			return true, nil // a writer holds it
		}
		return false, errors.New("store: execution lock state unknown")
	}
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ov)
	return false, nil
}
