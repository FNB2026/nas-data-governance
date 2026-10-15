#!/usr/bin/env python3
"""One fixed, authorized release: verify bytes, promote without rebuild, verify pull.

Registry mutations exist only in promote_private(), restricted to manual main
Actions dispatch by the owner. This tool never publishes a Release or changes
package visibility, release assets, existing tags, NAS state or Mac releases.
"""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import tarfile
import tempfile
import urllib.error
import urllib.request

REPO = 'FNB2026/nas-data-governance'
VERSION = '0.5.0-docker-preview.1'
TAG = 'v' + VERSION
SOURCE = '1db1e4ab9a1287d3f7f9b801b471b00bfc7c7d1e'
TREE = '23affa62ffa2685e7bced588cf90676e7c6242c2'
RELEASE = 413760377
IMAGE = 'ghcr.io/fnb2026/ndg-readonly-preview'
INDEX = 'sha256:49dada73f1eb9cc74485ed5d0b92023d7cb4c3df462d03e105f00158b91668e1'
MANIFESTS = {
    'amd64': 'sha256:bb6db808bc5a17cf27b951b07406136c6bba3056c89d2aee0268042a683f5be4',
    'arm64': 'sha256:4e13ba64c9b85d8e293b7c544c1a877d73c1a09b56323699e1a84464ed8ed216',
}
CONFIGS = {
    'amd64': 'sha256:2c38ca9493ccebc74cdf0cc84fb23e8a4c59babb1660d4892a2f9a50c12a96d4',
    'arm64': 'sha256:0980c4a5619801be0f21250ad62cdb34551d4ccd4781216942ee636523c11923',
}
PINS = json.loads(Path(__file__).with_name('docker-preview-1-pins.json').read_text())
ARCHIVE = 'ndg-' + VERSION + '-multiarch.oci.tar'


def require(ok, message):
    if not ok:
        raise RuntimeError(message)


def digest(data):
    return 'sha256:' + hashlib.sha256(data).hexdigest()


def api(path, method='GET', payload=None, missing=False):
    headers = {'Authorization': 'Bearer ' + os.environ['GH_TOKEN'],
               'Accept': 'application/vnd.github+json',
               'X-GitHub-Api-Version': '2022-11-28'}
    body = None if payload is None else json.dumps(payload).encode()
    req = urllib.request.Request('https://api.github.com/' + path, data=body,
                                 headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=60) as response:
            return json.load(response)
    except urllib.error.HTTPError as error:
        if missing and error.code == 404:
            return None
        raise RuntimeError('GitHub API failed: HTTP ' + str(error.code)) from None


def verify_index(raw):
    require(digest(raw) == INDEX, 'Multiarch index digest mismatch')
    descriptors = json.loads(raw)['manifests']
    require(len(descriptors) == 2, 'Expected exactly two architectures')
    found = {}
    for descriptor in descriptors:
        p = descriptor['platform']
        require(p['os'] == 'linux' and p['architecture'] not in found,
                'Unexpected or duplicate platform')
        found[p['architecture']] = descriptor['digest']
    require(found == MANIFESTS, 'Architecture manifest identities mismatch')


def verify_assets(folder):
    require({f.name for f in folder.iterdir()} == set(PINS), 'Frozen asset set mismatch')
    for name, expected in PINS.items():
        f = folder / name
        require(f.is_file() and not f.is_symlink(), 'Asset must be a regular file')
        require(hashlib.sha256(f.read_bytes()).hexdigest() == expected,
                'Frozen asset checksum mismatch: ' + name)
    sums = {}
    for line in (folder / 'SHA256SUMS').read_text().splitlines():
        sha, name = line.split('  ', 1)
        require(name not in sums, 'Duplicate SHA256SUMS entry')
        sums[name] = sha
    require(sums == {n: s for n, s in PINS.items() if n != 'SHA256SUMS'},
            'SHA256SUMS does not cover all fourteen other assets')
    candidate = json.loads((folder / 'release-candidate.json').read_text())
    require(candidate['version'] == VERSION and candidate['source']['commit'] == SOURCE
            and candidate['source']['tree'] == TREE
            and candidate['multiarch_index_digest'] == INDEX, 'Candidate identity mismatch')
    require(candidate['files'] == {n: s for n, s in PINS.items()
                                  if n not in ('SHA256SUMS', 'release-candidate.json')},
            'Candidate asset inventory mismatch')
    with tarfile.open(folder / ARCHIVE) as archive:
        blobs = {}
        for member in archive.getmembers():
            name = member.name.removeprefix('./')
            require(not member.issym() and not member.islnk() and '..' not in name.split('/'),
                    'Unsafe OCI member')
            if name.startswith('blobs/sha256/'):
                require(member.isfile() and name not in blobs, 'Invalid OCI blob')
                raw = archive.extractfile(member).read()
                require(digest(raw) == 'sha256:' + name.split('/')[-1], 'Corrupt OCI blob')
                blobs[name] = raw
        def blob(sha):
            return blobs['blobs/sha256/' + sha.split(':')[1]]
        outer = json.load(archive.extractfile('index.json'))
        require(len(outer['manifests']) == 1 and outer['manifests'][0]['digest'] == INDEX,
                'OCI selection does not identify the pinned index')
        verify_index(blob(INDEX))
        for arch, sha in MANIFESTS.items():
            manifest = json.loads(blob(sha))
            require(manifest['config']['digest'] == CONFIGS[arch], 'Config identity mismatch')
            config = json.loads(blob(CONFIGS[arch]))
            require(config['architecture'] == arch and config['os'] == 'linux'
                    and config['config']['User'] == '10001:10001', 'Runtime identity mismatch')
            labels = config['config']['Labels']
            require(labels['org.opencontainers.image.revision'] == SOURCE
                    and labels['org.opencontainers.image.version'] == VERSION,
                    'Source/version label mismatch')
            for layer in manifest['layers']:
                require(len(blob(layer['digest'])) == layer['size'], 'Layer length mismatch')
    return candidate


def download_assets(folder):
    release = api('repos/' + REPO + '/releases/' + str(RELEASE))
    require(release['tag_name'] == TAG and release['target_commitish'] == SOURCE
            and release['prerelease'], 'Release metadata mismatch')
    assets = release['assets']
    require(len(assets) == 15 and {a['name'] for a in assets} == set(PINS),
            'Server asset inventory mismatch')
    for asset in assets:
        require(asset['digest'] == 'sha256:' + PINS[asset['name']], 'Server digest mismatch')
        # gh handles authenticated asset redirect without forwarding credentials
        # to the storage origin; no token is placed in command arguments.
        result = subprocess.run(['gh', 'api', '-H', 'Accept: application/octet-stream',
                                 'repos/' + REPO + '/releases/assets/' + str(asset['id'])],
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
        require(result.returncode == 0, 'Asset download failed')
        require(len(result.stdout) == asset['size'], 'Server asset length mismatch')
        (folder / asset['name']).write_bytes(result.stdout)
    verify_assets(folder)


def run_private(args):
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
    require(result.returncode == 0, 'Registry/transport command failed (output withheld)')
    return result.stdout


def promote_private(folder):
    require(os.environ.get('GITHUB_ACTIONS') == 'true'
            and os.environ.get('GITHUB_EVENT_NAME') == 'workflow_dispatch'
            and os.environ.get('GITHUB_REPOSITORY') == REPO
            and os.environ.get('GITHUB_REF') == 'refs/heads/main'
            and os.environ.get('GITHUB_ACTOR') == 'FNB2026', 'Unauthorized execution context')
    download_assets(folder)
    # Check tag BEFORE any registry or ref mutation; never update an existing ref.
    ref_path = 'repos/' + REPO + '/git/ref/tags/' + TAG
    ref = api(ref_path, missing=True)
    require(ref is not None and ref['object']['type'] == 'commit'
            and ref['object']['sha'] == SOURCE, 'Missing or conflicting tag: STOP')
    package_path = 'users/FNB2026/packages/container/ndg-readonly-preview'
    package = api(package_path, missing=True)
    require(package is None or package['visibility'] == 'private',
            'Promotion requires a private package; use public verification otherwise')
    auth = folder.parent / 'registry-auth.json'
    auth.write_text(json.dumps({'auths': {'ghcr.io': {'auth': base64.b64encode(
        ('FNB2026:' + os.environ['GH_TOKEN']).encode()).decode()}}}))
    auth.chmod(0o600)
    skopeo = ['sudo', 'skopeo']
    try:
        inspect = skopeo + ['inspect', '--raw', '--authfile', str(auth)]
        # A missing tag is permitted, but ANY existing different content is a conflict.
        if package:
            versions = api(package_path + '/versions?per_page=100')
            require(len(versions) < 100, 'Package history requires manual conflict audit')
            for version in versions:
                if VERSION in version['metadata']['container']['tags']:
                    verify_index(run_private(inspect + ['docker://' + IMAGE + ':' + VERSION]))
        # --preserve-digests fails rather than silently converting the frozen OCI bytes.
        run_private(skopeo + ['copy', '--all', '--preserve-digests', '--authfile', str(auth),
                             'oci-archive:' + str(folder / ARCHIVE),
                             'docker://' + IMAGE + ':' + VERSION])
        verify_index(run_private(inspect + ['docker://' + IMAGE + ':' + VERSION]))
        for arch, sha in MANIFESTS.items():
            raw = run_private(inspect + ['docker://' + IMAGE + '@' + sha])
            require(digest(raw) == sha, 'Remote architecture digest mismatch')
            require(json.loads(raw)['config']['digest'] == CONFIGS[arch], 'Remote config mismatch')
        package = api(package_path)
        require(package['visibility'] == 'private', 'Package unexpectedly public: STOP')
        ref = api(ref_path)
        require(ref['object']['type'] == 'commit' and ref['object']['sha'] == SOURCE,
                'Final tag mismatch')
    finally:
        auth.unlink(missing_ok=True)
    return {'status': 'PRIVATE_PROMOTION_VERIFIED', 'asset_count': 15}


def public_verify(folder, arch):
    require(platform.machine() == {'amd64': 'x86_64', 'arm64': 'aarch64'}[arch],
            'Public pull must run on the matching native runner')
    # Anonymous token, no GitHub or Docker login. Request no scopes beyond pull.
    url = ('https://ghcr.io/token?service=ghcr.io&scope=repository:'
           'fnb2026/ndg-readonly-preview:pull')
    with urllib.request.urlopen(url, timeout=60) as response:
        token = json.load(response)['token']
    def registry(kind, reference):
        req = urllib.request.Request('https://ghcr.io/v2/fnb2026/ndg-readonly-preview/'
                                     + kind + '/' + reference, headers={
                                         'Authorization': 'Bearer ' + token,
                                         'Accept': 'application/vnd.oci.image.index.v1+json, '
                                         'application/vnd.oci.image.manifest.v1+json'})
        with urllib.request.urlopen(req, timeout=60) as response:
            return response.read()
    verify_index(registry('manifests', VERSION))
    for key, sha in MANIFESTS.items():
        raw = registry('manifests', sha)
        require(digest(raw) == sha, 'Anonymous manifest mismatch')
        manifest = json.loads(raw)
        require(manifest['config']['digest'] == CONFIGS[key], 'Anonymous config descriptor mismatch')
        for desc in [manifest['config']] + manifest['layers']:
            blob = registry('blobs', desc['digest'])
            require(digest(blob) == desc['digest'] and len(blob) == desc['size'],
                    'Anonymous blob checksum mismatch')
    os.environ['DOCKER_CONFIG'] = str(folder / 'empty-docker-auth')
    Path(os.environ['DOCKER_CONFIG']).mkdir()
    ref = IMAGE + '@' + INDEX
    run_private(['docker', 'pull', '--platform', 'linux/' + arch, ref])
    info = json.loads(run_private(['docker', 'image', 'inspect', ref]))[0]
    require(info['Architecture'] == arch and info['Config']['User'] == '10001:10001',
            'Pulled image platform/user mismatch')
    require(info['Config']['Labels']['org.opencontainers.image.revision'] == SOURCE,
            'Pulled image source mismatch')
    # Engine versions may report a manifest as Id; verify exported raw config and
    # all uncompressed layers instead of equating Id with OCI config digest.
    saved = folder / 'pulled-image.tar'
    run_private(['docker', 'save', '-o', str(saved), ref])
    with tarfile.open(saved) as archive:
        entries = json.load(archive.extractfile('manifest.json'))
        require(len(entries) == 1, 'Unexpected Docker export count')
        entry = entries[0]
        raw = archive.extractfile(entry['Config']).read()
        require(digest(raw) == CONFIGS[arch], 'Pulled config bytes mismatch')
        diffs = json.loads(raw)['rootfs']['diff_ids']
        require(len(diffs) == len(entry['Layers']), 'Pulled layer count mismatch')
        for name, sha in zip(entry['Layers'], diffs):
            require(digest(archive.extractfile(name).read()) == sha, 'Pulled layer bytes mismatch')
    verify_index(registry('manifests', VERSION))
    return {'status': 'ANONYMOUS_NATIVE_PULL_VERIFIED', 'architecture': arch,
            'all_registry_blobs_verified': True, 'docker_config_and_layers_verified': True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode', choices=['verify-local', 'promote-private', 'verify-public'])
    parser.add_argument('--folder', type=Path)
    parser.add_argument('--architecture', choices=list(MANIFESTS))
    args = parser.parse_args()
    if args.mode == 'verify-local':
        require(args.folder is not None, '--folder required')
        verify_assets(args.folder)
        result = {'status': 'FROZEN_ASSETS_VERIFIED', 'asset_count': 15}
    else:
        # New private directory only; download/cache never touches release source.
        with tempfile.TemporaryDirectory(prefix='ndg-fixed-preview-',
                                         dir=os.environ.get('RUNNER_TEMP')) as temp:
            folder = Path(temp) / 'assets'
            folder.mkdir(mode=0o700)
            if args.mode == 'promote-private':
                result = promote_private(folder)
            else:
                require(args.architecture is not None, '--architecture required')
                result = public_verify(folder, args.architecture)
    result.update({'version': VERSION, 'source_commit': SOURCE, 'tag': TAG,
                   'image': IMAGE + ':' + VERSION, 'index_digest': INDEX,
                   'manifests': MANIFESTS, 'signed': False, 'reproducibility_certified': False})
    output = json.dumps(result, indent=2) + '\n'
    print(output, end='')
    if os.environ.get('GITHUB_ACTIONS') == 'true':
        receipt = Path(os.environ['RUNNER_TEMP']) / ('ndg-publication-' + args.mode
                  + ('-' + args.architecture if args.architecture else '') + '.json')
        receipt.write_text(output)


if __name__ == '__main__':
    main()
