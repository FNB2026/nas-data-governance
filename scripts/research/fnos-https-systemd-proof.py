#!/usr/bin/env python3
"""Disposable GitHub-hosted systemd proof, never NAS/product acceptance.

Real counterexamples run first. Their synthetic writes must succeed before a
revised sandbox refusal can count. No business data or public listener is used.
"""
import array
import ctypes
import errno
import grp
import hashlib
import http.client
import json
import os
from pathlib import Path
import platform
import pwd
import select
import shutil
import signal
import socket
import ssl
import stat
import subprocess
import sys
import time
import uuid

PREFIX='ndg-ci-systemd-'
MARKER='NDG_DISPOSABLE_SYSTEMD_PROOF_V1'
LIBC=ctypes.CDLL(None,use_errno=True)
FROZEN='cde4c012393f6b58787aeed821101319680d7763'
HASHES={'x86_64':'f73963c9e8381654654963f08f3622530dfda33dd4e05a6033b9215a3f37fbe8',
        'aarch64':'a1ad1dc0f4fd98f6c87e11422e679a60438dbaca0ef2ec06e506e367d78f07f2'}
class How(ctypes.Structure):
    _fields_=[('flags',ctypes.c_uint64),('mode',ctypes.c_uint64),('resolve',ctypes.c_uint64)]
def syscall(number,*args):
    ctypes.set_errno(0)
    value=LIBC.syscall(number,*args)
    if value<0: raise OSError(ctypes.get_errno(),'synthetic syscall refusal')
    return value
def rooted(fd,name,flags,mode=0):
    how=How(flags,mode,13)
    return syscall(437,fd,name.encode(),ctypes.byref(how),ctypes.sizeof(how))
def fixture(root):
    assert not Path('/usr/trim').exists(), 'never run this proof on fnOS'
    assert root.parent==Path('/var/lib') and root.name.startswith(PREFIX)
    s=root.lstat()
    assert stat.S_ISDIR(s.st_mode) and s.st_uid==0 and not root.is_symlink()
    assert (root/'marker').read_text()==MARKER
def outcome(fn):
    try:
        fn()
        return {'ok':True}
    except OSError as e:
        return {'ok':False,'errno':e.errno}
def denied(fn,allowed):
    got=outcome(fn)
    assert not got['ok'] and got['errno'] in allowed, 'kernel refusal missing'
    return got['errno']
def unshare_user():
    if LIBC.unshare(0x10000000)<0:
        raise OSError(ctypes.get_errno(),'namespace denied')
def mutate_fd(fd):
    os.write(fd,b'artificial-write')
    os.pwrite(fd,b'artificial-pwrite',0)
    os.ftruncate(fd,48)
    os.fchmod(fd,0o4755)
def alias_write(path):
    f=os.open(path,os.O_RDWR|os.O_CLOEXEC)
    try: mutate_fd(f)
    finally: os.close(f)
def emit(data): print('NDG_SYSTEMD_PROOF '+json.dumps(data,sort_keys=True),flush=True)

def audit_extra_fds():
    for n in os.listdir('/proc/self/fd'):
        if int(n)<=2: continue
        try: os.fstat(int(n))
        except OSError as e:
            if e.errno==errno.EBADF: continue  # closed directory enumeration FD
            raise
        raise AssertionError('unexpected inherited descriptor')

def audit_standard_fds():
    audit_extra_fds()
    null=os.stat('/dev/null'); standard=os.fstat(0)
    assert stat.S_ISCHR(standard.st_mode) and standard.st_rdev==null.st_rdev
    flags=int(next(x for x in Path('/proc/self/fdinfo/0').read_text().splitlines() if x.startswith('flags:')).split()[1],8)
    assert flags&os.O_ACCMODE==os.O_RDONLY
    first=os.fstat(1); second=os.fstat(2)
    assert stat.S_ISSOCK(first.st_mode) and (first.st_dev,first.st_ino)==(second.st_dev,second.st_ino)
    with socket.socket(fileno=os.dup(1)) as journal:
        assert journal.family==socket.AF_UNIX and journal.type==socket.SOCK_STREAM
        assert journal.getpeername()=='/run/systemd/journal/stdout'
        import struct
        pid,peer_uid,peer_gid=struct.unpack('3i',journal.getsockopt(socket.SOL_SOCKET,socket.SO_PEERCRED,12))
        # Socket activation can retain PID 1's listen-time credentials.
        assert pid>=1 and peer_uid==0 and peer_gid==0
    return {'stdin':'readonly_dev_null','stdout_stderr':'same_root_journald_stream','extra_descriptors':0}

def peer(root,uid,gid):
    fixture(root)
    os.setgroups([gid]); os.setgid(gid); os.setuid(uid)
    assert LIBC.prctl(38,1,0,0,0)==0 and LIBC.prctl(4,1,0,0,0)==0
    os.chdir(root/'owned')
    file=os.open('item',os.O_RDWR|os.O_CLOEXEC)
    mem=os.memfd_create('ndg-artificial-peer',os.MFD_CLOEXEC)
    os.write(mem,b'artificial-memfd')
    server=socket.socket(socket.AF_UNIX,socket.SOCK_STREAM)
    server.bind('\0'+root.name); server.listen(2)
    print(json.dumps({'pid':os.getpid(),'file_fd':file,'memfd':mem,'userns':os.stat('/proc/self/ns/user').st_ino}),flush=True)
    while True:
        client,_=server.accept()
        with client:
            assert client.recv(1)==b'F'
            client.sendmsg([b'F'],[(socket.SOL_SOCKET,socket.SCM_RIGHTS,array.array('i',[file,mem]))])

def actor(root,kind,uid,gid,peer_info):
    fixture(root)
    assert os.getuid()==uid and os.getgid()==gid
    assert all(x==gid for x in os.getgroups())
    fields=dict(x.split(':',1) for x in Path('/proc/self/status').read_text().splitlines() if ':' in x)
    assert fields['NoNewPrivs'].strip()=='1'
    assert all(int(fields[k].strip(),16)==0 for k in ['CapInh','CapPrm','CapEff','CapAmb'])
    if kind=='original':
        f=os.open('/',os.O_RDONLY|os.O_DIRECTORY)
        try: value=denied(lambda:os.close(rooted(f,'var',os.O_RDONLY|os.O_DIRECTORY)),{errno.ENOSYS})
        finally: os.close(f)
        emit({'profile':kind,'original_openat2_ENOSYS_witness':value==errno.ENOSYS})
        return
    fixed=kind=='revised'
    namespace=os.stat('/proc/self/ns/user').st_ino
    assert (namespace!=peer_info['userns'])==fixed
    # No inherited regular/dir FD is acceptable, even when opened read-only:
    # a namespace-external descriptor might be reopened writable through proc.
    inherited=audit_standard_fds()
    aliases={
        'root':f'/proc/{peer_info["pid"]}/root{root}/owned/item',
        'cwd':f'/proc/{peer_info["pid"]}/cwd/item',
        'fd':f'/proc/{peer_info["pid"]}/fd/{peer_info["file_fd"]}',
    }
    proc={}
    for name,path in aliases.items():
        if fixed: proc[name]=denied(lambda p=path:alias_write(p),{errno.EACCES,errno.EPERM})
        else: alias_write(path); proc[name]='REAL_SYNTHETIC_WRITE_AND_MODE_CHANGE'
    scm={}
    if fixed:
        scm['new_unix_socket']=denied(lambda:socket.socket(socket.AF_UNIX,socket.SOCK_STREAM).close(),{errno.EAFNOSUPPORT,errno.EPERM})
    else:
        c=socket.socket(socket.AF_UNIX,socket.SOCK_STREAM)
        with c:
            c.connect('\0'+root.name); c.sendall(b'F')
            data,ancillary,_,_=c.recvmsg(1,socket.CMSG_SPACE(8))
            assert data==b'F'
            received=[]
            for level,type,body in ancillary:
                assert level==socket.SOL_SOCKET and type==socket.SCM_RIGHTS
                ids=array.array('i'); ids.frombytes(body[:len(body)-len(body)%ids.itemsize]); received.extend(ids)
            assert len(received)==2
            try:
                for f in received:
                    assert stat.S_ISREG(os.fstat(f).st_mode)
                    mutate_fd(f); alias_write('/proc/self/fd/'+str(f))
                scm['regular_and_memfd']='REAL_RECEIVED_FD_WRITE_TRUNCATE_MODE_REOPEN'
            finally:
                for f in received: os.close(f)
    negative={}
    if fixed:
        owned=root/'owned'; item=owned/'item'
        flags=os.O_WRONLY|os.O_CREAT|os.O_EXCL
        for directory in [owned,Path('/tmp'),Path('/var/tmp')]:
            assert os.statvfs(directory).f_flag&os.ST_RDONLY
            negative['create_'+directory.name]=denied(lambda d=directory:os.close(os.open(d/'new',flags,0o6755)),{errno.EROFS})
            negative['tmpfile_'+directory.name]=denied(lambda d=directory:os.close(os.open(d,os.O_TMPFILE|os.O_RDWR,0o6755)),{errno.EROFS})
        for label,fn in {
            'write_open':lambda:os.close(os.open(item,os.O_WRONLY)),
            'truncate':lambda:os.truncate(item,1),
            'chmod':lambda:os.chmod(item,0o6755),
            'rename':lambda:os.rename(item,owned/'renamed'),
            'unlink':lambda:os.unlink(item),
            'hardlink':lambda:os.link(item,owned/'link'),
        }.items(): negative[label]=denied(fn,{errno.EROFS})
        f=os.open(item,os.O_RDONLY)
        d=os.open(owned,os.O_RDONLY|os.O_DIRECTORY)
        try:
            negative['fchmod']=denied(lambda:os.fchmod(f,0o6755),{errno.EROFS})
            negative['fchmodat2']=denied(lambda:syscall(452,d,b'item',0o6755,0),{errno.EROFS})
            negative['openat2_create_mode']=denied(lambda:os.close(rooted(d,'new2',flags,0o6755)),{errno.EROFS})
        finally: os.close(d); os.close(f)
        for directory in [Path('/dev/shm'),Path('/dev/mqueue')]:
            negative['inaccessible_'+directory.name]=denied(lambda d=directory:os.close(os.open(d/'new',flags,0o6755)),{errno.EACCES,errno.EROFS})
        assert os.statvfs('/dev').f_flag&os.ST_RDONLY
        negative['memfd']=denied(lambda:os.close(os.memfd_create('blocked',0)),{errno.EPERM})
        native=platform.machine()
        numbers={'recvmsg':47,'recvmmsg':299,'ptrace':101,'process_vm_readv':310,'process_vm_writev':311} if native=='x86_64' else {'recvmsg':212,'recvmmsg':243,'ptrace':117,'process_vm_readv':270,'process_vm_writev':271}
        numbers.update({'memfd_secret':447,'pidfd_getfd':438,'io_uring_setup':425,'io_uring_enter':426,'io_uring_register':427})
        for label,num in numbers.items(): negative[label]=denied(lambda n=num:syscall(n,-1,0,0,0,0,0),{errno.EPERM})
        negative['new_user_namespace']=denied(unshare_user,{errno.EPERM})
    emit({'profile':kind,'identity':True,'userns_different':namespace!=peer_info['userns'],'inherited':inherited,'proc':proc,'scm':scm,'negative':negative,'uid_map':Path('/proc/self/uid_map').read_text(),'gid_map':Path('/proc/self/gid_map').read_text()})

def driver(unit_path,binary,evidence):
    assert os.geteuid()==0 and os.environ.get('GITHUB_ACTIONS')=='true'
    assert os.environ.get('RUNNER_ENVIRONMENT')=='github-hosted' and not Path('/usr/trim').exists()
    assert evidence==Path('/tmp/ndg-systemd-evidence') and not os.path.lexists(evidence)
    evidence.mkdir(mode=0o755)
    progress={'result':'RUNNING','phase':'frozen_identity','cleanup':'NOT_STARTED','proofs':{}}
    def save_progress():
        (evidence/'result.json').write_text(json.dumps(progress,sort_keys=True,indent=2)+'\n')
    save_progress()
    machine=platform.machine(); assert machine in HASHES
    assert hashlib.sha256(binary.read_bytes()).hexdigest()==HASHES[machine]
    assert (binary.parent/'source-sha.txt').read_text().strip()==FROZEN
    template=unit_path.read_text()
    assert 'RestrictSUIDSGID=' not in template and 'PrivateUsers=true' in template
    root=Path('/var/lib')/(PREFIX+uuid.uuid4().hex[:12]); root.mkdir(mode=0o755)
    (root/'marker').write_text(MARKER)
    app=Path('/var/apps/ndg-readonly-web'); assert not os.path.lexists(app)
    name='ndgreadonlyweb'; base_name='ndgci-base'
    for n in [name,base_name]:
        try: grp.getgrnam(n)
        except KeyError: pass
        else: raise AssertionError('preexisting test group')
    try: pwd.getpwnam(name)
    except KeyError: pass
    else: raise AssertionError('preexisting test user')
    used={u.pw_uid for u in pwd.getpwall()}|{g.gr_gid for g in grp.getgrall()}
    uid=next(n for n in range(21000,22000,3) if not {n,n+1,n+2}&used); gid=uid+1
    created_groups=[]; user_created=False; app_inode=None; peer_process=None; units=[]; outputs={}
    def command(*args,timeout=30,check=True):
        p=subprocess.run(args,capture_output=True,timeout=timeout)
        if check: assert p.returncode==0,'disposable command failed'
        return p
    def unit_file(label,text):
        n=root.name+'-'+label+'.service'; p=Path('/etc/systemd/system')/n
        assert not os.path.lexists(p)
        state=command('systemctl','show',n,'--property=LoadState,ActiveState,FragmentPath,MainPID').stdout.decode()
        assert dict(x.split('=',1) for x in state.splitlines())=={'LoadState':'not-found','ActiveState':'inactive','FragmentPath':'','MainPID':'0'}
        p.write_text(text); p.chmod(0o644); units.append((n,p,hashlib.sha256(text.encode()).hexdigest(),p.stat().st_ino))
        checked=command('systemd-analyze','verify',str(p))
        assert not any(x in checked.stderr.decode().lower() for x in ['not known','unknown system call','unknown key','failed to parse'])
        command('systemctl','daemon-reload')
        return n
    def state(item):
        s=item.stat();return {'hash':hashlib.sha256(item.read_bytes()).hexdigest(),'mode':stat.S_IMODE(s.st_mode),'inode':s.st_ino,'size':s.st_size}
    def actor_unit(kind,profile,peer_info):
        progress['phase']=kind; save_progress()
        # The peer JSON contains only integers. systemd unquotes double quotes;
        # pass its JSON as an escaped single quoted argument.
        args=' '.join([str(root/'proof.py'),'--actor',str(root),kind,str(uid),str(gid),"'"+json.dumps(peer_info,separators=(',',':'))+"'"])
        text=profile.replace('Type=simple','Type=oneshot').replace('ExecStart=/var/apps/ndg-readonly-web/cmd/ndg-fnos-https','ExecStart=/usr/bin/python3 '+args).replace('Restart=on-failure\n','').replace('StandardOutput=null','StandardOutput=journal')
        n=unit_file(kind,text); result=command('systemctl','start',n,timeout=50,check=False)
        journal=command('journalctl','-u',n,'-o','cat','--no-pager').stdout.decode()
        (evidence/(kind+'-journal.txt')).write_text(journal)
        assert result.returncode==0,'proof service failed'
        rows=[json.loads(x.split('NDG_SYSTEMD_PROOF ',1)[1]) for x in journal.splitlines() if 'NDG_SYSTEMD_PROOF ' in x]
        assert len(rows)==1 and rows[0]['profile']==kind
        outputs[kind]=rows[0]
        progress['proofs'][kind]=rows[0];save_progress()
    try:
        progress['phase']='trusted_journald_endpoint';save_progress()
        journal_socket=command('systemctl','show','systemd-journald.socket','--property=ActiveState,Listen,FragmentPath,DropInPaths').stdout.decode()
        journal_fields=dict(x.split('=',1) for x in journal_socket.splitlines())
        assert journal_fields['ActiveState']=='active' and '/run/systemd/journal/stdout (Stream)' in journal_fields['Listen']
        assert not journal_fields['DropInPaths']
        journal_unit=Path(journal_fields['FragmentPath']).resolve(strict=True)
        assert journal_unit==Path('/usr/lib/systemd/system/systemd-journald.socket').resolve(strict=True)
        for path in [journal_unit,*journal_unit.parents]:
            metadata=path.lstat()
            assert metadata.st_uid==0 and not metadata.st_mode&0o022 and not path.is_symlink()
        for path in [Path('/run'),Path('/run/systemd'),Path('/run/systemd/journal')]:
            metadata=path.lstat()
            assert metadata.st_uid==0 and stat.S_ISDIR(metadata.st_mode) and not metadata.st_mode&0o022 and not path.is_symlink()
        endpoint=Path('/run/systemd/journal/stdout').lstat()
        assert endpoint.st_uid==0 and stat.S_ISSOCK(endpoint.st_mode)
        progress['proofs']['journal_socket']={**journal_fields,'fragment_sha256':hashlib.sha256(journal_unit.read_bytes()).hexdigest()};save_progress()
        progress['phase']='inherited_fd_audit_counterexamples';save_progress()
        audit_command=['python3',str(Path(__file__).resolve()),'--fd-audit']
        clean=command(*audit_command);assert clean.stdout.strip()==b'NO_EXTRA_FDS'
        ro=os.open(binary,os.O_RDONLY|os.O_CLOEXEC)
        mem=os.memfd_create('artificial-readonly-inheritance',os.MFD_CLOEXEC)
        ro_mem=os.open('/proc/self/fd/'+str(mem),os.O_RDONLY|os.O_CLOEXEC)
        try:
            for label,descriptor in [('readonly_regular',ro),('readonly_memfd',ro_mem)]:
                rejected=subprocess.run(audit_command,pass_fds=(descriptor,),capture_output=True,timeout=5)
                assert rejected.returncode==17 and rejected.stdout.strip()==b'REJECTED_UNEXPECTED_INHERITED_FD'
                progress['proofs'][label]='ACTUALLY_INHERITED_AND_REJECTED'
        finally:os.close(ro);os.close(ro_mem);os.close(mem)
        progress['phase']='synthetic_fixture';save_progress()
        command('groupadd','--gid',str(gid),name);created_groups.append((name,gid))
        command('groupadd','--gid',str(uid+2),base_name);created_groups.append((base_name,uid+2))
        command('useradd','--uid',str(uid),'--gid',base_name,'--groups',name,'--no-create-home','--shell','/usr/sbin/nologin',name);user_created=True
        owned=root/'owned';owned.mkdir(mode=0o700);os.chown(owned,uid,gid)
        item=owned/'item';item.write_bytes(b'artificial-original-data');os.chown(item,uid,gid);item.chmod(0o640)
        # A real ordinary owner must be able to mutate this fixture before any
        # service refusal is interpreted as a namespace/security result.
        baseline_code='import os,sys;f=os.open(sys.argv[1],os.O_RDWR);os.pwrite(f,b"baseline",0);os.fchmod(f,0o4755);os.fchmod(f,0o640);os.close(f)'
        command('setpriv','--reuid='+str(uid),'--regid='+str(gid),'--clear-groups','--','python3','-c',baseline_code,str(item))
        item.write_bytes(b'artificial-original-data');before=state(item)
        (root/'proof.py').write_bytes(Path(__file__).read_bytes());(root/'proof.py').chmod(0o644)
        peer_process=subprocess.Popen(['python3',str(root/'proof.py'),'--peer',str(root),str(uid),str(gid)],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
        assert select.select([peer_process.stdout],[],[],5)[0]
        info=json.loads(peer_process.stdout.readline());assert info['pid']==peer_process.pid
        original=template.replace('PrivateUsers=true\n','').replace('TemporaryFileSystem=/tmp:ro,nosuid,nodev,noexec /var/tmp:ro,nosuid,nodev,noexec\nReadOnlyPaths=/dev\nInaccessiblePaths=/dev/shm /dev/mqueue\n','PrivateTmp=true\n').replace('RestrictNamespaces=true\n','RestrictSUIDSGID=true\n')
        actor_unit('original',original,info)
        counter=template.replace('PrivateUsers=true\n','').replace('RestrictAddressFamilies=AF_INET\n','RestrictAddressFamilies=AF_INET AF_UNIX\n')
        for call in ['recvmsg','recvmmsg','pidfd_getfd','process_vm_readv','process_vm_writev','ptrace']: counter=counter.replace(' '+call,'')
        actor_unit('counterexample',counter,info);assert peer_process.poll() is None
        changed=state(item);assert changed['hash']!=before['hash'] and changed['mode']&0o4000
        item.write_bytes(b'artificial-original-data');item.chmod(0o640);assert state(item)==before
        actor_unit('revised',template,info);assert peer_process.poll() is None and state(item)==before
        assert not os.path.lexists(root/'owned/new') and not os.path.lexists(root/'owned/new2')
        progress['phase']='frozen_application_tls';save_progress()
        # Actual immutable application in the revised full systemd namespace.
        app.parent.mkdir(mode=0o755,exist_ok=True);app.mkdir(mode=0o755);app_inode=app.stat().st_ino
        cmd=app/'cmd';cmd.mkdir(mode=0o755)
        shutil.copyfile(binary,cmd/'ndg-fnos-https');(cmd/'ndg-fnos-https').chmod(0o755)
        config={'gateway':{'app_name':'ndg-readonly-web','origin':'https://ndg.invalid:24483','upstream':'http://10.98.45.2:8080','package_user':name,'package_group':name},'listen':'127.0.0.1:24483'}
        (cmd/'https.json').write_text(json.dumps(config)+'\n');(cmd/'https.json').chmod(0o644)
        command('openssl','req','-x509','-newkey','ec','-pkeyopt','ec_paramgen_curve:P-256','-nodes','-days','1','-subj','/CN=ndg.invalid','-addext','subjectAltName=DNS:ndg.invalid','-keyout',str(cmd/'tls.key'),'-out',str(cmd/'tls.crt'))
        os.chown(cmd/'tls.key',0,gid);(cmd/'tls.key').chmod(0o640);(cmd/'tls.crt').chmod(0o644)
        context=ssl.create_default_context(cafile=str(cmd/'tls.crt'))
        def request(host='ndg.invalid:24483',path='/',origin=None):
            raw=socket.create_connection(('127.0.0.1',24483),timeout=2)
            tls=context.wrap_socket(raw,server_hostname='ndg.invalid')
            conn=http.client.HTTPConnection('ndg.invalid',24483,timeout=2);conn.sock=tls
            try:
                headers={'Host':host}
                if origin is not None:headers['Origin']=origin
                conn.request('GET',path,headers=headers);r=conn.getresponse();return r.status,r.read(300000)
            finally:conn.close()
        tls_unit=unit_file('tls',template.replace('Restart=on-failure\n',''))
        command('systemctl','start',tls_unit)
        for _ in range(40):
            try:status,body=request();break
            except (ConnectionRefusedError,ConnectionResetError):time.sleep(.05)
        else:raise AssertionError('frozen TLS unavailable')
        assert status==200 and b'NDG' in body
        assert request(host='wrong.invalid:24483')[0]==403
        assert request(origin='https://wrong.invalid:24483')[0]==403
        for route in ['/api/quarantine/execute','/api/restore/execute','/api/purge/execute']:assert request(path=route)==(403,b'{"error":"request_rejected"}')
        pid=int(command('systemctl','show',tls_unit,'--value','--property=MainPID').stdout)
        tasks=list(Path('/proc',str(pid),'task').iterdir());assert tasks
        for task in tasks:
            f=dict(x.split(':',1) for x in (task/'status').read_text().splitlines() if ':' in x)
            assert f['Uid'].split()==[str(uid)]*4 and f['Gid'].split()==[str(gid)]*4
            assert all(x==str(gid) for x in f['Groups'].split()) and f['NoNewPrivs'].strip()=='1'
            assert all(int(f[x].strip(),16)==0 for x in ['CapInh','CapPrm','CapEff','CapAmb'])
        command('systemctl','stop',tls_unit)
        progress['phase']='same_device_same_inode_bind';save_progress()
        # Bind of the certificate onto itself: identical device AND inode, new
        # mount boundary. Positive startup above must not turn this into PASS.
        cert=cmd/'tls.crt';old=cert.stat();command('mount','--bind',str(cert),str(cert))
        try:
            now=cert.stat();assert (old.st_dev,old.st_ino)==(now.st_dev,now.st_ino)
            refused=command('systemctl','start',tls_unit,check=False)
            # Type=simple may report start before process exits; inspect actual
            # terminal process state and never infer refusal from CLI success.
            for _ in range(50):
                active=command('systemctl','show',tls_unit,'--value','--property=ActiveState').stdout.strip()
                if active==b'failed':break
                time.sleep(.05)
            assert active==b'failed'
            assert command('systemctl','show',tls_unit,'--value','--property=ExecMainStatus').stdout.strip()==b'1'
            with socket.socket() as probe:assert probe.connect_ex(('127.0.0.1',24483))!=0
        finally:command('systemctl','stop',tls_unit,check=False);command('umount',str(cert))
        assert state(item)==before
        receipt={'scope':'DISPOSABLE_NATIVE_SYSTEMD_PROOF_NOT_NAS_ACCEPTANCE','result':'PASS','architecture':machine,'frozen_source':FROZEN,'binary_sha256':HASHES[machine],'unit_sha256':hashlib.sha256(template.encode()).hexdigest(),'counterexamples_really_mutated_synthetic_host_object':True,'revised_host_bytes_mode_inode_unchanged':True,'same_device_same_inode_bind_refused':True,'real_tls':True,'all_thread_identity':True,'proofs':outputs,'nas_browser_acceptance':'NOT_RUN'}
        progress['result']='PROOFS_PASSED_CLEANUP_PENDING';save_progress()
    except BaseException as e:
        progress['result']='FAIL';progress['failure']={'type':type(e).__name__,'reason':str(e)};save_progress()
        raise
    finally:
        # Preserve only artificial phase/unit diagnostics, outside all fixture
        # paths. Export before stop/unlink so RED evidence survives cleanup.
        for n,_,_,_ in units:
            journal=command('journalctl','-u',n,'-o','cat','--no-pager',check=False).stdout
            (evidence/(n+'-journal.txt')).write_bytes(journal)
            state_output=command('systemctl','show',n,'--property=LoadState,ActiveState,SubState,MainPID,ExecMainStatus,Result',check=False).stdout
            (evidence/(n+'-state.txt')).write_bytes(state_output)
        progress['cleanup']='STARTED';save_progress()
        for n,p,h,ino in reversed(units):
            assert not p.is_symlink() and p.stat().st_ino==ino and hashlib.sha256(p.read_bytes()).hexdigest()==h
            command('systemctl','stop',n,check=False);p.unlink()
        if units:command('systemctl','daemon-reload')
        if peer_process is not None:
            if peer_process.poll() is None:peer_process.terminate()
            peer_process.communicate(timeout=5)
        if app_inode is not None:
            assert app.lstat().st_ino==app_inode and app.lstat().st_uid==0 and not app.is_symlink();shutil.rmtree(app)
        if user_created:
            assert pwd.getpwnam(name).pw_uid==uid;command('userdel',name)
        for n,g in reversed(created_groups):
            try:found=grp.getgrnam(n)
            except KeyError:continue
            assert found.gr_gid==g;command('groupdel',n)
        fixture(root);shutil.rmtree(root)
        progress['cleanup']='COMPLETE';save_progress()
    progress['result']='PASS';progress['receipt']=receipt;save_progress()
    print(json.dumps(receipt,sort_keys=True))

if __name__=='__main__':
    if len(sys.argv)>1 and sys.argv[1]=='--fd-audit':
        try:audit_extra_fds()
        except AssertionError:
            print('REJECTED_UNEXPECTED_INHERITED_FD');sys.exit(17)
        print('NO_EXTRA_FDS')
    elif len(sys.argv)>1 and sys.argv[1]=='--peer':peer(Path(sys.argv[2]),int(sys.argv[3]),int(sys.argv[4]))
    elif len(sys.argv)>1 and sys.argv[1]=='--actor':actor(Path(sys.argv[2]),sys.argv[3],int(sys.argv[4]),int(sys.argv[5]),json.loads(sys.argv[6]))
    else:
        assert len(sys.argv)==4
        try:
            driver(Path(sys.argv[1]).resolve(strict=True),Path(sys.argv[2]).resolve(strict=True),Path(sys.argv[3]))
        except BaseException as e:
            report=Path('/tmp/ndg-systemd-evidence/result.json')
            if report.is_file():
                result=json.loads(report.read_text())
                result['result']='FAIL'
                result['terminal_failure']={'type':type(e).__name__,'reason':str(e)}
                report.write_text(json.dumps(result,sort_keys=True,indent=2)+'\n')
            raise
