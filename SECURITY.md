# Security Policy

## Reporting a Vulnerability

Report suspected vulnerabilities privately through GitHub Security Advisories:

<https://github.com/rshade/ax-go/security/advisories/new>

Do not report vulnerabilities through public GitHub issues, pull requests,
Discussions, or social media before coordinated disclosure. Public reports may
force immediate disclosure before maintainers and downstream users have a
reasonable chance to patch.

Please include:

- The affected ax-go version, commit, or branch.
- A concise impact statement.
- Reproduction steps or a minimal proof of concept.
- Any relevant logs or output with secrets, tokens, and personal data removed.
- Whether the issue is already public or being reported to other projects.

## Response Targets

The maintainer will aim to:

- Acknowledge a complete private report within 72 hours.
- Provide an initial triage result within 7 calendar days.
- Send status updates at least every 14 calendar days until resolution or
  closure.

These are targets, not legal guarantees. Reports with active exploitation,
credential exposure, or a practical bypass of an ax-go safety primitive are
prioritized.

## Supported Versions

ax-go is currently pre-v1.0. Patch releases (`0.x.PATCH`) are
backward-compatible and are the preferred vehicle for security fixes on a
supported release line. Minor releases (`0.MINOR.0`) may contain breaking
changes under the project stability policy.

| Version line | Security support | Notes |
| ------------ | ---------------- | ----- |
| `0.3.x` | Supported | Current released minor line. |
| `0.2.x` and earlier | Not supported | Upgrade to the latest `0.3.x` patch or newer. |
| `main` / unreleased | Best effort | Fixes may land before the next tagged release. |

When a new minor version becomes the current released line, update this table in
the same change that updates the README compatibility matrix.

## Disclosure Policy

ax-go follows coordinated disclosure:

- Keep vulnerability details private until a fix or documented mitigation is
  available, unless there is active exploitation or another safety reason to
  disclose sooner.
- The default embargo target is up to 90 days from acknowledgment, adjusted by
  mutual agreement for severity, patch complexity, downstream impact, or active
  exploitation.
- Security fixes are released with the smallest practical supported-version
  change. If a fix requires a breaking change, the release and advisory will
  call that out explicitly under the project SemVer policy.
- After release, the advisory or release notes will summarize impact, affected
  versions, fixed versions, and any required operator action.

## Scope

In scope examples include:

- Secret, token, credential, or personally identifying information exposure in
  logs, envelopes, telemetry, labels, or machine payloads.
- Bypasses of bounded Hujson/config reads that can cause unbounded memory use or
  avoid the documented validation error path.
- TLS verification bypasses or unsafe outbound transport defaults introduced by
  ax-go.
- Reproducible bypasses of stream separation, `--dry-run`, idempotency-key
  handling, deterministic error envelopes, or other agent-safety primitives when
  the bypass creates a credible security impact.
- Prompt-injection-shaped or agent-confusion bugs in ax-go itself, such as
  untrusted input crossing into logs, prompts, schemas, or machine-readable
  output in a way that can cause unsafe agent behavior.

Out of scope examples include:

- Reports that only show oversized Hujson/config input is rejected with the
  documented validation error. A bypass of that size cap is in scope.
- Vulnerabilities in downstream CLIs, services, or deployments unless they are
  caused by ax-go behavior.
- Missing authentication or authorization in an exposed deployment of an
  adopting CLI. ax-go holds no credentials and implements no auth flow; exposed
  services must put authn/authz in front of the endpoint.
- Denial-of-service reports that require unrealistic traffic volume and do not
  identify a specific ax-go resource-safety bypass.
- Dependency CVE reports without a reachable ax-go code path, exploitability
  analysis, or available upstream fix.
- Social engineering, spam, phishing, physical attacks, or issues requiring
  control of a maintainer's machine or account.

## Safe Harbor

Good-faith security research is welcome when it:

- Avoids privacy violations, data destruction, persistence, and service
  disruption.
- Uses only the access needed to demonstrate the issue.
- Stops testing and reports promptly after discovering a vulnerability.
- Gives maintainers a reasonable opportunity to fix before public disclosure.

Research outside these boundaries may not qualify for coordinated handling.
