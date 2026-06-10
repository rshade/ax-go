#!/usr/bin/env bash
# Wrapper: the generated speckit-review-* skills invoke this canonical path
# (.specify/scripts/bash/detect-changed-files.sh). The implementation lives in
# the review extension; delegate everything to it so the skills keep working
# even when they are regenerated.
exec "$(dirname "$0")/../../extensions/review/scripts/bash/detect-changed-files.sh" "$@"
