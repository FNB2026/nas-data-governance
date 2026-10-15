#!/usr/bin/env python3
"""Check current public docs against pinned identities, optionally live services.

This checks release/registry identity, not host/GUI functionality or new release
permission. Network/auth/429 errors are UNKNOWN and never called dead links.
"""
import argparse
import hashlib
import json
import os
import re
from pathlib import Path
import subprocess
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
REPO = 'FNB2026/nas-data-governance'


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


def get(url, github=False):
    headers = {'Accept': 'application/vnd.github+json' if github else 'application/vnd.oci.image.index.v1+json'}
    if github and os.environ.get('GH_TOKEN'):
        headers['Authorization'] = 'Bearer ' + os.environ['GH_TOKEN']
    with urllib.request.urlopen(urllib.request.Request(url, headers=headers), timeout=30) as response:
        return response.read()


def offline(pins):
    readme = (ROOT / 'README.md').read_text()
    guide = (ROOT / 'docs/multichannel/DOCKER-READONLY-PREVIEW.md').read_text()
    for c in pins['channels'].values():
        require(c['release_url'] in readme, 'README release link mismatch')
        require(c['prerelease'] is True and len(c['source_commit']) == 40, 'Invalid public identity pin')
    c = pins['channels']['docker']
    require(c['index_digest'] in readme and c['index_digest'] in guide, 'Docker documentation digest mismatch')
    require(c['tag'] == 'v' + (ROOT / 'packaging/docker/PREVIEW_VERSION').read_text().strip(), 'Preview version mismatch')
    tracked = set(subprocess.check_output(['git', 'ls-files', '-z'], cwd=ROOT).decode().split('\0'))
    for name in ('cmd/ndg-desktop/build/appicon.png', 'docs/assets/ndg-overview.png', 'docs/assets/ndg-duplicates.png'):
        require(name in tracked and (ROOT / name).is_file() and name in readme, 'Missing/untracked README asset')
    # Ordinary development VERSION may advance independently of public channels.


def online(pins):
    published = json.loads(get('https://api.github.com/repos/' + REPO + '/releases?per_page=100', True))
    patterns = {'macos': r'^v[0-9]+\.[0-9]+\.[0-9]+(?:-beta\.[0-9]+)?$',
                'docker': r'^v[0-9]+\.[0-9]+\.[0-9]+-docker-preview\.[0-9]+$'}
    for name, c in pins['channels'].items():
        candidates = [r for r in published if not r['draft'] and re.fullmatch(patterns[name], r['tag_name'])]
        require(candidates and max(candidates, key=lambda r: r['published_at'])['tag_name'] == c['tag'],
                'Published channel advanced; update current README and identity ledger')
        release = json.loads(get('https://api.github.com/repos/' + REPO + '/releases/tags/' + c['tag'], True))
        require(not release['draft'] and release['prerelease'] == c['prerelease']
                and release['id'] == c['release_id'], 'Public release identity mismatch')
        assets = {a['name']: {'id': a['id'], 'sha256': a['digest']} for a in release['assets']}
        require(assets == c['assets'], 'Frozen release asset inventory mismatch')
        ref = json.loads(get('https://api.github.com/repos/' + REPO + '/git/ref/tags/' + c['tag'], True))
        obj = ref['object']
        require({'type': obj['type'], 'sha': obj['sha']} == c['tag_object'], 'Immutable tag object mismatch')
        for _ in range(4):
            if obj['type'] == 'commit':
                break
            require(obj['type'] == 'tag', 'Unsupported tag target')
            obj = json.loads(get('https://api.github.com/repos/' + REPO + '/git/tags/' + obj['sha'], True))['object']
        require(obj['type'] == 'commit' and obj['sha'] == c['source_commit'], 'Public tag source mismatch')
    c = pins['channels']['docker']
    token = json.loads(get('https://ghcr.io/token?service=ghcr.io&scope=repository:fnb2026/ndg-readonly-preview:pull'))['token']
    request = urllib.request.Request('https://ghcr.io/v2/fnb2026/ndg-readonly-preview/manifests/' + c['tag'][1:],
                                    headers={'Authorization': 'Bearer ' + token, 'Accept': 'application/vnd.oci.image.index.v1+json'})
    with urllib.request.urlopen(request, timeout=30) as response:
        raw = response.read()
    require('sha256:' + hashlib.sha256(raw).hexdigest() == c['index_digest'], 'Public registry index mismatch')
    descriptors = json.loads(raw)['manifests']
    require(len(descriptors) == 2 and {d['platform']['architecture']: d['digest'] for d in descriptors} == c['manifests']
            and all(d['platform']['os'] == 'linux' for d in descriptors), 'Public architecture identities mismatch')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--online', action='store_true')
    args = parser.parse_args()
    pins = json.loads((ROOT / 'docs/public-channels.json').read_text())
    offline(pins)
    if args.online:
        try:
            online(pins)
        except (OSError, ValueError, urllib.error.URLError) as error:
            # No raw error body or credential-bearing request is logged.
            print(json.dumps({'status': 'UNKNOWN_NETWORK_OR_AUTH', 'error_class': type(error).__name__}))
            return 2
    print(json.dumps({'status': 'LIVE_IDENTITY_PASS' if args.online else 'OFFLINE_DOCS_PASS',
                      'scope': 'public release identities and current documentation; not product acceptance'}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
