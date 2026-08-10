# CI Security Hardening Design

## Goal

Add independent CI checks for source vulnerabilities, known Go
vulnerabilities, suspicious Go constructs, and committed secrets. Harden every
GitHub-hosted job against unexpected network access without expanding its
`GITHUB_TOKEN` permissions.

This change does not alter application behavior, repository visibility,
release authority, or scrub-private-data work.

## Current State

The repository has CI jobs for native Go builds, Nix builds, and repository
quality checks. The release workflow publishes GitHub releases and GHCR images.
All current action references use full commit SHAs. CI has only `contents: read`;
the release job adds only `contents: write` and `packages: write`.

The repository is private. StepSecurity documents private-repository support as
an Enterprise feature, so the Harden-Runner and StepSecurity-maintained
TruffleHog steps require the repository's StepSecurity subscription to be
active. The workflows will not change visibility to avoid that prerequisite.
GitHub also requires Code Security to be enabled before a private repository
can upload CodeQL results. That setting has not been verified in this session;
if it is disabled, the CodeQL job will fail without a fallback or visibility
change until an owner enables it.

## Design

### Runtime Hardening

Add `step-security/harden-runner` as the first step of every existing and new
job. Every supported runner uses `egress-policy: block` with a job-specific
`allowed-endpoints` list. Endpoint lists stay in the workflow files so changes
remain reviewable with the code they protect.

The first allowlists cover the endpoints required by GitHub, Go modules, Nix,
CodeQL, GHCR, vulnerability data, and StepSecurity. An omitted legitimate
endpoint is an intentional CI failure. The allowlist must be expanded from a
specific failed call, not weakened to audit mode or a broad internet wildcard.

Harden-Runner v2.20.0 added block mode for GitHub-hosted macOS and Windows.
Version v2.20.1 is the current release and will be pinned. Its current source
still rejects Windows ARM64 before installing the agent. The existing
`windows-11-arm` matrix leg will therefore include a step immediately after
Harden-Runner that prints an unsupported-platform error and exits nonzero. This
prevents the job from continuing without enforcement. The explicit failure
step can be removed when a pinned upstream release supports Windows ARM64.

### Security Analysis

Add a read-only `Security` workflow with two Linux jobs:

- `Go security` checks out the repository, sets up the Go version from
  `go.mod`, runs `go vet ./...`, runs gosec v2.28.0 over `./...`, and runs
  govulncheck v1.6.0 over `./...`.
- `Secret scan` checks out full Git history and runs the
  StepSecurity-maintained TruffleHog action. The action is pinned to its current
  v3.95.9 commit. Its `version` input pins the TruffleHog scanner to v3.96.0,
  while `extra_args` selects verified and unknown results.

The Go security tools run as pinned Go modules after the existing official Go
setup action. This avoids adding action wrappers where the upstream command is
the stable interface. Findings and tool errors fail their jobs directly; no
`continue-on-error`, `-no-fail`, or success-shaped fallback is added.

The workflow runs for pull requests and pushes to `main`, matching existing CI.
It adds no schedule, manual trigger, secrets, or write permission.

### CodeQL

Add a separate `CodeQL` workflow for Go. Keeping CodeQL separate confines
`security-events: write` to its analysis job. The job also has only
`contents: read` and `actions: read`.

The job initializes CodeQL for Go, uses the supported autobuild path, and
uploads the analysis. It runs for pull requests and pushes to `main`. It uses
the default security query suite and adds no schedule.

### Existing Workflows

The CI and release workflows keep their current triggers, job matrices,
commands, timeouts, concurrency, and token permissions. Their only behavioral
change is the first-step Harden-Runner block policy plus the explicit Windows
ARM64 unsupported-platform failure.

Release retains exactly `contents: write` and `packages: write`. No package
administration, visibility, deletion, identity-token, or security-event
permission is added to release.

## Verified Versions

The implementation uses versions verified from the upstream repositories on
2026-08-10:

| Dependency | Version | Full commit SHA |
| --- | --- | --- |
| `github/codeql-action` | v4.37.6 | `5595ccaf912efad79be6eef63a5619ff05969be3` |
| `step-security/harden-runner` | v2.20.1 | `b09bb98e06d4d774595224525879c09bc6e98c40` |
| `step-security/trufflehog-action` | v3.95.9 | `72d1f67314fcd8ca707e19692ff0cb670d7d02c7` |
| `actions/checkout` | v7.0.1 | `3d3c42e5aac5ba805825da76410c181273ba90b1` |
| `actions/setup-go` | v7.0.0 | `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e` |
| gosec module | v2.28.0 | `603c0aefdd7112af1dd8884e40f9c3a2d90a105a` |
| govulncheck module | v1.6.0 | `19b0bb6a272792b9afa8a6983c3e9b9a1816947f` |
| TruffleHog scanner | v3.96.0 | `6f3c981e7b77f235fd2702dd74af25fc4b72bf11` |

Every `uses:` entry remains pinned to a 40-character commit SHA with a version
comment. Direct Go tools use explicit module versions.

## Failure Behavior

- Static-analysis findings fail only their named job and remain visible as
  ordinary Actions logs.
- CodeQL upload failures fail the CodeQL job; results are never discarded to
  preserve a green check.
- TruffleHog findings or verification errors fail the secret-scan job.
- Harden-Runner blocks unlisted egress. Missing endpoints are fixed narrowly
  from observed failures.
- Windows ARM64 fails explicitly while the pinned Harden-Runner release lacks
  agent support.
- No workflow logs response bodies or adds credentials.

## Verification

This is workflow configuration, so no committed test will mirror YAML literals.
Verification uses the format's consumers and the repository's existing gates:

- `actionlint`
- `prek run --all-files`
- `goreleaser check`
- `go test -race -count=1 ./...`
- `golangci-lint run ./...`
- `nix build --no-link .#default`

The new Go security commands also run locally at their pinned versions. A clean
local TruffleHog check may be used as session verification, but it does not
replace an actual Actions run for Harden-Runner egress enforcement, CodeQL
upload, or StepSecurity subscription status.

## Alternatives Considered

Putting every scanner in the existing quality job was rejected because it
would mix runtime, static-analysis, and SARIF permissions and make failures less
specific. One monolithic security job was rejected because TruffleHog's Docker
and network needs differ from Go analysis. Audit-first Harden-Runner rollout
was rejected by explicit user direction; strict block mode is expected to fail
until each legitimate endpoint is enumerated.
