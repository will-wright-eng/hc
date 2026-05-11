#!/usr/bin/env bash
#
# Post or update file-level hotspot review comments on a pull request.
#
# Usage: hc md comment < hotspots.json | post-pr-file-comments.sh
#
# Stdin is NDJSON: one {path, quadrant, tag, body} object per line, as
# produced by `hc md comment`. This script does no filtering or rendering —
# it only does the GitHub-API find-or-create.
#
# Required env: GH_TOKEN, GITHUB_REPOSITORY, PR_NUMBER, HEAD_SHA

set -euo pipefail

: "${GH_TOKEN:?GH_TOKEN must be set}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY must be set}"
: "${PR_NUMBER:?PR_NUMBER must be set}"
: "${HEAD_SHA:?HEAD_SHA must be set}"

command -v gh >/dev/null 2>&1 || {
  echo "gh is required" >&2
  exit 1
}

command -v jq >/dev/null 2>&1 || {
  echo "jq is required" >&2
  exit 1
}

body_file="$(mktemp)"
trap 'rm -f "$body_file"' EXIT

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

while IFS= read -r entry; do
  [[ -n "$entry" ]] || continue

  path="$(jq -r '.path' <<<"$entry")"
  tag="$(jq -r '.tag' <<<"$entry")"
  jq -r '.body' <<<"$entry" >"$body_file"

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
done

if [[ "$created" -eq 0 && "$updated" -eq 0 ]]; then
  echo "No changed files matched hot-critical or cold-complex base-branch hotspots"
  exit 0
fi

echo "Hotspot file comments complete: ${created} created, ${updated} updated"
