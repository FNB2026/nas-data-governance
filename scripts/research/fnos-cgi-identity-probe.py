#!/usr/bin/env python3
"""Stage one new, local-only fnOS CGI identity probe. Never installs or contacts NAS.

The CGI reads only its own process identity. It does not mount data, connect to
Docker/backend, accept commands or report request headers/NAS cookies. A runtime
that differs from its dedicated package user returns 503 rather than continuing.
"""
import argparse
import hashlib
import json
from pathlib import Path
import secrets
import shutil

CGI = '''#!/usr/bin/python3
import grp,json,os,pwd
expected=pwd.getpwnam(PACKAGE_USER)
expected_group=grp.getgrnam(PACKAGE_USER)
fields={}
for line in open('/proc/self/status'):
 key,_,value=line.partition(':')
 if key in ('Uid','Gid','Groups','CapEff','CapPrm','NoNewPrivs'):
  fields[key]=value.strip()
uids=os.getresuid()
gids=os.getresgid()
valid=(expected.pw_uid!=0 and expected.pw_gid!=0
       and expected_group.gr_gid==expected.pw_gid
       and all(u==expected.pw_uid for u in uids)
       and all(g==expected.pw_gid for g in gids)
       and set(os.getgroups())<=set([expected.pw_gid])
       and int(fields.get('CapEff','1'),16)==0
       and int(fields.get('CapPrm','1'),16)==0)
if os.environ.get('REQUEST_METHOD') not in ('GET','HEAD') or os.environ.get('QUERY_STRING'):
 print('Status: 400 Bad Request')
 valid=False
elif not valid:
 print('Status: 503 Service Unavailable')
print('Content-Type: application/json; charset=utf-8')
print('Cache-Control: no-store')
print("Content-Security-Policy: default-src 'none'; frame-ancestors 'none'")
print('X-Content-Type-Options: nosniff')
print()
if os.environ.get('REQUEST_METHOD')!='HEAD':
 print(json.dumps({'scope':'PROCESS_IDENTITY_PROBE_ONLY_NO_BACKEND_OR_SOURCE_ACCESS',
                   'runtime_package_identity':'MATCH' if valid else 'REJECTED',
                   'expected_uid':expected.pw_uid,'expected_gid':expected.pw_gid,
                   'expected_named_group_gid':expected_group.gr_gid,
                   'process':fields}))
'''


def stage(folder, icon):
    folder.mkdir(mode=0o700, parents=False, exist_ok=False)
    nonce = secrets.token_hex(3)
    app = 'ndg-cgi-probe-' + nonce
    user = 'ndgprobe' + nonce
    files = {
        'manifest': '\n'.join(['appname='+app, 'version=0.0.1', 'source=thirdparty',
                              'display_name=NDG CGI Identity Probe', 'platform=x86',
                              'maintainer=FNB2026', 'maintainer_url=https://github.com/FNB2026/nas-data-governance',
                              'desc=Local disposable identity experiment only. No backend or file access.',
                              'desktop_uidir=ui', 'desktop_applaunchname='+app+'.main', 'ctl_stop=true', '']) ,
        'config/privilege': json.dumps({'defaults':{'run-as':'package'}, 'username':user, 'groupname':user})+'\n',
        'config/resource': '{}\n',
        'app/ui/config': json.dumps({'.url':{app+'.main':{'title':'NDG CGI Identity Probe',
            'icon':'images/icon_{0}.png','type':'url','protocol':'','port':'',
            'url':'/cgi/ThirdParty/'+app+'/index.cgi/','allUsers':False,
            'control':{'accessPerm':'readonly'}}}})+'\n',
        'app/ui/index.cgi': CGI.replace('PACKAGE_USER', repr(user)),
        'cmd/main': '#!/bin/sh\n# A stateless CGI probe has no background daemon.\ncase "${1:-}" in start|stop|status) exit 0;; *) exit 1;; esac\n',
    }
    for hook in ('install_init', 'install_callback', 'uninstall_init',
                 'uninstall_callback', 'upgrade_init', 'upgrade_callback',
                 'config_init', 'config_callback'):
        files['cmd/' + hook] = '#!/bin/sh\n# Stateless identity probe: no lifecycle side effects.\nexit 0\n'
    for name, content in files.items():
        path = folder / name
        path.parent.mkdir(mode=0o755, parents=True, exist_ok=True)
        path.write_text(content)
        path.chmod(0o755 if name == 'app/ui/index.cgi' or name.startswith('cmd/') else 0o644)
    for name in ('ICON.PNG','app/ui/images/icon_64.png','app/ui/images/icon_256.png','ICON_256.PNG'):
        source = icon / ('ICON_256.PNG' if '256' in name else 'ICON.PNG')
        path = folder / name
        path.parent.mkdir(mode=0o755, parents=True, exist_ok=True)
        shutil.copyfile(source, path)
        path.chmod(0o644)
    inventory = {str(p.relative_to(folder)):hashlib.sha256(p.read_bytes()).hexdigest()
                 for p in sorted(folder.rglob('*')) if p.is_file()}
    return {'app':app,'package_user':user,'url_path':'/cgi/ThirdParty/'+app+'/index.cgi/',
            'files':inventory,'scope':'LOCAL_IDENTITY_PROBE_NOT_A_PRODUCT_OR_RELEASE'}


if __name__ == '__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--stage-only',type=Path,required=True)
    args=parser.parse_args()
    icon=Path(__file__).resolve().parents[2]/'packaging/fnos/ndg-readonly-test'
    print(json.dumps(stage(args.stage_only,icon),indent=2))
