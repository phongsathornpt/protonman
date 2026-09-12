#!/usr/bin/env python3
"""
Audit Go control flow conventions:
1. Unnecessary 'else' blocks after return, break, or continue.
2. Complex 'if' conditions with >= 3 boolean operators (&&, ||) needing variable extraction.
"""

import argparse
import json
import os
import re
import sys

def parse_args():
    parser = argparse.ArgumentParser(description="Check Go control flow style")
    parser.add_argument("path", nargs="?", default=".", help="Directory to scan")
    parser.add_argument("--format", choices=["text", "json"], default="text", help="Output format")
    return parser.parse_args()

def scan_file(file_path):
    findings = []
    with open(file_path, "r", encoding="utf-8", errors="ignore") as f:
        lines = f.readlines()

    # Rule 1: unnecessary else after return / break / continue
    for i, line in enumerate(lines):
        if "} else {" in line or "} else" in line:
            # check previous non-empty line
            prev_idx = i - 1
            while prev_idx >= 0 and not lines[prev_idx].strip():
                prev_idx -= 1
            if prev_idx >= 0:
                prev_text = lines[prev_idx].strip()
                match = re.search(r"\b(return|break|continue)\b", prev_text)
                if match:
                    findings.append({
                        "file": file_path,
                        "line": i + 1,
                        "rule": "no_else_after_terminal",
                        "keyword": match.group(1),
                        "message": f"Unnecessary 'else' block after '{match.group(1)}'. Drop 'else' and un-indent the block."
                    })

    # Rule 2: complex if condition (>= 3 boolean operators)
    if_pattern = re.compile(r"^\s*if\s+(.*)\s*\{")
    for i, line in enumerate(lines):
        m = if_pattern.match(line)
        if m:
            cond = m.group(1)
            # Exclude simple initialization statements like: if err := foo(); err != nil
            if ";" in cond:
                cond = cond.split(";", 1)[1]
            op_count = cond.count("&&") + cond.count("||")
            if op_count >= 3:
                findings.append({
                    "file": file_path,
                    "line": i + 1,
                    "rule": "extract_complex_condition",
                    "operator_count": op_count,
                    "condition": cond.strip(),
                    "message": f"Complex 'if' condition with {op_count} boolean operators. Extract into named booleans for clarity."
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
            print(f"No control flow violations found in {args.path}.")
            return 0
        print(f"Found {len(findings)} control flow issues:")
        for f in findings:
            print(f"  {f['file']}:{f['line']} [{f['rule']}] - {f['message']}")

    return 1 if findings else 0

if __name__ == "__main__":
    sys.exit(main())
