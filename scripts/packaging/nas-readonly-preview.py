#!/usr/bin/env python3
"""Frozen preview host test. Reuses the guarded disposable protocol.

No published port and no extra external network. HTTP is sent by the trusted
host to its nonce-owned internal endpoint with the configured loopback Host.
Historical default smoke behavior and its failure records remain unchanged.
"""
import functools
import importlib
import sys
sys.dont_write_bytecode = True
smoke = importlib.import_module("nas-disposable-smoke")
smoke.FROZEN = {'commit': '1db1e4ab9a1287d3f7f9b801b471b00bfc7c7d1e', 'tree': '23affa62ffa2685e7bced588cf90676e7c6242c2', 'archive_sha256': '3ea1b7a00d4626cce65b09694f7905eb949fb1cc261b7f5c6e47e71e3aa9f432', 'config_digest': 'sha256:2c38ca9493ccebc74cdf0cc84fb23e8a4c59babb1660d4892a2f9a50c12a96d4', 'receipt_sha256': '072d1cec71efad2f9492cd56c1c9fe98432d9a5cd5ee38e0cdfe345acc643fb7'}
smoke.exercise = functools.partial(smoke.exercise, access_mode="internal-direct")
if __name__ == "__main__":
    raise SystemExit(smoke.main())
