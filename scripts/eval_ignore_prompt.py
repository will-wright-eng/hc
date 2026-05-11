#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.10"
# dependencies = []
# ///
"""
Evaluate `hc md ignore | claude -p` output coverage.

Runs the pipeline N times and checks whether each generated .hcignore
excludes the categories we expect for this repo: tests, docs, and config.
LLM output is non-deterministic, so we report a pass rate per category
rather than a single pass/fail.

Usage:
    uv run scripts/eval_ignore_prompt.py [-n N] [-o OUTDIR]
    # or, since the shebang invokes uv run:
    ./scripts/eval_ignore_prompt.py [-n N] [-o OUTDIR]
"""

from __future__ import annotations

import argparse
import re
import shutil
import subprocess
import sys
import tempfile
from dataclasses import dataclass
from pathlib import Path


@dataclass
class Category:
    name: str
    pattern: re.Pattern[str]


# A trial passes a category if any non-comment line in the generated
# .hcignore matches the category's regex. Patterns are matched against
# the body line-by-line (re.MULTILINE), so `^` anchors to a line start.
_M = re.MULTILINE
CATEGORIES: list[Category] = [
    Category(
        "tests",
        re.compile(r"_test\.[a-z]+|(^|/)tests?/|(^|/)testdata/|(^|/)__tests__/", _M),
    ),
    Category(
        "docs",
        re.compile(r"\.md(\b|$)|\.rst(\b|$)|(^|/)docs?/|README|CHANGELOG", _M),
    ),
    Category(
        "config",
        re.compile(
            r"(^|/)Makefile\b|\.ya?ml\b|\.toml\b|\.github/|"
            r"(^|/)\.[a-zA-Z]+ignore\b|Dockerfile|\.pre-commit",
            _M,
        ),
    ),
    Category(
        "lockfiles",
        re.compile(
            r"(^|/)go\.sum\b|package-lock\.json|yarn\.lock|"
            r"pnpm-lock\.yaml|poetry\.lock|uv\.lock|Cargo\.lock",
            _M,
        ),
    ),
    Category(
        "deps",
        re.compile(
            r"(^|/)vendor/|(^|/)node_modules/|(^|/)\.venv/|(^|/)site-packages/", _M
        ),
    ),
    Category(
        "build",
        re.compile(
            r"(^|/)dist/|(^|/)build/|(^|/)bin/|(^|/)target/|\.min\.(js|css)\b", _M
        ),
    ),
]
# Categories that must appear in *every* completed trial for the eval to pass.
# Others are reported but not required (informational coverage).
REQUIRED = {"tests", "docs", "config"}


def repo_root() -> Path:
    out = subprocess.check_output(["git", "rev-parse", "--show-toplevel"], text=True)
    return Path(out.strip())


def ensure_hc(root: Path) -> Path:
    hc = root / "hc"
    if not hc.exists():
        print("building hc...", file=sys.stderr)
        subprocess.check_call(["go", "build", "-o", "hc", "./cmd/hc"], cwd=root)
    return hc


def run_pipeline(hc: Path, out_path: Path) -> bool:
    """Run `hc md ignore | claude -p` writing stdout to out_path."""
    with out_path.open("wb") as out:
        prompt = subprocess.Popen([str(hc), "md", "ignore"], stdout=subprocess.PIPE)
        claude = subprocess.Popen(["claude", "-p"], stdin=prompt.stdout, stdout=out)
        assert prompt.stdout is not None
        prompt.stdout.close()  # let prompt receive SIGPIPE if claude exits
        claude_rc = claude.wait()
        prompt_rc = prompt.wait()
    return claude_rc == 0 and prompt_rc == 0


def pattern_lines(text: str) -> list[str]:
    """Strip comments and blank lines."""
    return [
        line
        for line in text.splitlines()
        if line.strip() and not line.lstrip().startswith("#")
    ]


def evaluate(text: str) -> dict[str, bool]:
    body = "\n".join(pattern_lines(text))
    return {c.name: bool(c.pattern.search(body)) for c in CATEGORIES}


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("-n", "--trials", type=int, default=5)
    ap.add_argument(
        "-o",
        "--outdir",
        type=Path,
        default=None,
        help="keep raw outputs in this directory",
    )
    args = ap.parse_args()

    root = repo_root()
    hc = ensure_hc(root)

    if shutil.which("claude") is None:
        print("claude CLI not found in PATH", file=sys.stderr)
        return 1

    cleanup_outdir = False
    if args.outdir is None:
        args.outdir = Path(tempfile.mkdtemp(prefix="eval-ignore-"))
        cleanup_outdir = True
    else:
        args.outdir.mkdir(parents=True, exist_ok=True)

    pass_count = {c.name: 0 for c in CATEGORIES}
    all_pass_trials = 0
    completed = 0

    try:
        for i in range(1, args.trials + 1):
            out_path = args.outdir / f"trial-{i}.hcignore"
            print(f"=== trial {i}/{args.trials} ===")
            if not run_pipeline(hc, out_path):
                print("  pipeline failed; skipping", file=sys.stderr)
                continue
            completed += 1

            results = evaluate(out_path.read_text())
            for name, ok in results.items():
                tag = "REQ " if name in REQUIRED else "opt "
                marker = "PASS" if ok else "MISS"
                print(f"  {tag}{marker}  {name}")
                if ok:
                    pass_count[name] += 1
            if all(results[n] for n in REQUIRED):
                all_pass_trials += 1
            print(f"  output: {out_path}")

        print()
        print(f"=== summary (completed {completed}/{args.trials}) ===")
        print(
            f"  trials covering all required categories: {all_pass_trials}/{completed}"
        )
        for c in CATEGORIES:
            tag = "REQ" if c.name in REQUIRED else "opt"
            print(f"  {tag} {c.name + ':':12s} {pass_count[c.name]}/{completed}")
    finally:
        if cleanup_outdir and args.outdir.exists() and completed == args.trials:
            shutil.rmtree(args.outdir)

    # Non-zero exit if any required category never appeared across trials.
    if completed == 0 or any(pass_count[name] == 0 for name in REQUIRED):
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
