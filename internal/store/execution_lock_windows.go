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
