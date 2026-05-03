#!/usr/bin/env bash
#
# Post or update file-level hotspot review comments on a pull request.
#
# Usage: post-pr-file-comments.sh <hotspots-json>
#
# Required env: GH_TOKEN, GITHUB_REPOSITORY, PR_NUMBER, BASE_SHA, HEAD_SHA
#
# The hotspots JSON must come from running hc against BASE_SHA. This script
# intersects that historical hotspot list with the PR diff BASE_SHA...HEAD_SHA
# and posts file-level comments for hot-critical and cold-complex files.

set -euo pipefail

hotspots_json="${1:?hotspots JSON path required}"

: "${GH_TOKEN:?GH_TOKEN must be set}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY must be set}"
: "${PR_NUMBER:?PR_NUMBER must be set}"
: "${BASE_SHA:?BASE_SHA must be set}"
: "${HEAD_SHA:?HEAD_SHA must be set}"

if [[ ! -f "$hotspots_json" ]]; then
  echo "hotspots JSON not found: $hotspots_json" >&2
  exit 1
fi

command -v gh >/dev/null 2>&1 || {
  echo "gh is required" >&2
  exit 1
}

command -v python3 >/dev/null 2>&1 || {
  echo "python3 is required" >&2
  exit 1
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
filter_script="${script_dir}/filter-pr-hotspots.py"
template_dir="${script_dir}/templates"

changed_txt="$(mktemp)"
matches_tsv="$(mktemp)"
comments_json="$(mktemp)"
body_file="$(mktemp)"
trap 'rm -f "$changed_txt" "$matches_tsv" "$comments_json" "$body_file"' EXIT

git diff --name-only --diff-filter=ACM "$BASE_SHA...$HEAD_SHA" -- > "$changed_txt"
python3 "$filter_script" "$hotspots_json" "$changed_txt" > "$matches_tsv"

if [[ ! -s "$matches_tsv" ]]; then
  echo "No changed files matched hot-critical or cold-complex base-branch hotspots"
  exit 0
fi

gh api --paginate \
  --slurp \
  -H "Accept: application/vnd.github+json" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  "repos/${GITHUB_REPOSITORY}/pulls/${PR_NUMBER}/comments" > "$comments_json"

template_for_quadrant() {
  case "$1" in
    hot-critical)
      printf '%s\n' "${template_dir}/hotcritical.md"
      ;;
    cold-complex)
      printf '%s\n' "${template_dir}/coldcomplex.md"
      ;;
    *)
      return 1
      ;;
  esac
}

render_body() {
  local template="$1"
  local path="$2"
  local commits="$3"
  local score="$4"
  local lines="$5"
  local complexity="$6"
  local authors="$7"
  local tag="$8"

  cat "$template" > "$body_file"
  {
    printf '\n\n'
    printf '<details>\n'
    printf '<summary>Hotspot details</summary>\n\n'
    printf -- "- Base path: \`%s\`\n" "$path"
    printf -- '- Commits: %s\n' "$commits"
    if [[ "$score" != "n/a" ]]; then
      printf -- '- Score: %.1f\n' "$score"
    fi
    printf -- '- Lines: %s\n' "$lines"
    printf -- '- Complexity: %s\n' "$complexity"
    printf -- '- Authors: %s\n' "$authors"
    printf '\n</details>\n\n'
    printf '%s\n' "$tag"
  } >> "$body_file"
}

existing_comment_id() {
  local tag="$1"

  python3 - "$comments_json" "$tag" <<'PY'
import json
import sys

comments_path = sys.argv[1]
tag = sys.argv[2]

with open(comments_path, encoding="utf-8") as f:
    pages = json.load(f)

if isinstance(pages, dict):
    pages = [pages]

for page in pages:
    comments = page if isinstance(page, list) else [page]
    for comment in comments:
        if not isinstance(comment, dict):
            continue
        if tag in str(comment.get("body", "")):
            print(comment.get("id", ""))
            raise SystemExit(0)
PY
}

created=0
updated=0

while IFS=$'\t' read -r path quadrant commits score lines complexity authors; do
  [[ -n "$path" ]] || continue

  template="$(template_for_quadrant "$quadrant")"
  if [[ ! -f "$template" ]]; then
    echo "template not found for ${quadrant}: ${template}" >&2
    exit 1
  fi

  tag="<!-- hc-pr-comment:${path} -->"
  render_body "$template" "$path" "$commits" "$score" "$lines" "$complexity" "$authors" "$tag"

  existing_id="$(existing_comment_id "$tag")"

  if [[ -n "$existing_id" ]]; then
    echo "Updating hotspot review comment ${existing_id} for ${path}"
    gh api --method PATCH \
      -H "Accept: application/vnd.github+json" \
      -H "X-GitHub-Api-Version: 2022-11-28" \
      "repos/${GITHUB_REPOSITORY}/pulls/comments/${existing_id}" \
      -F body=@"$body_file" >/dev/null
    updated=$((updated + 1))
  else
    echo "Creating hotspot review comment for ${path}"
    gh api --method POST \
      -H "Accept: application/vnd.github+json" \
      -H "X-GitHub-Api-Version: 2022-11-28" \
      "repos/${GITHUB_REPOSITORY}/pulls/${PR_NUMBER}/comments" \
      -F body=@"$body_file" \
      -f commit_id="$HEAD_SHA" \
      -f path="$path" \
      -f subject_type=file >/dev/null
    created=$((created + 1))
  fi
done < "$matches_tsv"

echo "Hotspot file comments complete: ${created} created, ${updated} updated"
