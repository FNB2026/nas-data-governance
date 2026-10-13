#!/usr/bin/env python3
"""Normalize whitespace-only lines in generated SDK output deterministically."""
from pathlib import Path
import sys

target = Path(__file__).resolve().parents[1] / "cmd/ndg-desktop/frontend/src/wailsjs/go/models.ts"
if target.is_symlink() or not target.is_file():
    sys.exit("wails-model-normalization: generated file unavailable")
original = target.read_text()
normalized = "\n".join(line if line.strip() else "" for line in original.splitlines()) + "\n"
if normalized != original:
    target.write_text(normalized)
