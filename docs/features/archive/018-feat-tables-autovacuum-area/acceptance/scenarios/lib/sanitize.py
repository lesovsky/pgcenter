#!/usr/bin/env python3
"""Minimize and fail closed on credential-like content in published evidence."""

import argparse
import re
import sys
from pathlib import Path

SECRET_PATTERNS = (
    re.compile(r"(?i)(?:password|passwd|secret|token|api[_-]?key|authorization)\s*[:=]\s*(?!<REDACTED>)[^\s,;]+"),
    re.compile(r"(?i)\b(?:PGPASSWORD|PGPASSFILE|PGSERVICE|AWS_SECRET_ACCESS_KEY)\s*=\s*(?!<REDACTED>)\S+"),
    re.compile(r"(?i)\b[a-z][a-z0-9+.-]*://[^\s/:@]+:[^\s/@]+@"),
    re.compile(r"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----"),
)
CONTROL = re.compile(r"[\x00-\x08\x0b\x0c\x0e-\x1a\x1c-\x1f\x7f]")


def sanitized(data: str) -> list[str]:
    problems = []
    for number, line in enumerate(data.splitlines(), 1):
        for pattern in SECRET_PATTERNS:
            if pattern.search(line):
                problems.append(f"line {number}: credential-like value")
                break
        if CONTROL.search(line):
            problems.append(f"line {number}: unsafe control character")
    return problems


def sanitize(data: str, replacements: list[str]) -> str:
    for item in replacements:
        old, separator, new = item.partition("=")
        if not separator or not old:
            raise ValueError("replacement must be OLD=NEW")
        data = data.replace(old, new)
    data = re.sub(
        r"(?i)(password|passwd|secret|token|api[_-]?key|authorization)(\s*[:=]\s*)[^\s,;]+",
        r"\1\2<REDACTED>",
        data,
    )
    data = re.sub(
        r"(?i)\b([a-z][a-z0-9+.-]*://)[^\s/:@]+:[^\s/@]+@",
        r"\1<REDACTED>@",
        data,
    )
    return data


def main() -> int:
    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest="command", required=True)
    for name in ("check", "sanitize"):
        command = sub.add_parser(name)
        command.add_argument("--input", required=True)
        if name == "sanitize":
            command.add_argument("--output", required=True)
            command.add_argument("--replace", action="append", default=[])
    args = parser.parse_args()
    data = Path(args.input).read_text(encoding="utf-8", errors="strict")
    if args.command == "check":
        problems = sanitized(data)
        if problems:
            print("unsanitized evidence rejected: " + "; ".join(problems), file=sys.stderr)
            return 1
        return 0
    try:
        result = sanitize(data, args.replace)
    except ValueError as error:
        print(str(error), file=sys.stderr)
        return 2
    problems = sanitized(result)
    if problems:
        print("redaction did not produce sanitized evidence", file=sys.stderr)
        return 1
    Path(args.output).write_text(result, encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
