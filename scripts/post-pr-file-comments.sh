#!/usr/bin/env bash
#
# Post or update file-level hotspot review comments on a pull request.
#
# Usage: post-pr-file-comments.sh <hotspot-matches-tsv>
#
# Required env: GH_TOKEN, GITHUB_REPOSITORY, PR_NUMBER, HEAD_SHA
#
# The matches TSV is produced by filter-pr-hotspots.py. This script only
# renders comment bodies and creates or updates GitHub PR review comments.

set -euo pipefail

matches_tsv="${1:?hotspot matches TSV path required}"

: "${GH_TOKEN:?GH_TOKEN must be set}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY must be set}"
: "${PR_NUMBER:?PR_NUMBER must be set}"
: "${HEAD_SHA:?HEAD_SHA must be set}"

if [[ ! -f "$matches_tsv" ]]; then
  echo "hotspot matches TSV not found: $matches_tsv" >&2
  exit 1
fi

command -v gh >/dev/null 2>&1 || {
  echo "gh is required" >&2
  exit 1
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
template_dir="${script_dir}/templates"

body_file="$(mktemp)"
trap 'rm -f "$body_file"' EXIT

if [[ ! -s "$matches_tsv" ]]; then
  echo "No changed files matched hot-critical or cold-complex base-branch hotspots"
  exit 0
fi

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

json_escape() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//$'\n'/\\n}"
  value="${value//$'\r'/\\r}"
  value="${value//$'\t'/\\t}"
  printf '%s' "$value"
}

existing_comment_id() {
  local tag="$1"
  local escaped_tag

  escaped_tag="$(json_escape "$tag")"
  gh api --paginate \
    -H "Accept: application/vnd.github+json" \
    -H "X-GitHub-Api-Version: 2022-11-28" \
    "repos/${GITHUB_REPOSITORY}/pulls/${PR_NUMBER}/comments" \
    --jq ".[] | select((.body // \"\") | contains(\"${escaped_tag}\")) | .id" |
    sed -n '1p'
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
