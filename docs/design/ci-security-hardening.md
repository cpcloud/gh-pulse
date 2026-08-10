<!--
SPDX-FileCopyrightText: 2026 Phillip Cloud

SPDX-License-Identifier: Apache-2.0
-->

# CI Security Hardening

This is the living design for GitHub Actions security in `gh-pulse`. Keep it in
sync with the workflows whenever permissions, runner coverage, scanners,
network policy, or pinned dependencies change.

## Purpose

The workflows protect the repository in four complementary ways:

- restrict network access from every supported GitHub-hosted job;
- detect suspicious Go constructs and known Go vulnerabilities;
- scan Git history for committed secrets;
- run CodeQL analysis with narrowly scoped upload permission.

These controls do not alter application behavior, release authority,
repository visibility, or the separate private-data scrubbing work.

## Workflow Model

| Workflow | Trigger | Jobs | Token authority |
| --- | --- | --- | --- |
| `CI` | Pull requests and pushes to `main` | Native Go, Nix, repository quality | `contents: read` |
| `Security` | Pull requests and pushes to `main` | Go security, secret scan | `contents: read` |
| `CodeQL` | Pull requests and pushes to `main` | Go analysis | `actions: read`, `contents: read`, `security-events: write` |
| `Release` | Tags matching `v*` | GitHub release and container publication | `contents: write`, `packages: write` |

CodeQL is separate from read-only security checks so only its analysis job can
upload SARIF results. Release keeps only the authority needed for GitHub
releases and GHCR publication. No job receives package administration,
deletion, identity-token, or repository-administration permission.

Every checkout sets `persist-credentials: false`.

## Runtime Enforcement

Every retained job begins with
`step-security/harden-runner@b09bb98e06d4d774595224525879c09bc6e98c40`
and uses `egress-policy: block`. Each job owns a narrow endpoint allowlist in
its workflow file so reviewers can evaluate network access beside the command
that needs it.

The allowlists cover these dependency groups:

| Job | Allowed dependency groups |
| --- | --- |
| Native Go | GitHub action assets, Go distribution, and Go modules |
| Nix | GitHub action assets, Nix installation and cache endpoints, and build dependencies |
| Repository quality | Nix dependencies plus the Go endpoints used by repository hooks |
| Go security | GitHub action assets, Go modules, and the Go vulnerability database |
| Secret scan | StepSecurity telemetry, GitHub checkout, and the TruffleHog container image |
| CodeQL | GitHub action and release assets, Go dependencies, and CodeQL result upload |
| Release | GitHub releases, Go dependencies, GHCR, Docker Hub, and the distroless base image registry |

An omitted legitimate endpoint is a CI failure. The remedy is a narrow
allowlist addition supported by a blocked-call log or current upstream source.
Audit mode, broad internet wildcards, ignored network failures, and
`continue-on-error` are not acceptable fallbacks.

Harden-Runner v2.20.1 does not install its Windows agent on GitHub-hosted
Windows ARM64. That runner is omitted from the native Go test matrix so no test
job runs without enforcement. GoReleaser remains unchanged and continues to
build and archive `gh-pulse-windows-arm64.exe`.

## Go Security

The `Go security` job runs on `ubuntu-latest` and uses the Go version declared
by `go.mod`. It runs:

- `go vet ./...` for suspicious constructs recognized by the Go toolchain;
- gosec v2.28.0 over `./...` for source security findings;
- govulncheck v1.6.0 over `./...` for reachable known vulnerabilities.

gosec and govulncheck run as versioned Go modules. This uses each upstream
command directly instead of adding action wrappers. Findings and tool errors
fail the job without suppression.

## Secret Scanning

The `Secret scan` job checks out full Git history and runs the
StepSecurity-maintained TruffleHog action. The action is pinned to v3.95.9 and
installs TruffleHog v3.96.0.

TruffleHog receives:

```text
--results=verified,unknown --fail-on-scan-errors
```

Verified and unknown candidates fail the job. Scan errors also fail the job so
an incomplete scan cannot produce a successful check.

Credential-provider verification endpoints are detector- and
candidate-specific, so they are not broadly allowlisted. A blocked verification
attempt remains an unknown candidate and fails closed. This can require manual
triage, but it avoids hiding a possible credential exposure.

## CodeQL

The `CodeQL` workflow initializes Go analysis, uses the supported autobuild,
runs the default security queries, and uploads results. It has no schedule,
custom query suite, or ignored upload failure.

The repository is currently private and GitHub Code Security is disabled.
Analysis completes, but SARIF upload fails until the repository becomes public
or an owner enables Code Security. The workflow remains enabled and fail-closed
during that period. Repository visibility and security settings are managed
outside this change.

## Security Dependency Pins

These security dependency versions were verified against upstream repositories
on 2026-08-10.

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

Every `uses:` reference is pinned to a full commit SHA with a version comment.
Direct Go tools use explicit module versions. Dependency updates require a new
upstream verification of the release tag, dereferenced commit, supported
platforms, inputs, and network behavior.

## Failure Behavior

- Static-analysis findings fail only their named job and remain visible in
  ordinary Actions logs.
- CodeQL analysis or upload errors fail the CodeQL job.
- TruffleHog findings, verification uncertainty, and scan errors fail the
  secret-scan job.
- Harden-Runner blocks unlisted egress; legitimate omissions are corrected
  narrowly from evidence.
- No workflow logs response bodies or adds credentials.

## Verification

Workflow configuration is checked by its real consumers rather than committed
tests that mirror YAML literals. Before merging a workflow change, run:

```bash
nix develop -c goreleaser check
go test -race -count=1 ./...
golangci-lint run ./...
nix develop -c prek run --all-files
actionlint
nix build --no-link .#default
```

Changes affecting release behavior also require a non-publishing snapshot:

```bash
nix develop -c goreleaser release --snapshot --clean
```

Security command changes require their pinned commands as well:

```bash
go vet ./...
go run github.com/securego/gosec/v2/cmd/gosec@v2.28.0 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
```

An actual GitHub Actions run remains authoritative for Harden-Runner egress,
runner support, StepSecurity subscription status, and CodeQL upload.

## Design Decisions

The scanners remain separate because they have different permission, runtime,
and network needs. Combining them would broaden access and make failures less
specific.

Network enforcement remains beside each job rather than in a local composite
action. A local action would require checkout before enforcement, and a shared
endpoint union would grant jobs access they do not need.

Strict block mode is intentional. It trades rollout convenience for immediate
enforcement and makes every newly required destination an explicit reviewable
change.
