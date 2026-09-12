#!/usr/bin/env python3
"""
Audit Go slice & map declarations.
Flags 'var s []T' declarations that may serialize to null or cause nil map panics.
"""

import argparse
import json
import os
import re
import sys

def parse_args():
    parser = argparse.ArgumentParser(description="Check Go slice & map initialization")
    parser.add_argument("path", nargs="?", default=".", help="Directory to scan")
    parser.add_argument("--format", choices=["text", "json"], default="text", help="Output format")
    return parser.parse_args()

def scan_file(file_path):
    findings = []
    with open(file_path, "r", encoding="utf-8", errors="ignore") as f:
        lines = f.readlines()

    var_slice_re = re.compile(r"^\s*var\s+([A-Za-z0-9_]+)\s+\[\]([A-Za-z0-9_.*]+)\s*$")
    var_map_re = re.compile(r"^\s*var\s+([A-Za-z0-9_]+)\s+map\[([^\]]+)\]([A-Za-z0-9_.*]+)\s*$")

    for i, line in enumerate(lines):
        ms = var_slice_re.match(line)
        if ms:
            name, elem = ms.group(1), ms.group(2)
            findings.append({
                "file": file_path,
                "line": i + 1,
                "rule": "explicit_slice_init",
                "variable": name,
                "type": f"[]{elem}",
                "message": f"Uninitialized slice 'var {name} []{elem}'. Prefer explicit empty slice '{name} := []{elem}{{}}' or 'make' to avoid 'null' JSON serialization."
            })
        mm = var_map_re.match(line)
        if mm:
            name, k, v = mm.group(1), mm.group(2), mm.group(3)
            findings.append({
                "file": file_path,
                "line": i + 1,
                "rule": "explicit_map_init",
                "variable": name,
                "type": f"map[{k}]{v}",
                "message": f"Uninitialized map 'var {name} map[{k}]{v}'. Writing to a nil map panics; initialize with '{name} := map[{k}]{v}{{}}' or make."
            })

    return findings

def scan_directory(base_dir):
    findings = []
    for root, dirs, files in os.walk(base_dir):
        dirs[:] = [d for d in dirs if not d.startswith(".") and d not in ("vendor", "data", "test")]
        for f in files:
            if f.endswith(".go") and not f.endswith("_test.go"):
                p = os.path.normpath(os.path.join(root, f))
                findings.extend(scan_file(p))
    return findings

def main():
    args = parse_args()
    findings = scan_directory(args.path)

    if args.format == "json":
        print(json.dumps(findings, indent=2))
    else:
        if not findings:
            print(f"No uninitialized slice/map declarations found in {args.path}.")
            return 0
        print(f"Found {len(findings)} uninitialized slice/map declarations:")
        for f in findings:
            print(f"  {f['file']}:{f['line']} [{f['rule']}] - {f['message']}")

    return 1 if findings else 0

if __name__ == "__main__":
    sys.exit(main())
