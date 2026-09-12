#!/usr/bin/env python3
"""
Audit Go function signatures for parameter arity.
Flags functions with > 4 parameters.
"""

import argparse
import json
import os
import re
import sys

def parse_args():
    parser = argparse.ArgumentParser(description="Check Go function parameter arity")
    parser.add_argument("path", nargs="?", default=".", help="Directory to scan")
    parser.add_argument("--max-params", type=int, default=4, help="Maximum allowed parameters (default: 4)")
    parser.add_argument("--format", choices=["text", "json"], default="text", help="Output format")
    return parser.parse_args()

def extract_params(param_str):
    if not param_str or param_str.isspace():
        return []
    depth = 0
    current = []
    params = []
    for ch in param_str:
        if ch in "([{<":
            depth += 1
            current.append(ch)
        elif ch in ")]}>":
            depth -= 1
            current.append(ch)
        elif ch == "," and depth == 0:
            p = "".join(current).strip()
            if p:
                params.append(p)
            current = []
        else:
            current.append(ch)
    if current:
        p = "".join(current).strip()
        if p:
            params.append(p)
    return params

def scan_file(file_path, max_params):
    findings = []
    with open(file_path, "r", encoding="utf-8", errors="ignore") as f:
        content = f.read()

    # Matches func (recv) Name(params) (returns) { or func Name(params)
    pattern = re.compile(
        r"func\s+(?:\((?P<recv>[^)]+)\)\s+)?(?P<name>[A-Za-z0-9_]+)\s*\((?P<params>[^)]*)\)",
        re.MULTILINE
    )

    for m in pattern.finditer(content):
        param_list = extract_params(m.group("params"))
        if len(param_list) > max_params:
            # find line number
            line_no = content[:m.start()].count("\n") + 1
            name = m.group("name")
            recv = m.group("recv")
            full_name = f"({recv.strip()}).{name}" if recv else name
            findings.append({
                "file": file_path,
                "line": line_no,
                "function": full_name,
                "param_count": len(param_list),
                "params": param_list,
                "rule": "func_max_params",
                "message": f"Function '{full_name}' has {len(param_list)} parameters (max allowed: {max_params}). Consider refactoring to an options struct."
            })
    return findings

def scan_directory(base_dir, max_params):
    findings = []
    for root, dirs, files in os.walk(base_dir):
        # Skip hidden directories, git, vendor, data
        dirs[:] = [d for d in dirs if not d.startswith(".") and d not in ("vendor", "data", "test")]
        for f in files:
            if f.endswith(".go") and not f.endswith("_test.go"):
                p = os.path.normpath(os.path.join(root, f))
                findings.extend(scan_file(p, max_params))
    return findings

def main():
    args = parse_args()
    findings = scan_directory(args.path, args.max_params)

    if args.format == "json":
        print(json.dumps(findings, indent=2))
    else:
        if not findings:
            print(f"All functions in {args.path} satisfy max parameter threshold ({args.max_params}).")
            return 0
        print(f"Found {len(findings)} functions exceeding {args.max_params} parameters:")
        for f in findings:
            print(f"  {f['file']}:{f['line']} - {f['function']} ({f['param_count']} params)")

    return 1 if findings else 0

if __name__ == "__main__":
    sys.exit(main())
