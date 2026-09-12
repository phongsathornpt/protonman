#!/usr/bin/env python3
"""
Master runner for Go code quality audit.
Aggregates checks for parameter arity, control flow, slice nilness, and line length.
"""

import argparse
import json
import os
import sys

# Import local checks
script_dir = os.path.dirname(os.path.abspath(__file__))
if script_dir not in sys.path:
    sys.path.insert(0, script_dir)

import check_func_arity
import check_control_flow
import check_slice_nilness

def check_line_length(base_dir, max_len=120):
    findings = []
    for root, dirs, files in os.walk(base_dir):
        dirs[:] = [d for d in dirs if not d.startswith(".") and d not in ("vendor", "data", "test")]
        for f in files:
            if f.endswith(".go") and not f.endswith("_test.go"):
                p = os.path.normpath(os.path.join(root, f))
                with open(p, "r", encoding="utf-8", errors="ignore") as fh:
                    for line_no, line in enumerate(fh, 1):
                        rline = line.rstrip()
                        if len(rline) > max_len:
                            findings.append({
                                "file": p,
                                "line": line_no,
                                "rule": "line_length",
                                "length": len(rline),
                                "message": f"Line exceeds {max_len} characters ({len(rline)} chars). Break at semantic boundary."
                            })
    return findings

def parse_args():
    parser = argparse.ArgumentParser(description="Run complete Go code quality audit")
    parser.add_argument("path", nargs="?", default=".", help="Directory to scan")
    parser.add_argument("--max-params", type=int, default=4, help="Max allowed parameters")
    parser.add_argument("--max-line-len", type=int, default=120, help="Max line length")
    parser.add_argument("--format", choices=["text", "json"], default="text", help="Output format")
    return parser.parse_args()

def main():
    args = parse_args()
    target_path = args.path

    arity_findings = check_func_arity.scan_directory(target_path, args.max_params)
    control_findings = check_control_flow.scan_directory(target_path)
    slice_findings = check_slice_nilness.scan_directory(target_path)
    line_findings = check_line_length(target_path, args.max_line_len)

    total_findings = len(arity_findings) + len(control_findings) + len(slice_findings) + len(line_findings)

    report = {
        "target": target_path,
        "summary": {
            "total_issues": total_findings,
            "func_arity_violations": len(arity_findings),
            "control_flow_violations": len(control_findings),
            "slice_nilness_violations": len(slice_findings),
            "line_length_violations": len(line_findings),
        },
        "findings": {
            "function_arity": arity_findings,
            "control_flow": control_findings,
            "slice_nilness": slice_findings,
            "line_length": line_findings,
        }
    }

    if args.format == "json":
        print(json.dumps(report, indent=2))
    else:
        print("========================================")
        print(" Go Code Quality Audit Report")
        print("========================================")
        print(f"Target directory: {target_path}")
        print(f"Total issues:     {total_findings}")
        print(f" - Function arity (> {args.max_params} params):   {len(arity_findings)}")
        print(f" - Control flow (else/complex):   {len(control_findings)}")
        print(f" - Slice/map nilness:             {len(slice_findings)}")
        print(f" - Line length (> {args.max_line_len} cols):        {len(line_findings)}")
        print("----------------------------------------")

        if arity_findings:
            print("\n[Function Arity Violations (Sample)]")
            for f in arity_findings[:5]:
                print(f"  {f['file']}:{f['line']} - {f['function']} ({f['param_count']} params)")
            if len(arity_findings) > 5:
                print(f"  ... and {len(arity_findings) - 5} more.")

        if control_findings:
            print("\n[Control Flow Violations (Sample)]")
            for f in control_findings[:5]:
                print(f"  {f['file']}:{f['line']} - {f['message']}")
            if len(control_findings) > 5:
                print(f"  ... and {len(control_findings) - 5} more.")

        if slice_findings:
            print("\n[Slice/Map Nilness Violations (Sample)]")
            for f in slice_findings[:5]:
                print(f"  {f['file']}:{f['line']} - {f['message']}")
            if len(slice_findings) > 5:
                print(f"  ... and {len(slice_findings) - 5} more.")

        if line_findings:
            print("\n[Line Length Violations (Sample)]")
            for f in line_findings[:5]:
                print(f"  {f['file']}:{f['line']} - length {f['length']}")
            if len(line_findings) > 5:
                print(f"  ... and {len(line_findings) - 5} more.")

    return 1 if total_findings > 0 else 0

if __name__ == "__main__":
    sys.exit(main())
