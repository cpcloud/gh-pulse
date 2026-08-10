<!--
SPDX-FileCopyrightText: 2026 Phillip Cloud

SPDX-License-Identifier: Apache-2.0
-->

# Contributor Guide

## Product Boundaries

`gh-pulse` answers one question: what is the health of GitHub's public service
right now, and what does its reconstructed uptime history look like?

- Keep the command focused on GitHub service health. Do not add account,
  repository, issue, pull-request, or notification features.
- The command must not require GitHub authentication, a token, a daemon, a
  database, or local configuration.
- Keep official current status and reconstructed history separate. Never join
  them to fill gaps or make one source appear more complete.
- Never invent uptime, interpolate outages, average component percentages, or
  label incomplete coverage as lifetime history.
- Never include response bodies in errors or logs.

## Architecture

- `cmd/gh-pulse` contains the executable entry point.
- `internal/statuspage` decodes the official current-status API.
- `internal/feed` decodes the official Atom history feed.
- `internal/history` decodes and calculates reconstructed uptime.
- `internal/snapshot` combines independent source results into the public model.
- `internal/tui` owns Bubble Tea state and terminal rendering.
- `internal/command` owns CLI behavior, dependency wiring, and exit codes.

Keep clients, calculations, orchestration, and rendering in separate packages.
The Bubble Tea model must not become the data or uptime calculation layer.

## User Interface

- Preserve a useful layout at 80x24, 120x40, and 200x50.
- Use color as reinforcement, never as the only status signal. Preserve the
  monochrome path.
- Refresh in place. Do not replace loaded content with a transient loading
  screen.
- Keep keyboard behavior small: `q` or `Esc` quits, `r` refreshes, and scrolling
  appears only when content does not fit.
- Treat JSON output as a stable agent-facing interface. It must contain no ANSI
  escapes or progress prose.

## Development

Enter the development environment with `nix develop`. Before committing, run:

```sh
goreleaser check
go test -race -count=1 ./...
golangci-lint run ./...
prek run --all-files
actionlint
nix build --no-link .#default
```

Changes to the release configuration also require a clean, non-publishing
snapshot build with `goreleaser release --snapshot --clean`.

Tests use Testify. Use `require` when later assertions depend on setup and
`assert` when collecting independent failures is useful. HTTP tests use local
`httptest` servers and deterministic clocks; they never contact production.
Committed tests must protect behavior that can drift independently, not mirror
configuration literals.

Run real-terminal visual checks in an isolated tmux socket. Kill the session and
remove every listener and temporary directory when finished. Do not publish
screenshots without explicit approval.

## Repository Policy

- Format Go with `gofmt` and `goimports`, and Nix with `nixfmt`.
- Fix every linter warning. Do not disable a check without a narrow explanatory
  comment.
- Keep GitHub Actions dependencies current and pinned to full commit hashes.
  Use GitHub-maintained actions or official actions published by the tool
  vendor. The approved vendors are Cachix for Nix, Docker for container tooling,
  and GoReleaser for releases.
- Release assets must keep GitHub CLI's `gh-pulse-OS-ARCH` naming convention.
- GoReleaser owns release binaries, checksums, and the multi-architecture GHCR
  image. Keep all binaries reproducible with `-trimpath`, preserve the version
  ldflag, and run the image as a non-root user in a minimal runtime.
- GHCR package visibility is a one-time owner setting after the first publish;
  the release workflow must not gain package-admin or delete permission to
  automate it.
- Do not commit credentials, private endpoints, local absolute paths, or real
  user data in fixtures.
