#!/usr/bin/env python3
"""Structural assertions over public pgcenter report/describe output."""

import argparse
import datetime as dt
import json
import re
import sys
import tarfile
from pathlib import Path

from fixtures import BASE, INDEXES, NEW, OLD, TABLES

HEADER_CELL = re.compile(r"\x1b\[37;1m(.*?)\x1b\[0m")
TIMESTAMP = re.compile(r"^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}), rate: ")
BAD_DIAGNOSTIC = re.compile(r"(?i)panic|runtime error|traceback|diff failed|invalid (?:number|result)|decode .*failed|advance read position failed")


class HarnessError(Exception):
    pass


class ProductMismatch(Exception):
    pass


def require(condition, message):
    if not condition:
        raise ProductMismatch(message)


def parse_report(text):
    lines = text.splitlines()
    samples = []
    current_header = None
    current_sample = None
    for line in lines:
        matches = list(HEADER_CELL.finditer(line))
        if matches:
            cells = [match.group(1) for match in matches]
            names = [value.rstrip() for value in cells]
            widths = [len(value) for value in cells]
            if not names or any(not name for name in names):
                raise HarnessError("malformed ANSI report header")
            starts = []
            position = 0
            for width in widths:
                starts.append(position)
                position += width
            current_header = {"cols": names, "widths": widths, "starts": starts}
            current_sample = None
            continue
        stamp = TIMESTAMP.match(line)
        if stamp:
            if current_header is None:
                raise HarnessError("sample has no header")
            current_sample = {"timestamp": stamp.group(1), "header": current_header, "rows": []}
            samples.append(current_sample)
            continue
        if current_sample is None or not line or line.startswith("INFO:"):
            continue
        if BAD_DIAGNOSTIC.search(line) or "zero or negative width" in line:
            current_sample = None
            continue
        header = current_sample["header"]
        values = []
        for index, start in enumerate(header["starts"]):
            if start >= len(line):
                values.append("")
                continue
            if index + 1 < len(header["starts"]):
                end = header["starts"][index + 1]
                values.append(line[start:end].strip())
            else:
                values.append(line[start:].strip())
        current_sample["rows"].append(values)
    return samples


def load_report(path, status):
    text = Path(path).read_text(encoding="utf-8")
    return text, parse_report(text)


def tick_time(index):
    return (BASE + dt.timedelta(seconds=index)).strftime("%Y/%m/%d %H:%M:%S")


def expect_clean(text, status):
    require(status == 0, f"report exited {status}")
    require(not BAD_DIAGNOSTIC.search(text), "panic/parser/diff diagnostic in report")
    require("zero or negative width" not in text, "zero-width rendering diagnostic in report")


def expect_sample(samples, index, tick, cols):
    require(len(samples) > index, f"missing sample {index}")
    sample = samples[index]
    require(sample["timestamp"] == tick_time(tick), f"sample {index} has wrong timestamp")
    require(sample["header"]["cols"] == cols, f"sample {index} has wrong column layout")
    require(len(sample["rows"]) == 2, f"sample {index} does not contain both fixture rows")
    return sample


def row_map(sample):
    return {row[0]: row for row in sample["rows"]}


def expect_metrics(sample, metric, expected):
    rows = row_map(sample)
    index = sample["header"]["cols"].index(metric)
    for identity, value in expected.items():
        require(identity in rows, f"missing row {identity}")
        require(rows[identity][index] == str(value), f"wrong {metric} delta for {identity}")


def expect_order(sample, identities):
    require([row[0] for row in sample["rows"]] == identities, "wrong requested/default row order")


def expect_width(sample, column, content_width):
    index = sample["header"]["cols"].index(column)
    require(sample["header"]["widths"][index] == content_width + 2, f"wrong recomputed width for {column}")


def no_truncation(text):
    require("~" not in text, "truncated value indicates stale column widths")


def metric_identities(screen):
    if screen == "tables":
        return "public.acc018_alpha", "public.acc018_zulu", "seq_scan"
    return "public.alpha.alpha_idx", "public.zulu.zulu_idx", "scan"


def expected_ages(tick):
    return [f"1 day 00:00:{tick:02d}", f"00:00:{tick:02d}"]


def assert_control(path, status, screen, layout, age):
    text, samples = load_report(path, status)
    expect_clean(text, status)
    cols = NEW[screen] if layout == "new" else OLD[screen]
    sample = expect_sample(samples, 0, 1, cols)
    alpha, zulu, metric = metric_identities(screen)
    expect_metrics(sample, metric, {alpha: 10, zulu: 20})
    if layout == "old" or age == "blank":
        expect_order(sample, [alpha, zulu] if layout == "new" else [zulu, alpha])
    else:
        expect_order(sample, [alpha, zulu])
    if layout == "new":
        expect_width(sample, "stats_age", 32)
        rows = row_map(sample)
        if age == "blank":
            require(rows[alpha][-1] == "" and rows[zulu][-1] == "", "blank ages became populated")
        else:
            expected = expected_ages(1)
            require([rows[alpha][-1], rows[zulu][-1]] == expected, "wrong populated ages")
    print(f"assertion=pass control={screen}/{layout}/{age} samples=1")


def assert_mixed(path, status, screen, pg19_layout):
    text, samples = load_report(path, status)
    expect_clean(text, status)
    require(len(samples) == 2, "mixed-version archive must produce exactly two readable samples")
    old_sample = expect_sample(samples, 0, 1, OLD[screen])
    second_cols = NEW[screen] if pg19_layout == "new" else OLD[screen]
    new_sample = expect_sample(samples, 1, 3, second_cols)
    alpha, zulu, metric = metric_identities(screen)
    expect_metrics(old_sample, metric, {alpha: 10, zulu: 30})
    expect_order(old_sample, [zulu, alpha])
    expect_metrics(new_sample, metric, {alpha: 123456789, zulu: 30})
    expect_order(new_sample, [alpha, zulu])
    expect_width(old_sample, metric, 8)
    expect_width(new_sample, metric, 9)
    if pg19_layout == "new":
        require(new_sample["header"]["cols"][-1] == "stats_age", "PG19 stats_age is not trailing")
        expect_width(new_sample, "stats_age", 32)
        rows = row_map(new_sample)
        expected = [f"1 day 00:00:{13:02d}", f"00:00:{13:02d}"]
        require([rows[alpha][-1], rows[zulu][-1]] == expected, "wrong PG19 stats_age values")
    else:
        require("stats_age" not in new_sample["header"]["cols"], "old PG19 capability used new layout")
    no_truncation(text)
    print(f"assertion=pass mixed={screen}/{pg19_layout} samples=2 entries_read=true")


def assert_describe(path, status, screen, feature):
    require(status == 0, f"describe exited {status}")
    lines = Path(path).read_text(encoding="utf-8").splitlines()
    marked = [line for line in lines if line.startswith("- stats_age")]
    previous = "- tidx_hit" if screen == "tables" else "- read,KiB"
    if not feature:
        require(not marked, f"unexpected stats_age description for {screen}")
        require(any(line.startswith(previous) for line in lines), "legacy describe tail missing")
        return
    require(len(marked) == 1, f"{screen} needs exactly one stats_age description")
    line = marked[0]
    fields = [field for field in line.split("\t") if field]
    require(fields == [
        "- stats_age", "stats_reset",
        "Age of collected statistics in the moment when stats are taken (PG 19+)",
    ], "stats_age origin/wording/PG19 marker mismatch")
    position = lines.index(line)
    require(position > 0 and lines[position - 1].startswith(previous), "stats_age description is not trailing")
    require(position + 1 < len(lines) and lines[position + 1] == "", "stats_age is not final column description")


def assert_legacy(path, status, screen):
    text, samples = load_report(path, status)
    expect_clean(text, status)
    sample = expect_sample(samples, 0, 1, OLD[screen])
    require("stats_age" not in sample["header"]["cols"], "legacy layout gained stats_age")
    alpha, zulu, _ = metric_identities(screen) if screen in NEW else (None, None, None)
    if screen in NEW:
        metric = "seq_scan" if screen == "tables" else "scan"
        expect_metrics(sample, metric, {alpha: 10, zulu: 20})
        expect_order(sample, [zulu, alpha])
    elif screen == "functions":
        index = sample["header"]["cols"].index("calls")
        values = sorted(int(row[index]) for row in sample["rows"])
        require(values == [10, 11], "functions fixture deltas were not read")
    elif screen == "sizes":
        index = sample["header"]["cols"].index("size,KiB")
        values = sorted(int(row[index]) for row in sample["rows"])
        require(values == [8, 9], "sizes fixture deltas were not read")
    print(f"assertion=pass legacy={screen} rows=2 entries_read=true")


def expect_new_age(sample, age, tick):
    alpha, zulu, _ = metric_identities("tables" if len(sample["rows"][0]) == len(TABLES) + 1 else "indexes")
    rows = row_map(sample)
    if age == "blank":
        require(rows[alpha][-1] == "" and rows[zulu][-1] == "", "blank stats_age rendered a value")
        expect_width(sample, "stats_age", 32)
    else:
        expected = expected_ages(tick)
        require([rows[alpha][-1], rows[zulu][-1]] == expected, "populated stats_age mismatch")
        expect_width(sample, "stats_age", 32)


def assert_layout_feature(path, status, screen, direction, age):
    text, samples = load_report(path, status)
    expect_clean(text, status)
    require(len(samples) == 2, "first new-layout sample was not a no-diff boundary")
    first_layout = "old" if direction == "old-to-new" else "new"
    second_layout = "new" if direction == "old-to-new" else "old"
    first_cols = NEW[screen] if first_layout == "new" else OLD[screen]
    second_cols = NEW[screen] if second_layout == "new" else OLD[screen]
    first = expect_sample(samples, 0, 1, first_cols)
    second = expect_sample(samples, 1, 3, second_cols)
    alpha, zulu, metric = metric_identities(screen)
    expect_metrics(first, metric, {alpha: 10, zulu: 20})
    expect_metrics(second, metric, {alpha: 30, zulu: 40})
    if first_layout == "new":
        expect_new_age(first, age, 1)
        expect_order(first, [alpha, zulu])
    else:
        expect_order(first, [zulu, alpha])
    if second_layout == "new":
        expect_new_age(second, age, 3)
        expect_order(second, [alpha, zulu])
        require(second["header"]["cols"][-1] == "stats_age", "old-to-new lost trailing stats_age")
    else:
        require("stats_age" not in second["header"]["cols"], "new-to-old retained stats_age")
        expect_order(second, [zulu, alpha])
        expect_width(second, metric, 8)
    no_truncation(text)
    print(f"assertion=pass layout={screen}/{direction}/{age} boundary=1 deltas=30,40")


def classify_layout_base(path, status, screen, direction, age):
    text, samples = load_report(path, status)
    require(status == 0, f"base report exited {status}; not classified as feature_missing")
    require(not BAD_DIAGNOSTIC.search(text), "panic/parser/diff failure is not the expected base checkpoint")
    first_layout = "old" if direction == "old-to-new" else "new"
    first_cols = NEW[screen] if first_layout == "new" else OLD[screen]
    first = expect_sample(samples, 0, 1, first_cols)
    alpha, zulu, metric = metric_identities(screen)
    expect_metrics(first, metric, {alpha: 10, zulu: 20})
    if first_layout == "new":
        expect_new_age(first, age, 1)
        expect_order(first, [alpha, zulu])
    else:
        expect_order(first, [zulu, alpha])

    if direction == "old-to-new" and age == "populated":
        require(len(samples) == 2, "populated old-to-new did not reach the first new-layout sample")
        require(samples[1]["timestamp"] == tick_time(2) and samples[1]["rows"] == [],
                "populated old-to-new must stop at its first new-layout row")
        require(text.count("zero or negative width, skip") == 1, "expected precise old-to-new width failure")
        return
    require("zero or negative width" not in text, "unexpected width failure at base checkpoint")
    require(len(samples) == 3, "base must expose both the cross-layout and same-layout samples")
    cross = samples[1]
    second = samples[2]
    require(cross["timestamp"] == tick_time(2), "missing base cross-layout sample")
    require(second["timestamp"] == tick_time(3), "wrong same-layout sample at base boundary")
    expect_metrics(cross, metric, {alpha: 3990, zulu: 3980})
    expect_metrics(second, metric, {alpha: 30, zulu: 40})
    if direction == "old-to-new":
        require(age == "blank", "invalid base classification branch")
        require(cross["header"]["cols"] == OLD[screen] and second["header"]["cols"] == OLD[screen],
                "blank old-to-new did not expose stale old headers")
        expect_order(cross, [alpha, zulu])
        expect_order(second, [alpha, zulu])
    else:
        require(cross["header"]["cols"] == NEW[screen] and second["header"]["cols"] == NEW[screen],
                "new-to-old did not expose stale new headers")
        expect_order(cross, [alpha, zulu])
        expect_order(second, [alpha, zulu])
        rows = row_map(second)
        require(rows[alpha][-1] == "" and rows[zulu][-1] == "", "stale trailing cell should be empty on old data")


def parse_timestamp(value):
    return dt.datetime.fromisoformat(value)


def duration_seconds(value):
    match = re.fullmatch(r"(?:(\d+) days? )?(\d+):(\d{2}):(\d{2})", value)
    if not match:
        raise ProductMismatch("active stats_age is not a second-precision duration")
    days, hours, minutes, seconds = (int(part or 0) for part in match.groups())
    return days * 86400 + hours * 3600 + minutes * 60 + seconds


def load_capture(db_path, before_path, after_path):
    data = json.loads(Path(db_path).read_text(encoding="utf-8"))
    data["host_before"] = float(Path(before_path).read_text().strip())
    data["host_after"] = float(Path(after_path).read_text().strip())
    return data


def validate_live_fixture(pre, post):
    require(pre.get("server_version_num") == "190000" and post.get("server_version_num") == "190000",
            "disposable cluster is not PG19")
    require(pre.get("database_name") == "acc018" and post.get("database_name") == "acc018",
            "capture did not use the fresh synthetic database")
    keys = ("database_stats_reset", "active_table_stats_reset", "active_index_stats_reset")
    for key in keys:
        require(pre.get(key) is not None, f"active fixture {key} is null before recording")
        require(pre[key] == post.get(key), f"active fixture {key} changed during recording")
    for key in ("untouched_table_stats_reset", "untouched_index_stats_reset"):
        require(pre.get(key) is None and post.get(key) is None, f"untouched fixture {key} became populated")
    require(parse_timestamp(pre["clock"]) <= parse_timestamp(post["clock"]), "DB clock moved backwards")
    require(pre["host_before"] <= pre["host_after"], "invalid pre-recording host clock bounds")
    require(post["host_before"] <= post["host_after"], "invalid post-recording host clock bounds")


def validate_live_archive(path):
    counts = {"tables": 0, "indexes": 0, "meta": 0, "sysinfo": 0}
    with tarfile.open(path, "r:*") as archive:
        for member in archive.getmembers():
            require(not member.name.startswith("autovacuum_scores."), "autovacuum_scores tar member exists")
            prefix = member.name.split(".", 1)[0]
            if prefix in counts:
                counts[prefix] += 1
    require(all(counts[name] == 3 for name in counts), "real recording does not contain exactly three ticks")
    print("assertion=pass archive_ticks=3 autovacuum_scores_members=0")


def live_age_bounds(sample, reset_value, pre, post):
    offsets = [
        parse_timestamp(pre["clock"]).timestamp() - pre["host_before"],
        parse_timestamp(pre["clock"]).timestamp() - pre["host_after"],
        parse_timestamp(post["clock"]).timestamp() - post["host_before"],
        parse_timestamp(post["clock"]).timestamp() - post["host_after"],
    ]
    sample_time = dt.datetime.strptime(sample["timestamp"], "%Y/%m/%d %H:%M:%S").astimezone().timestamp()
    reset = parse_timestamp(reset_value).timestamp()
    return sample_time + min(offsets) - reset - 1, sample_time + max(offsets) - reset + 1


def assert_live(args):
    pre = load_capture(args.pre_db, args.pre_host_before, args.pre_host_after)
    post = load_capture(args.post_db, args.post_host_before, args.post_host_after)
    validate_live_fixture(pre, post)
    validate_live_archive(args.archive)
    reports = {"tables": load_report(args.tables, args.tables_status), "indexes": load_report(args.indexes, args.indexes_status)}
    for screen, (text, samples) in reports.items():
        expect_clean(text, args.tables_status if screen == "tables" else args.indexes_status)
        expected_cols = OLD[screen] if args.mode == "control" else NEW[screen]
        require(len(samples) == 2, f"live {screen} report must contain two diffed samples from three ticks")
        for index, sample in enumerate(samples):
            require(sample["timestamp"] == tick_time_live(sample["timestamp"]), "invalid live sample timestamp")
            require(sample["header"]["cols"] == expected_cols, f"live {screen} has wrong layout")
        active = "public.acc018_active" if screen == "tables" else "public.acc018_active.acc018_active_idx"
        untouched = "public.acc018_untouched" if screen == "tables" else "public.acc018_untouched.acc018_untouched_idx"
        ages = []
        for sample in samples:
            rows = row_map(sample)
            require(active in rows and untouched in rows, f"active/untouched rows missing from {screen}")
            if args.mode == "control":
                continue
            require(rows[untouched][-1] == "", f"untouched {screen} stats_age is not blank")
            value = rows[active][-1]
            age = duration_seconds(value)
            reset_key = "active_table_stats_reset" if screen == "tables" else "active_index_stats_reset"
            lower, upper = live_age_bounds(sample, pre[reset_key], pre, post)
            require(lower <= age <= upper, f"active {screen} stats_age outside DB-derived +/-1s bounds")
            ages.append((sample["timestamp"], age))
        if args.mode == "feature":
            for previous, current in zip(ages, ages[1:]):
                previous_ts = dt.datetime.strptime(previous[0], "%Y/%m/%d %H:%M:%S")
                current_ts = dt.datetime.strptime(current[0], "%Y/%m/%d %H:%M:%S")
                elapsed = (current_ts - previous_ts).total_seconds()
                require(current[1] > previous[1], f"active {screen} stats_age did not grow")
                require(abs((current[1] - previous[1]) - elapsed) <= 1, f"active {screen} stats_age growth mismatch")
        print(f"assertion=pass live={screen} samples=2 active_untouched_read=true")
    if args.mode == "feature":
        record = Path(args.record).read_text(encoding="utf-8")
        named = [line for line in record.splitlines() if "autovacuum_scores" in line]
        require(named, "record INFO does not name autovacuum_scores")
        require(any(
            line.startswith("INFO:") and "not recordable" in line and "live TUI only" in line
            for line in named
        ), "record INFO lacks deliberate live-TUI-only reason")
        require(not any(re.search(r"(?i)version|not supported|unsupported", line) for line in named),
                "record INFO falsely attributes exclusion to PostgreSQL version")
        print("assertion=pass record_info=live_tui_only false_version_reason=false")


def tick_time_live(value):
    dt.datetime.strptime(value, "%Y/%m/%d %H:%M:%S")
    return value


def parse_args():
    parser = argparse.ArgumentParser()
    sub = parser.add_subparsers(dest="command", required=True)
    for name in ("mixed", "control", "layout-feature", "layout-base", "legacy"):
        command = sub.add_parser(name)
        command.add_argument("--output", required=True)
        command.add_argument("--status", type=int, required=True)
        if name != "legacy":
            command.add_argument("--screen", choices=("tables", "indexes"), required=True)
        if name == "mixed":
            command.add_argument("--pg19-layout", choices=("old", "new"), required=True)
        if name == "control":
            command.add_argument("--layout", choices=("old", "new"), required=True)
            command.add_argument("--age", choices=("blank", "populated"), required=True)
        if name.startswith("layout"):
            command.add_argument("--direction", choices=("old-to-new", "new-to-old"), required=True)
            command.add_argument("--age", choices=("blank", "populated"), required=True)
        if name == "legacy":
            command.add_argument("--screen", choices=("tables", "indexes", "functions", "sizes"), required=True)
    describe = sub.add_parser("describe")
    describe.add_argument("--output", required=True)
    describe.add_argument("--status", type=int, required=True)
    describe.add_argument("--screen", choices=("tables", "indexes"), required=True)
    describe.add_argument("--feature", action="store_true")
    live = sub.add_parser("live")
    live.add_argument("--mode", choices=("control", "feature"), required=True)
    live.add_argument("--archive", required=True)
    live.add_argument("--record", required=True)
    live.add_argument("--tables", required=True)
    live.add_argument("--tables-status", type=int, required=True)
    live.add_argument("--indexes", required=True)
    live.add_argument("--indexes-status", type=int, required=True)
    for prefix in ("pre", "post"):
        live.add_argument(f"--{prefix}-db", required=True)
        live.add_argument(f"--{prefix}-host-before", required=True)
        live.add_argument(f"--{prefix}-host-after", required=True)
    return parser.parse_args()


def main():
    args = parse_args()
    if args.command == "mixed":
        assert_mixed(args.output, args.status, args.screen, args.pg19_layout)
    elif args.command == "control":
        assert_control(args.output, args.status, args.screen, args.layout, args.age)
    elif args.command == "layout-feature":
        assert_layout_feature(args.output, args.status, args.screen, args.direction, args.age)
    elif args.command == "layout-base":
        classify_layout_base(args.output, args.status, args.screen, args.direction, args.age)
    elif args.command == "legacy":
        assert_legacy(args.output, args.status, args.screen)
    elif args.command == "describe":
        assert_describe(args.output, args.status, args.screen, args.feature)
    elif args.command == "live":
        assert_live(args)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except ProductMismatch as error:
        print(f"assertion=failed reason={error}", file=sys.stderr)
        raise SystemExit(1)
    except (HarnessError, OSError, ValueError, KeyError, IndexError, tarfile.TarError, json.JSONDecodeError) as error:
        print(f"fixture/harness parsing failed: {type(error).__name__}", file=sys.stderr)
        raise SystemExit(2)
