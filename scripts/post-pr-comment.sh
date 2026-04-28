#!/usr/bin/env bash
#
# Post (or update) a sticky comment on a pull request.
#
# Usage: post-pr-comment.sh <body-file> [marker]
#
# Required env: GH_TOKEN, GITHUB_REPOSITORY, PR_NUMBER
#
# Re-running with the same marker updates the existing comment instead of
# appending a new one. The marker is embedded as an HTML comment on the
# first line of the posted body.

set -euo pipefail

body_file="${1:?body file path required}"
marker="${2:-hc-hotspots}"

: "${GH_TOKEN:?GH_TOKEN must be set}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY must be set}"
: "${PR_NUMBER:?PR_NUMBER must be set}"

if [[ ! -f "$body_file" ]]; then
  echo "body file not found: $body_file" >&2
  exit 1
fi

tag="<!-- sticky-comment: ${marker} -->"
tmp_body="$(mktemp)"
trap 'rm -f "$tmp_body"' EXIT
printf '%s\n\n' "$tag" > "$tmp_body"
cat "$body_file" >> "$tmp_body"

existing_id="$(
  gh api --paginate \
    "repos/${GITHUB_REPOSITORY}/issues/${PR_NUMBER}/comments" \
    --jq "[.[] | select(.body | startswith(\"${tag}\"))][0].id // empty"
)"

if [[ -n "$existing_id" ]]; then
  echo "Updating comment ${existing_id}"
  gh api --method PATCH \
    "repos/${GITHUB_REPOSITORY}/issues/comments/${existing_id}" \
    -F body=@"$tmp_body" >/dev/null
else
  echo "Creating new comment"
  gh api --method POST \
    "repos/${GITHUB_REPOSITORY}/issues/${PR_NUMBER}/comments" \
    -F body=@"$tmp_body" >/dev/null
fi
