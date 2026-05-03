#!/usr/bin/env python3
"""Filter base-branch hotspot JSON down to changed high-signal PR files."""

from __future__ import annotations

import argparse
import csv
import json
import sys
from pathlib import Path
from typing import Any


FLAGGED_QUADRANTS = {"cold-complex", "hot-critical"}
QUADRANT_PRIORITY = {
    "cold-complex": 0,
    "hot-critical": 1,
}


def main() -> int:
    parser = argparse.ArgumentParser(
        description=(
            "Compare hc analyze JSON with a newline-delimited changed-file list "
            "and emit TSV rows for PR file comments."
        )
    )
    parser.add_argument("hotspots_json", type=Path)
    parser.add_argument("changed_txt", type=Path)
    args = parser.parse_args()

    try:
        hotspots = load_hotspots(args.hotspots_json)
        changed = load_changed(args.changed_txt)
        matches = filter_matches(hotspots, changed)
        write_tsv(matches)
    except ValueError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    return 0


def load_hotspots(path: Path) -> list[dict[str, Any]]:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except OSError as exc:
        raise ValueError(f"reading {path}: {exc}") from exc
    except json.JSONDecodeError as exc:
        raise ValueError(f"parsing {path}: {exc}") from exc

    if not isinstance(data, list):
        raise ValueError("hotspots JSON must be a list")
    if not data:
        return []

    first = data[0]
    if not isinstance(first, dict):
        raise ValueError("hotspots JSON entries must be objects")
    if "files" in first and "total_commits" in first:
        raise ValueError(
            "directory-level analyze JSON is not supported; "
            "run hc analyze --json to produce file-level JSON"
        )

    entries: list[dict[str, Any]] = []
    for index, item in enumerate(data):
        if not isinstance(item, dict):
            raise ValueError(f"hotspots JSON entry {index} is not an object")
        entries.append(item)
    return entries


def load_changed(path: Path) -> set[str]:
    try:
        return {line for line in path.read_text(encoding="utf-8").splitlines() if line}
    except OSError as exc:
        raise ValueError(f"reading {path}: {exc}") from exc


def filter_matches(
    hotspots: list[dict[str, Any]], changed: set[str]
) -> list[dict[str, Any]]:
    matches = [
        item
        for item in hotspots
        if item.get("path") in changed and item.get("quadrant") in FLAGGED_QUADRANTS
    ]
    return sorted(matches, key=sort_key)


def sort_key(item: dict[str, Any]) -> tuple[int, float]:
    quadrant = str(item.get("quadrant", ""))
    return (
        QUADRANT_PRIORITY.get(quadrant, 99),
        -numeric(item.get("weighted_commits", item.get("commits", 0))),
    )


def numeric(value: Any) -> float:
    try:
        return float(value)
    except (TypeError, ValueError):
        return 0.0


def write_tsv(matches: list[dict[str, Any]]) -> None:
    writer = csv.writer(sys.stdout, delimiter="\t", lineterminator="\n")
    for item in matches:
        writer.writerow(
            [
                item.get("path", ""),
                item.get("quadrant", ""),
                item.get("commits", 0),
                item.get("weighted_commits", "n/a"),
                item.get("lines", 0),
                item.get("complexity", 0),
                item.get("authors", 0),
            ]
        )


if __name__ == "__main__":
    raise SystemExit(main())
