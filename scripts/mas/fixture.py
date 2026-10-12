#!/usr/bin/env python3
"""Independent artificial fixture; private manifest never belongs in Git."""
import argparse, hashlib, json, os, pathlib, subprocess, tempfile
p=argparse.ArgumentParser()
p.add_argument('action',choices=['prepare','serve','mount','unmount','verify'])
p.add_argument('--manifest',default='/tmp/ndg-mas-phase0-private.json')
p.add_argument('--port',type=int,default=2445)
p.add_argument('--read-delay-ms',type=int,default=0,choices=range(0,251),help='Disposable server SMB READ delay only; no product injection')
a=p.parse_args();dest=pathlib.Path(a.manifest)
if a.action=='prepare':
    if dest.exists(): raise SystemExit('Manifest exists; choose a new filename. No overwrite.')
    root=pathlib.Path(tempfile.mkdtemp(prefix='ndg-mas-phase0-'));root.chmod(0o700)
    for name in ['local','export','mount','evidence']: (root/name).mkdir(mode=0o700)
    for name in ['local','export']:
        for i in range(3):
            for j in range(2): (root/name/f'pair-{i}-{j}.bin').write_bytes(f'disposable-pair-{i}-'.encode()*22000)
        (root/name/'singleton.bin').write_bytes(b'disposable-singleton')
    (root/'unselected-test.bin').write_bytes(b'artificial boundary canary')
    m={'root':str(root),'port':a.port,'share':'NDG_MAS_PHASE0',
       'baseline':{n:{f.name:hashlib.sha256(f.read_bytes()).hexdigest() for f in (root/n).iterdir()} for n in ['local','export']}}
    fd=os.open(dest,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    with os.fdopen(fd,'w') as f: json.dump(m,f)
    print('Prepared independent 7-file local and SMB sources; private manifest:',dest)
else:
    m=json.loads(dest.read_text());root=pathlib.Path(m['root'])
    if not root.name.startswith('ndg-mas-phase0-') or not (root/'evidence').is_dir(): raise SystemExit('Not a Phase 0 fixture')
    if a.action=='serve':
        from impacket.smbserver import SimpleSMBServer
        s=SimpleSMBServer(listenAddress='127.0.0.1',listenPort=m['port'])
        s.addShare(m['share'],str(root/'export'),'Disposable read-only phase0',readOnly='yes')
        if a.read_delay_ms:
            import time
            from impacket import smb3structs
            server=s.getServer()
            def delayed_read(*args,**kwargs):
                time.sleep(a.read_delay_ms/1000)
                return original_read(*args,**kwargs)
            original_read=server.hookSmb2Command(smb3structs.SMB2_READ,delayed_read)
            if original_read is None: raise SystemExit('SMB read hook unavailable; refuse pretend delay')
        s.setSMB2Support(True);s.setLogFile(str(root/'evidence'/'smb-private.log'));s.start()
    elif a.action=='mount':
        subprocess.run(['/sbin/mount_smbfs','-N',f'//guest@127.0.0.1:{m["port"]}/{m["share"]}',str(root/'mount')],check=True)
    elif a.action=='unmount':
        subprocess.run(['/sbin/umount',str(root/'mount')],check=True)
    else:
        for name in ['local','export']:
            actual={f.name:hashlib.sha256(f.read_bytes()).hexdigest() for f in (root/name).iterdir()}
            if actual!=m['baseline'][name]: raise SystemExit('FAIL: synthetic source bytes or coverage changed')
        print('PASS: 14 synthetic source files unchanged')
