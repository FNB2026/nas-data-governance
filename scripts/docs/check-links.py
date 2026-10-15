#!/usr/bin/env python3
"""Audit tracked Markdown/HTML links without fetching private/example endpoints.

Offline mode checks paths and GitHub-style heading anchors. --external performs
credential-free, DNS-pinned public HTTP GETs, validates redirects separately and
classifies auth/rate-limit/network failures without declaring them dead.
"""
import argparse
from collections import Counter
import concurrent.futures
import hashlib
import http.client
import ipaddress
import json
from pathlib import Path
import re
import socket
import ssl
import subprocess
import unicodedata
import urllib.parse


def prose(text):
    # Keep line numbers stable when excluding fenced code examples.
    result, fence = [], None
    for line in text.splitlines(keepends=True):
        marker = re.match(r'^ {0,3}(`{3,}|~{3,})', line)
        if fence:
            if marker and marker[1][0] == fence[0] and len(marker[1]) >= len(fence):
                fence = None
            result.append('\n' if line.endswith('\n') else '')
        elif marker:
            fence = marker[1]
            result.append('\n' if line.endswith('\n') else '')
        else:
            result.append(line)
    return ''.join(result)


def links(text):
    text = prose(text)
    found = []
    # Balanced parenthesis destinations, including GitHub filenames with spaces.
    for match in re.finditer(r'!?\[[^\]\n]*\]\(', text):
        start, depth, pos = match.end(), 1, match.end()
        while pos < len(text) and depth:
            if text[pos] == '\\':
                pos += 2
                continue
            if text[pos] == '(':
                depth += 1
            elif text[pos] == ')':
                depth -= 1
            pos += 1
        if depth == 0:
            value = text[start:pos - 1].strip()
            if value.startswith('<') and '>' in value:
                value = value[1:value.index('>')]
            else:
                value = re.split(r'\s+[\"\']', value, maxsplit=1)[0]
            found.append((text.count('\n', 0, match.start()) + 1, value))
    for match in re.finditer(r'^\s*\[[^\]]+\]:\s*<?([^\s>]+)', text, re.MULTILINE):
        found.append((text.count('\n', 0, match.start()) + 1, match.group(1)))
    for match in re.finditer(r'(?:src|href)=[\"\']([^\"\']+)[\"\']|<(https?://[^>]+)>', text):
        found.append((text.count('\n', 0, match.start()) + 1, match.group(1) or match.group(2)))
    return found


def anchors(text):
    result, seen = set(), Counter()
    for line in prose(text).splitlines():
        m = re.match(r'^ {0,3}#{1,6}\s+(.+?)\s*#*$', line)
        if not m:
            continue
        title = re.sub(r'<[^>]*>', '', m.group(1)).lower()
        title = re.sub(r'!?\[([^\]]+)\]\([^)]*\)', r'\1', title)
        slug = ''.join(c for c in title if c in '-_ ' or
                       unicodedata.category(c)[0] not in 'PS')
        slug = slug.replace(' ', '-')
        count = seen[slug]
        seen[slug] += 1
        result.add(slug + ('-' + str(count) if count else ''))
    result.update(re.findall(r'(?:id|name)=[\"\']([^\"\']+)', text))
    return result


def local_check(root, source, value, tracked=None):
    parsed = urllib.parse.urlsplit(value)
    if parsed.scheme or value.startswith('//'):
        return None
    raw = urllib.parse.unquote(parsed.path)
    target = root / raw.lstrip('/') if raw.startswith('/') else source.parent / raw
    target = target.resolve()
    if not target.is_relative_to(root):
        return 'OUTSIDE_REPOSITORY'
    if not target.exists():
        return 'LOCAL_MISSING'
    if tracked is not None:
        relative = target.relative_to(root).as_posix()
        if relative not in tracked and not (target.is_dir() and any(n.startswith(relative.rstrip('/') + '/') for n in tracked)):
            return 'LOCAL_UNTRACKED'
    if parsed.fragment:
        if target.is_dir():
            target = target / 'README.md'
        if target.suffix.lower() in ('.md', '.markdown', '.html') and target.is_file():
            if urllib.parse.unquote(parsed.fragment) not in anchors(target.read_text()):
                return 'ANCHOR_MISSING'
    return 'LOCAL_PASS'


def public_target(url):
    parsed = urllib.parse.urlsplit(url)
    host = parsed.hostname or ''
    if (parsed.scheme not in ('http', 'https') or parsed.username or parsed.password
            or parsed.port not in (None, 80, 443) or not host):
        return None, 'NOT_PUBLIC_HTTP'
    if (any(c in url for c in '{}<>') or host in ('example.com', 'example.org', 'example.net')
            or host.endswith(('.example', '.invalid', '.test', '.local', '.localhost'))
            or host in ('localhost', 'host', 'your-domain.com')):
        return None, 'EXAMPLE_OR_TEMPLATE'
    try:
        records = socket.getaddrinfo(host, parsed.port or (443 if parsed.scheme == 'https' else 80),
                                     type=socket.SOCK_STREAM)
    except OSError:
        return None, 'UNKNOWN_DNS'
    addresses = sorted({record[4][0] for record in records}, key=lambda ip: ':' in ip)
    if not addresses or any(not ipaddress.ip_address(ip).is_global for ip in addresses):
        return None, 'DNS_NOT_PUBLIC_NOT_FETCHED'
    return (parsed, addresses[0]), None


def http_check(url):
    current, redirects = url, []
    for _ in range(6):
        try:
            target, status = public_target(current)
            if status:
                return {'status': status, 'redirects': redirects}
            parsed, address = target
            host = parsed.hostname.encode('idna').decode()
            conn = http.client.HTTPConnection(host, parsed.port or (443 if parsed.scheme == 'https' else 80), timeout=12)
            sock = socket.create_connection((address, conn.port), timeout=12)
            conn.sock = ssl.create_default_context().wrap_socket(sock, server_hostname=host) if parsed.scheme == 'https' else sock
            path = urllib.parse.quote(parsed.path or '/', safe='/%:@!$&\'()*+,;=-._~')
            if parsed.query:
                path += '?' + parsed.query
            try:
                conn.request('GET', path, headers={'User-Agent': 'NDG-public-link-audit/1', 'Range': 'bytes=0-4095'})
                response = conn.getresponse()
                code, location = response.status, response.getheader('Location')
                response.read(4096)
            finally:
                conn.close()
            if code in (301, 302, 303, 307, 308) and location:
                current = urllib.parse.urljoin(current, location)
                redirects.append(current)
                continue
            if code in (401, 403):
                status = 'AUTH_OR_POLICY_REQUIRED'
            elif code == 429:
                status = 'RATE_LIMITED'
            elif code in (404, 410):
                status = 'DEAD_CANDIDATE'
            elif 200 <= code < 300:
                status = 'REDIRECT_PASS' if redirects else 'HTTP_PASS'
            else:
                status = 'UNKNOWN_HTTP'
            return {'status': status, 'http_status': code, 'redirects': redirects}
        except (OSError, ValueError, http.client.HTTPException):
            return {'status': 'UNKNOWN_NETWORK', 'redirects': redirects}
    return {'status': 'REDIRECT_LIMIT', 'redirects': redirects}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--external', action='store_true')
    parser.add_argument('--report', type=Path)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[2]
    files = subprocess.check_output(['git', 'ls-files', '-z', '*.md', '*.markdown'], cwd=root).decode().split('\0')
    tracked = set(subprocess.check_output(['git', 'ls-files', '-z'], cwd=root).decode().split('\0'))
    rows, external = [], {}
    for name in filter(None, files):
        source = root / name
        if not source.is_file():
            continue
        for line, value in links(source.read_text()):
            status = local_check(root, source, value, tracked)
            row = {'file': name, 'line': line, 'target': value, 'status': status or 'EXTERNAL_NOT_RUN'}
            rows.append(row)
            if status is None and urllib.parse.urlsplit(value).scheme in ('http', 'https'):
                key = urllib.parse.urldefrag(value)[0]
                external.setdefault(key, []).append(row)
    if args.external:
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            for url, result in zip(external, pool.map(http_check, external)):
                for row in external[url]:
                    row.update(result)
    result = {'schema': 1, 'source_sha': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root).decode().strip(),
              'worktree_dirty': bool(subprocess.check_output(['git', 'status', '--porcelain'], cwd=root).strip()),
              'checker_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'scope': 'tracked Markdown inline/reference/HTML links; fenced examples excluded; external anchors not certified',
              'external_run': args.external, 'counts': dict(Counter(r['status'] for r in rows)), 'links': rows}
    if args.report:
        args.report.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'source_sha': result['source_sha'], 'counts': result['counts']}, ensure_ascii=False))
    failures = [r for r in rows if r['status'] in ('LOCAL_MISSING', 'LOCAL_UNTRACKED', 'ANCHOR_MISSING', 'OUTSIDE_REPOSITORY')]
    for row in failures:
        print(json.dumps(row, ensure_ascii=False))
    return 1 if failures or any(r['status'] == 'DEAD_CANDIDATE' for r in rows) else 0


if __name__ == '__main__':
    raise SystemExit(main())
