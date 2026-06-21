#!/usr/bin/env bash
#
# Upsert the sticky go-apidiff PR comment.
#
# The comment is identified by a hidden marker so repeated workflow runs edit a
# single comment instead of stacking a new one on every push. Uses gh + jq only
# (no third-party action) to keep the privileged, pull-requests:write workflow's
# dependency surface minimal.
#
# Usage: apidiff-comment.sh <markdown-body-file>
# Requires env: GH_TOKEN, GITHUB_REPOSITORY, PR_NUMBER
set -euo pipefail

body_file="${1:?usage: apidiff-comment.sh <markdown-body-file>}"
marker="<!-- apidiff-report -->"

: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${PR_NUMBER:?PR_NUMBER is required}"

if ! grep -q "$marker" "$body_file"; then
  echo "apidiff-comment: body file '$body_file' is missing the marker; refusing to post" >&2
  exit 1
fi

# Stream matching comment ids across all pages; keep the first (oldest) so we
# always edit the same comment.
existing_id="$(
  gh api --paginate "repos/${GITHUB_REPOSITORY}/issues/${PR_NUMBER}/comments" \
    --jq ".[] | select(.body | contains(\"${marker}\")) | .id" | head -n 1
)"

# JSON-encode the markdown body via jq -Rs so backticks, quotes, and newlines
# round-trip safely; send it as the request body with --input -.
payload="$(jq -Rs '{body: .}' <"$body_file")"

if [ -n "$existing_id" ]; then
  echo "apidiff-comment: updating comment ${existing_id}"
  printf '%s' "$payload" |
    gh api -X PATCH "repos/${GITHUB_REPOSITORY}/issues/comments/${existing_id}" --input - >/dev/null
else
  echo "apidiff-comment: creating comment on PR #${PR_NUMBER}"
  printf '%s' "$payload" |
    gh api -X POST "repos/${GITHUB_REPOSITORY}/issues/${PR_NUMBER}/comments" --input - >/dev/null
fi
