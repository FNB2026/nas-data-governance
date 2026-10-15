#!/usr/bin/env python3
"""Exact frozen fnOS preview, Compose installation in the guarded private network.

No daemon configuration or existing project is modified. A nonce project uses
an already verified internal network by ID; it never creates a default network.
"""
import importlib
import json
import time
import urllib.request

preview = importlib.import_module('nas-readonly-preview')
smoke = preview.smoke
BaseResources = smoke.Resources
base_exercise = smoke.exercise


class ComposeResources(BaseResources):
    compose_id = None
    compose_started = False

    def compose_call(self, action):
        smoke.require(action in {'up', 'down'}, 'invalid_compose_action')
        smoke.daemon_capacity(self.docker, self.docker.capacity)
        smoke.require((self.root / 'private/compose.json').read_bytes() == self.compose_bytes, 'compose_config_changed')
        self.own('network', self.network)
        suffix = ['up', '-d', '--no-build', '--pull', 'never'] if action == 'up' else ['down', '--timeout', '60']
        return self.docker.call(['compose', '--project-name', 'ndg-preview-' + self.nonce,
                                 '--file', str(self.root / 'private/compose.json'), *suffix], check=False)

    def compose_install(self, root):
        smoke.require(root == self.root and self.compose_id is None, 'foreign_compose_root')
        name = 'ndg-compose-' + self.nonce
        service = {'image': self.image, 'pull_policy': 'never', 'container_name': name,
                   'labels': {smoke.LABEL: self.nonce}, 'network_mode': self.network,
                   'user': '10001:10001', 'read_only': True, 'cap_drop': ['ALL'],
                   'security_opt': ['no-new-privileges:true'], 'pids_limit': 128,
                   'mem_limit': '384m', 'cpus': 1, 'restart': 'no',
                   'tmpfs': ['/tmp:rw,nosuid,nodev,noexec,size=64m'], 'stop_grace_period': '60s',
                   'volumes': [{'type': 'bind', 'source': str(root / area), 'target': target, 'read_only': readonly}
                               for area, target, readonly in [('source', '/sources/source_a', True), ('config', '/config', True), ('state', '/data', False)]]}
        self.compose_bytes = json.dumps({'services': {'ndg': service}}, sort_keys=True).encode()
        if self.compose_started:
            smoke.require((root / 'private/compose.json').read_bytes() == self.compose_bytes, 'compose_config_changed')
        else:
            smoke.private_write(root / 'private/compose.json', self.compose_bytes)
            smoke.private_write(root / 'private/compose-planned-name.json', json.dumps({'name': name, 'nonce': self.nonce}).encode())
        smoke.require(self.docker.call(['container', 'inspect', name], check=False).returncode != 0, 'compose_name_exists')
        result = self.compose_call('up')
        # Capture only this exact pre-recorded name, never discover by prefix.
        found = self.docker.call(['container', 'inspect', name], check=False)
        if found.returncode == 0:
            value = smoke.evidence.json_value(found.stdout)[0]
            smoke.require(value.get('Config', {}).get('Labels', {}).get(smoke.LABEL) == self.nonce and value.get('Image') == self.image, 'compose_ownership_unknown')
            identifier = value['Id']; smoke.require(smoke.ID.fullmatch(identifier), 'compose_id_unknown')
            self.containers.append(identifier); self.compose_id = identifier; self.record()
            self.own('container', identifier)
        smoke.require(result.returncode == 0 and self.compose_id is not None, 'compose_install_failed')
        self.compose_started = True
        return self.compose_id

    def create_container(self, root, port=None):
        smoke.require(port is None, 'compose_requires_internal_access')
        if not self.compose_started:
            return self.compose_install(root)
        return super().create_container(root)

    def compose_remove(self):
        if self.compose_id is None:
            return
        identifier = self.compose_id
        self.own('container', identifier)
        result = self.compose_call('down')
        smoke.require(result.returncode == 0 and self.docker.call(['container', 'inspect', identifier], check=False).returncode != 0, 'compose_uninstall_failed')
        self.containers.remove(identifier); self.compose_id = None; self.record()
        smoke.require((self.root / 'state').is_dir() and any((self.root / 'state').iterdir()), 'compose_state_lost')

    def cleanup(self):
        self.compose_remove()
        super().cleanup()


def exercise(resources, root, port, settings, password, marker, source):
    result = base_exercise(resources, root, port, settings, password, marker, source)
    resources.compose_remove()
    identifier = resources.compose_install(root)
    own = resources.own('container', identifier)
    address = next(iter(own['NetworkSettings']['Networks'].values()))['IPAddress']
    jar = smoke.http.cookiejar.CookieJar()
    client = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(jar))
    csrf = ''
    def request(path, body):
        req = urllib.request.Request('http://' + address + ':8080' + path, data=json.dumps(body).encode(),
            headers={'Host': '127.0.0.1:' + str(port), 'Origin': settings['origin'], 'Content-Type': 'application/json', 'X-CSRF-Token': csrf})
        with client.open(req, timeout=5) as response:
            raw = response.read(1024 * 1024)
            smoke.require(all(value.encode() not in raw for value in [marker, password, str(root), '/sources/source_a']), 'http_privacy_window_failed')
            return json.loads(raw)
    time.sleep(1)
    csrf = request('/api/login', {'password': password})['csrf']
    snapshot = request('/api/status', {'alias': 'source_a'})
    smoke.require(snapshot['active'] == 7 and snapshot['full'] == 6 and snapshot['checkpoint'] == 'completed', 'restart_state_lost')
    resources.compose_remove()
    result['compose_install_recreate_persistence_uninstall'] = True
    return result


smoke.Resources = ComposeResources
smoke.exercise = exercise
if __name__ == '__main__':
    raise SystemExit(smoke.main())
