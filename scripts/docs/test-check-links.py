#!/usr/bin/env python3
import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('links', Path(__file__).with_name('check-links.py'))
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)


class LinkAuditTests(unittest.TestCase):
    def test_balanced_destinations_and_fenced_examples_keep_line_numbers(self):
        text = '```md\n[ignored](missing)\n```\n[x](docs/a(b).md#id)\n'
        self.assertEqual(m.links(text), [(4, 'docs/a(b).md#id')])

    def test_reference_html_and_autolinks(self):
        self.assertEqual([v for _, v in m.links('[id]: x.md\n<img src="a.png">\n<https://example.com>')],
                         ['x.md', 'a.png', 'https://example.com'])

    def test_heading_duplicates_and_explicit_id(self):
        self.assertEqual(m.anchors('# Same!\n## Same!\n<a id="other">'), {'same', 'same-1', 'other'})

    def test_local_anchor_and_untracked_asset(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d).resolve()
            source = root / 'README.md'
            source.write_text('# Heading\n')
            (root / 'untracked.png').touch()
            self.assertEqual(m.local_check(root, source, '#heading'), 'LOCAL_PASS')
            self.assertEqual(m.local_check(root, source, '#missing'), 'ANCHOR_MISSING')
            self.assertEqual(m.local_check(root, source, 'untracked.png', {'README.md'}), 'LOCAL_UNTRACKED')
            self.assertEqual(m.local_check(root, source, '../escape'), 'OUTSIDE_REPOSITORY')

    def test_private_dns_and_examples_never_connect(self):
        records = [(2, 1, 6, '', ('127.0.0.1', 443))]
        with patch.object(m.socket, 'getaddrinfo', return_value=records), \
                patch.object(m.socket, 'create_connection') as connect:
            self.assertEqual(m.http_check('https://public-looking.invalid/')['status'], 'EXAMPLE_OR_TEMPLATE')
            self.assertEqual(m.http_check('https://public-looking.example.org/')['status'], 'DNS_NOT_PUBLIC_NOT_FETCHED')
            self.assertEqual(m.http_check('https://user:secret@example.org/')['status'], 'NOT_PUBLIC_HTTP')
            connect.assert_not_called()


if __name__ == '__main__':
    unittest.main()
