---
name: pick-issue
description: Choose and claim one ax-go roadmap issue, implement it through the repository workflow, prepare a pull request, and release the claim. Use when the user invokes pick-issue or asks to pick up a roadmap issue, optionally with an issue number.
---

# Pick a Roadmap Issue

Read and follow [the shared command](../../../.claude/commands/pick-issue.md)
before starting. That file is the maintained workflow; resolve its relative
links from `.claude/commands/` and run its shell commands from the repository
root unless a step changes directories.

Treat any issue number or other arguments in the user's invocation as inputs
to the command. Map `/speckit-*`, `/code-review`, and `/scout` references to
the corresponding available Codex skills. If a referenced command or skill
is unavailable, report that limitation rather than claiming to have run it.

Apply the current session's instructions and authorization when interpreting
Claude-specific tool or permission notes. Loading this skill alone does not
authorize external actions beyond the user's requested scope.
