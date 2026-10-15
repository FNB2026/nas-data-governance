#!/usr/bin/env python3
"""Explicit disposable different-device mount experiment; no source arguments.

Creates and detaches only its own read-only disk image. No sudo, force detach,
existing mount, source directory or database is used. Normal output is fixed
booleans. A failure retains the fixture; never delete a still-mounted tree.
"""
import json
import os
from pathlib import Path
import plistlib
import shutil
import subprocess
import sys
import tempfile

C_SOURCE = r'''
#include <fcntl.h>
#include <sys/attr.h>
#include <sys/mount.h>
#include <sys/stat.h>
#include <unistd.h>
#include <stdio.h>
#include <string.h>
int main(int argc, char **argv) {
    if (argc != 2) return 2;
    int root = open(argv[1], O_RDONLY|O_DIRECTORY|O_CLOEXEC|O_NOFOLLOW_ANY);
    if (root < 0) return 3;
    int flags = O_RDONLY|O_CLOEXEC|O_NOFOLLOW_ANY|O_RESOLVE_BENEATH;
    int dir = openat(root, "nested", flags|O_DIRECTORY);
    int file = openat(root, "nested/probe.txt", flags);
    if (dir < 0 || file < 0) return 4;
    struct stat a, b;
    struct statfs fa, fb;
    if (fstat(root,&a) || fstat(file,&b) || fstatfs(root,&fa) || fstatfs(file,&fb)) return 5;
    struct attrlist list = {0}; list.bitmapcount = ATTR_BIT_MAP_COUNT;
    list.dirattr = ATTR_DIR_MOUNTSTATUS;
    struct { unsigned int size, status; } attrs = {0};
    int attr_ok = fgetattrlist(dir,&list,&attrs,sizeof(attrs),0)==0 && attrs.size==sizeof(attrs);
    char bytes[16] = {0}; ssize_t n=read(file,bytes,sizeof(bytes));
    int correct = n==10 && memcmp(bytes,"synthetic\n",10)==0;
    printf("{\"whole_root_flags_crossed_nested_mount\":true,"
           "\"different_device_observed\":%s,\"different_fsid_observed\":%s,"
           "\"mountstatus_root_flag_observed\":%s,\"known_bytes_read\":%s,"
           "\"same_device_nested_mount_tested\":false,\"mount_boundary_proved\":false}\n",
           a.st_dev!=b.st_dev?"true":"false",
           memcmp(&fa.f_fsid,&fb.f_fsid,sizeof(fa.f_fsid))?"true":"false",
           attr_ok && (attrs.status & DIR_MNTSTATUS_MNTPOINT)?"true":"false",
           correct?"true":"false");
    close(file); close(dir); close(root);
    return correct && a.st_dev!=b.st_dev && attr_ok ? 0 : 6;
}
'''


def run(args):
    return subprocess.run(args, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)


def main():
    if sys.platform != "darwin" or len(sys.argv) != 1:
        raise RuntimeError("Darwin only; no arguments accepted")
    os.umask(0o077)
    fixture = Path(tempfile.mkdtemp(prefix="ndg-darwin-mount-", dir="/private/tmp"))
    root = fixture / "root"
    nested = root / "nested"
    nested.mkdir(parents=True, mode=0o700)
    seed = fixture / "seed"
    seed.mkdir(mode=0o700)
    (seed / "probe.txt").write_bytes(b"synthetic\n")
    code, binary, image = (fixture / x for x in ("probe.c", "probe", "fixture.dmg"))
    code.write_text(C_SOURCE)
    attached = False
    complete = False
    try:
        run(["xcrun", "clang", "-Wall", "-Wextra", "-Werror", str(code), "-o", str(binary)])
        run(["hdiutil", "create", "-size", "16m", "-fs", "HFS+", "-srcfolder", str(seed),
             "-volname", "NDGProof", "-format", "UDZO", "-o", str(image)])
        reply = run(["hdiutil", "attach", "-readonly", "-nobrowse", "-mountpoint", str(nested), "-plist", str(image)])
        attached = True
        mounts = plistlib.loads(reply.stdout).get("system-entities", [])
        if sum(x.get("mount-point") == str(nested) for x in mounts) != 1:
            raise RuntimeError("unconfirmed fixture mount")
        result = json.loads(run([str(binary), str(root)]).stdout)
        if not result["whole_root_flags_crossed_nested_mount"] or not result["known_bytes_read"]:
            raise RuntimeError("unexpected primitive result")
        complete = True
    finally:
        if attached:
            # This exact new fixture mountpoint is the only detach target.
            run(["hdiutil", "detach", str(nested)])
            attached = False
        if complete and not attached:
            shutil.rmtree(fixture)
    print(json.dumps(result, sort_keys=True))


if __name__ == "__main__":
    try:
        main()
    except Exception:
        print("DISPOSABLE_MOUNT_PROBE_FAILED: private fixture retained; no proof claimed", file=sys.stderr)
        sys.exit(1)
