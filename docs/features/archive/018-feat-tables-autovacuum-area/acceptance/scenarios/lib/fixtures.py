#!/usr/bin/env python3
"""Generate and independently validate deterministic pgcenter recording archives."""

import argparse
import datetime as dt
import io
import json
import re
import sys
import tarfile
from pathlib import Path

TABLES = [
    "relation", "seq_scan", "seq_read", "idx_scan", "idx_fetch", "inserts",
    "updates", "deletes", "hot_updates", "live", "dead", "heap_read,KiB",
    "heap_hit", "idx_read,KiB", "idx_hit", "toast_read,KiB", "toast_hit",
    "tidx_read,KiB", "tidx_hit",
]
INDEXES = ["index", "scan", "tuples_read", "tuples_fetch", "hit", "read,KiB"]
FUNCTIONS = ["funcid", "function", "calls_total", "calls", "total", "self", "total_avg,ms", "self_avg,ms"]
SIZES = [
    "relation", "indexes_total", "size_total,KiB", "table_total,KiB", "meta_total,KiB",
    "toast_total,KiB", "indexes_total,KiB", "size,KiB", "table,KiB", "meta,KiB",
    "toast,KiB", "indexes,KiB",
]
OLD = {"tables": TABLES, "indexes": INDEXES, "functions": FUNCTIONS, "sizes": SIZES}
NEW = {"tables": TABLES + ["stats_age"], "indexes": INDEXES + ["stats_age"]}
META_COLS = [
    "version", "version_num", "track_commit_timestamp", "max_connections",
    "autovacuum_max_workers", "recovery", "start_time_unix",
]
BASE = dt.datetime(2026, 5, 19, 10, 0, 0)
NAME_RE = re.compile(r"^(meta|sysinfo|tables|indexes|functions|sizes)\.(\d{8}T\d{6}\.\d{3})\.json$")


def cell(value):
    if value is None:
        return {"String": "", "Valid": False}
    return {"String": str(value), "Valid": True}


def result(cols, rows):
    return {
        "Values": [[cell(value) for value in row] for row in rows],
        "Cols": cols,
        "Ncols": len(cols),
        "Nrows": len(rows),
        "Valid": True,
    }


def metadata(version):
    rows = [[f"{version // 10000}.0", str(version), "off", "100", "3", "false", "1779184800000000e-6"]]
    return result(META_COLS, rows)


def identities(screen):
    if screen == "tables" or screen == "sizes":
        return ["public.acc018_alpha", "public.acc018_zulu"]
    if screen == "indexes":
        return ["public.alpha.alpha_idx", "public.zulu.zulu_idx"]
    raise ValueError(screen)


def ages(age, tick):
    if age == "blank":
        return [None, None]
    if age == "wide":
        return [f"1 day 00:00:{10 + tick:02d}", f"00:00:{10 + tick:02d}"]
    if age == "populated":
        return [f"1 day 00:00:{tick:02d}", f"00:00:{tick:02d}"]
    raise ValueError(age)


def relation_rows(screen, layout, tick, role, age):
    old = OLD[screen]
    cols = NEW[screen] if layout == "new" else old
    rows = []
    for row_index, identity in enumerate(identities(screen)):
        row = [identity]
        for col_index in range(1, len(old)):
            value = (row_index + 1) * 100000 + col_index * 1000 + tick * ((row_index + 1) * 10 + col_index)
            row.append(value)
        rows.append(row)

    first_metric = 1
    if role == "mixed":
        cumulative = [
            [100, 100],
            [110, 130],
            [1000, 1000],
            [123457789, 1030],
        ]
        for row_index in range(2):
            rows[row_index][first_metric] = cumulative[tick][row_index]
    else:
        cumulative = [
            [1000, 2000],
            [1010, 2020],
            [5000, 6000],
            [5030, 6040],
        ]
        for row_index in range(2):
            rows[row_index][first_metric] = cumulative[tick][row_index]

    if layout == "new":
        for row_index, value in enumerate(ages(age, tick)):
            rows[row_index].append(value)
    return result(cols, rows)


def function_result(tick):
    rows = []
    for row_index, identity in enumerate(identities("tables")):
        calls_total = 1000 * (row_index + 1) + tick * (10 + row_index)
        rows.append([
            str(18001 + row_index), identity.replace("acc018_", "acc018_fn_"), calls_total,
            calls_total, "00:00:01", "00:00:01", "1.0000", "1.0000",
        ])
    return result(FUNCTIONS, rows)


def size_result(tick):
    rows = []
    for row_index, identity in enumerate(identities("sizes")):
        row = [identity]
        for col_index in range(1, len(SIZES)):
            row.append(1000 * (row_index + 1) + col_index * 100 + tick * (col_index + row_index + 1))
        rows.append(row)
    return result(SIZES, rows)


def screen_result(screen, version, layout, tick, role, age):
    if screen in ("tables", "indexes"):
        return relation_rows(screen, layout, tick, role, age)
    if screen == "functions":
        return function_result(tick)
    if screen == "sizes":
        return size_result(tick)
    raise ValueError(screen)


def tick_spec(version, screens):
    return {"version": version, "screens": screens}


def expected_ticks(args):
    kind = args.kind
    if kind == "mixed":
        if args.screen not in NEW or args.pg19_layout not in ("old", "new"):
            raise ValueError("mixed requires tables/indexes and old/new PG19 layout")
        ticks = []
        for tick in range(4):
            version = 180000 if tick < 2 else 190000
            layout = "old" if tick < 2 else args.pg19_layout
            ticks.append(tick_spec(version, {
                args.screen: screen_result(args.screen, version, layout, tick, "mixed", "wide")
            }))
        return ticks
    if kind == "legacy":
        if args.version not in (170000, 180000):
            raise ValueError("legacy version must be 170000 or 180000")
        ticks = []
        for tick in range(2):
            screens = {
                screen: screen_result(screen, args.version, "old", tick, "control", "blank")
                for screen in ("tables", "indexes", "functions", "sizes")
            }
            ticks.append(tick_spec(args.version, screens))
        return ticks
    if kind == "layout":
        if args.screen not in NEW or args.direction not in ("old-to-new", "new-to-old"):
            raise ValueError("layout requires tables/indexes and a direction")
        if args.age not in ("blank", "populated"):
            raise ValueError("layout age must be blank or populated")
        layouts = ["old", "old", "new", "new"] if args.direction == "old-to-new" else ["new", "new", "old", "old"]
        return [tick_spec(190000, {
            args.screen: screen_result(args.screen, 190000, layouts[tick], tick, "boundary", args.age)
        }) for tick in range(4)]
    if kind == "control":
        if args.screen not in NEW or args.control_layout not in ("old", "new"):
            raise ValueError("control requires tables/indexes and old/new layout")
        if args.age not in ("blank", "populated"):
            raise ValueError("control age must be blank or populated")
        return [tick_spec(190000, {
            args.screen: screen_result(args.screen, 190000, args.control_layout, tick, "control", args.age)
        }) for tick in range(2)]
    raise ValueError(kind)


def timestamp(index):
    return (BASE + dt.timedelta(seconds=index)).strftime("%Y%m%dT%H%M%S.000")


def expected_entries(ticks):
    entries = {}
    for index, tick in enumerate(ticks):
        ts = timestamp(index)
        entries[f"meta.{ts}.json"] = metadata(tick["version"])
        for screen, value in sorted(tick["screens"].items()):
            entries[f"{screen}.{ts}.json"] = value
        entries[f"sysinfo.{ts}.json"] = {"ticks": 100, "cpu_count": 4}
    return entries


def encode(value):
    return json.dumps(value, separators=(",", ":"), ensure_ascii=True).encode("utf-8")


def generate(path, ticks):
    entries = expected_entries(ticks)
    with tarfile.open(path, "w", format=tarfile.PAX_FORMAT) as archive:
        for name, value in entries.items():
            payload = encode(value)
            info = tarfile.TarInfo(name)
            info.size = len(payload)
            info.mode = 0o600
            info.mtime = int(BASE.timestamp())
            archive.addfile(info, io.BytesIO(payload))
    validate(path, ticks)


def validate(path, ticks):
    expected = expected_entries(ticks)
    actual = {}
    with tarfile.open(path, "r:*") as archive:
        members = archive.getmembers()
        if len(members) != len(expected):
            raise ValueError("archive member count does not match fixture model")
        for member in members:
            if not member.isfile() or not NAME_RE.fullmatch(member.name):
                raise ValueError("unsafe or malformed archive member")
            if member.name in actual:
                raise ValueError("duplicate archive member")
            stream = archive.extractfile(member)
            if stream is None:
                raise ValueError("unreadable archive member")
            actual[member.name] = json.loads(stream.read())
    if set(actual) != set(expected):
        raise ValueError("archive member names or time bounds do not match fixture model")
    if actual != expected:
        raise ValueError("archive payload does not match fixture model")
    for index, tick in enumerate(ticks):
        ts = timestamp(index)
        for screen, value in tick["screens"].items():
            recorded = actual[f"{screen}.{ts}.json"]
            if not recorded["Valid"] or recorded["Ncols"] != len(recorded["Cols"]):
                raise ValueError("invalid PGresult shape")
            if recorded["Nrows"] != len(recorded["Values"]) or recorded["Nrows"] < 1:
                raise ValueError("fixture must contain readable rows")
            if any(len(row) != recorded["Ncols"] for row in recorded["Values"]):
                raise ValueError("ragged PGresult values")
    return len(actual)


def parse_args():
    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest="command", required=True)
    for name in ("generate", "validate"):
        command = sub.add_parser(name)
        command.add_argument("--archive", required=True)
        command.add_argument("--kind", choices=("mixed", "legacy", "layout", "control"), required=True)
        command.add_argument("--screen", choices=("tables", "indexes"))
        command.add_argument("--version", type=int)
        command.add_argument("--pg19-layout", dest="pg19_layout")
        command.add_argument("--direction")
        command.add_argument("--age", default="blank")
        command.add_argument("--control-layout", dest="control_layout")
    bounds = sub.add_parser("bounds")
    bounds.add_argument("--archive", required=True)
    return parser.parse_args()


def main():
    args = parse_args()
    if args.command == "bounds":
        with tarfile.open(args.archive, "r:*") as archive:
            stamps = []
            for member in archive.getmembers():
                match = NAME_RE.fullmatch(member.name)
                if match:
                    stamps.append(dt.datetime.strptime(match.group(2), "%Y%m%dT%H%M%S.%f"))
        if not stamps:
            raise ValueError("archive has no bounded samples")
        print((min(stamps) - dt.timedelta(seconds=1)).strftime("%Y-%m-%d %H:%M:%S"))
        print((max(stamps) + dt.timedelta(seconds=1)).strftime("%Y-%m-%d %H:%M:%S"))
        return 0
    ticks = expected_ticks(args)
    if args.command == "generate":
        generate(args.archive, ticks)
    count = validate(args.archive, ticks)
    print(f"fixture=valid kind={args.kind} ticks={len(ticks)} members={count}")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, KeyError, IndexError, tarfile.TarError) as error:
        print(f"fixture generation/validation failed: {type(error).__name__}", file=sys.stderr)
        raise SystemExit(2)
