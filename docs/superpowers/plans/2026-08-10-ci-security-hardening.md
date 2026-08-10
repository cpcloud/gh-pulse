# CI Security Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add fail-closed runtime hardening, Go security analysis, secret
scanning, and CodeQL to GitHub Actions.

**Architecture:** Keep runtime enforcement beside each job and split scanners
by permission and runtime needs. Existing CI and release jobs retain their
authority and behavior except for strict egress enforcement; new Security and
CodeQL workflows isolate read-only scanning from SARIF upload permission.

**Tech Stack:** GitHub Actions, StepSecurity Harden-Runner, StepSecurity's
TruffleHog action, GitHub CodeQL, Go toolchain, gosec, govulncheck

## Global Constraints

- Add Harden-Runner as the first step of every job with `egress-policy: block`.
- Pin Harden-Runner v2.20.1 to
  `b09bb98e06d4d774595224525879c09bc6e98c40`.
- Pin the StepSecurity-maintained TruffleHog action v3.95.9 to
  `72d1f67314fcd8ca707e19692ff0cb670d7d02c7` and its scanner to v3.96.0.
- Pin CodeQL v4.37.6 to the dereferenced commit
  `5595ccaf912efad79be6eef63a5619ff05969be3`.
- Run gosec v2.28.0 and govulncheck v1.6.0 as versioned Go modules.
- Keep every `uses:` value pinned to a 40-character commit SHA.
- Preserve existing triggers, timeouts, concurrency, release authority, and
  least-privilege token permissions.
- Omit `windows-11-arm` from the CI test matrix until Harden-Runner supports its
  agent, while preserving the GoReleaser Windows ARM64 build target.
- Treat missing egress endpoints as CI failures; do not add audit fallback,
  broad internet wildcards, `continue-on-error`, or ignored findings.
- Do not change repository visibility or scrub-private-data work.
- Do not commit mirror tests for workflow literals; use `actionlint` and the
  repository's real checks.

---

### Task 1: Harden Existing CI And Release Jobs

**Files:**

- Modify: `.github/workflows/ci.yml`
- Modify: `.github/workflows/release.yml`

**Interfaces:**

- Consumes: Existing CI matrices, commands, and release permissions.
- Produces: First-step block policies for all retained jobs; Windows ARM64
  remains a GoReleaser target but is omitted from the CI test matrix.

- [ ] **Step 1: Record the format-check baseline**

Run:

```bash
nix develop --command actionlint
```

Expected: PASS before editing. This is configuration-only work, so no committed
test is added; a mirror assertion could fail only when someone edits the YAML
literal it copies.

- [ ] **Step 2: Add the CI hardening steps**

Remove the unsupported `windows-11-arm` entry from the `go` test matrix, then
insert this as the first step in the retained matrix jobs:

```yaml
- name: Harden runner
  uses: step-security/harden-runner@b09bb98e06d4d774595224525879c09bc6e98c40 # v2.20.1
  with:
    egress-policy: block
    allowed-endpoints: >
      api.github.com:443
      github.com:443
      github-releases.githubusercontent.com:443
      objects.githubusercontent.com:443
      proxy.golang.org:443
      release-assets.githubusercontent.com:443
      storage.googleapis.com:443
      sum.golang.org:443
```

Insert the same pinned action as the first step in `nix` with this job-specific
allowlist:

```yaml
allowed-endpoints: >
  api.github.com:443
  cache.nixos.org:443
  codeload.github.com:443
  github.com:443
  github-releases.githubusercontent.com:443
  objects.githubusercontent.com:443
  proxy.golang.org:443
  releases.nixos.org:443
  storage.googleapis.com:443
  sum.golang.org:443
```

Insert it as the first step in `quality` with the union needed by `nix develop`
and the Go checks run by prek:

```yaml
allowed-endpoints: >
  api.github.com:443
  cache.nixos.org:443
  codeload.github.com:443
  github.com:443
  github-releases.githubusercontent.com:443
  objects.githubusercontent.com:443
  proxy.golang.org:443
  releases.nixos.org:443
  storage.googleapis.com:443
  sum.golang.org:443
```

Do not change any existing step, matrix entry, trigger, permission, timeout, or
concurrency value.

- [ ] **Step 3: Add release hardening**

Insert the pinned action as the first release step with the endpoints used by
checkout, setup-go, GoReleaser, GitHub releases, GHCR, and the distroless base
image:

```yaml
- name: Harden runner
  uses: step-security/harden-runner@b09bb98e06d4d774595224525879c09bc6e98c40 # v2.20.1
  with:
    egress-policy: block
    allowed-endpoints: >
      api.github.com:443
      auth.docker.io:443
      gcr.io:443
      ghcr.io:443
      github.com:443
      github-releases.githubusercontent.com:443
      objects.githubusercontent.com:443
      pkg-containers.githubusercontent.com:443
      production.cloudflare.docker.com:443
      proxy.golang.org:443
      release-assets.githubusercontent.com:443
      registry-1.docker.io:443
      storage.googleapis.com:443
      sum.golang.org:443
      uploads.github.com:443
```

Keep `contents: write` and `packages: write` unchanged and add no other
permission.

- [ ] **Step 4: Validate the existing workflows and repository**

Run:

```bash
nix develop --command goreleaser check
nix develop --command go test -race -count=1 ./...
nix develop --command golangci-lint run ./...
nix develop --command prek run --all-files
nix develop --command actionlint
nix build --no-link .#default
nix develop --command goreleaser release --snapshot --clean
```

Expected: every command exits zero. The snapshot release is required because
the release workflow changed.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/ci.yml .github/workflows/release.yml
git commit -m "ci: harden existing Actions jobs"
```

### Task 2: Add Go Security And Secret Scanning

**Files:**

- Create: `.github/workflows/security.yml`

**Interfaces:**

- Consumes: `go.mod`, full Git history, the pinned Go and TruffleHog tools.
- Produces: Independent `Go security` and `Secret scan` checks with read-only
  repository permission.

- [ ] **Step 1: Create the Security workflow**

Create a workflow triggered by pull requests and pushes to `main`, with
workflow-level `contents: read`, concurrency group
`security-${{ github.ref }}`, and cancellation of superseded runs.

The `Go security` job runs on `ubuntu-latest`, times out after 20 minutes, and
contains these steps in order:

```yaml
- name: Harden runner
  uses: step-security/harden-runner@b09bb98e06d4d774595224525879c09bc6e98c40 # v2.20.1
  with:
    egress-policy: block
    allowed-endpoints: >
      api.github.com:443
      github.com:443
      github-releases.githubusercontent.com:443
      objects.githubusercontent.com:443
      proxy.golang.org:443
      release-assets.githubusercontent.com:443
      storage.googleapis.com:443
      sum.golang.org:443
      vuln.go.dev:443

- name: Check out source
  uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
  with:
    persist-credentials: false

- name: Set up Go
  uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
  with:
    go-version-file: go.mod
    cache-dependency-path: go.sum

- name: Vet
  run: go vet ./...

- name: Run gosec
  run: go run github.com/securego/gosec/v2/cmd/gosec@v2.28.0 ./...

- name: Check known vulnerabilities
  run: go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
```

The `Secret scan` job runs on `ubuntu-latest`, times out after 15 minutes, and
contains these steps in order:

```yaml
- name: Harden runner
  uses: step-security/harden-runner@b09bb98e06d4d774595224525879c09bc6e98c40 # v2.20.1
  with:
    egress-policy: block
    allowed-endpoints: >
      agent.api.stepsecurity.io:443
      api.github.com:443
      ghcr.io:443
      github.com:443
      objects.githubusercontent.com:443
      pkg-containers.githubusercontent.com:443

- name: Check out full history
  uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
  with:
    fetch-depth: 0
    persist-credentials: false

- name: Scan for secrets
  uses: step-security/trufflehog-action@72d1f67314fcd8ca707e19692ff0cb670d7d02c7 # v3.95.9
  with:
    version: 3.96.0
    # Blocked provider verification is an unknown result and fails closed.
    extra_args: --results=verified,unknown --fail-on-scan-errors
```

Provider verification endpoints are intentionally absent because they vary by
detector and candidate. TruffleHog classifies blocked verification as
`unknown`; the selected result categories preserve that candidate as a failing
finding instead of hiding it.

Do not add secrets, write permissions, schedules, manual triggers, ignored
findings, or fallback behavior.

- [ ] **Step 2: Validate the Security workflow**

Run:

```bash
nix develop --command actionlint .github/workflows/security.yml
```

Expected: PASS with no diagnostics.

- [ ] **Step 3: Run the pinned Go security tools locally**

Run:

```bash
nix develop --command go vet ./...
nix develop --command go run github.com/securego/gosec/v2/cmd/gosec@v2.28.0 ./...
nix develop --command go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
```

Expected: every command exits zero with no findings. Fix source findings rather
than suppressing them.

- [ ] **Step 4: Run the repository gates**

Run each command independently:

```bash
nix develop --command goreleaser check
nix develop --command go test -race -count=1 ./...
nix develop --command golangci-lint run ./...
nix develop --command prek run --all-files
nix develop --command actionlint
nix build --no-link .#default
```

Expected: every command exits zero.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/security.yml
git commit -m "ci: add Go and secret security checks"
```

### Task 3: Add CodeQL Analysis

**Files:**

- Create: `.github/workflows/codeql.yml`

**Interfaces:**

- Consumes: GitHub Code Security, Go source, and the pinned CodeQL action.
- Produces: A Go CodeQL check whose job alone can upload SARIF results.

- [ ] **Step 1: Create the CodeQL workflow**

Create a workflow triggered by pull requests and pushes to `main`, with
concurrency group `codeql-${{ github.ref }}` and cancellation of superseded
runs. Define no workflow-level write permission. The single `Analyze Go` job
runs on `ubuntu-latest`, times out after 30 minutes, and has exactly these
permissions:

```yaml
permissions:
  actions: read
  contents: read
  security-events: write
```

Add these steps in order:

```yaml
- name: Harden runner
  uses: step-security/harden-runner@b09bb98e06d4d774595224525879c09bc6e98c40 # v2.20.1
  with:
    egress-policy: block
    allowed-endpoints: >
      api.github.com:443
      github.com:443
      github-releases.githubusercontent.com:443
      objects.githubusercontent.com:443
      proxy.golang.org:443
      release-assets.githubusercontent.com:443
      storage.googleapis.com:443
      sum.golang.org:443
      uploads.github.com:443

- name: Check out source
  uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
  with:
    persist-credentials: false

- name: Set up Go
  uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
  with:
    go-version-file: go.mod
    cache-dependency-path: go.sum

- name: Initialize CodeQL
  uses: github/codeql-action/init@5595ccaf912efad79be6eef63a5619ff05969be3 # v4.37.6
  with:
    languages: go

- name: Autobuild
  uses: github/codeql-action/autobuild@5595ccaf912efad79be6eef63a5619ff05969be3 # v4.37.6

- name: Analyze
  uses: github/codeql-action/analyze@5595ccaf912efad79be6eef63a5619ff05969be3 # v4.37.6
```

Keep setup-go caching enabled. Harden-Runner automatically detects the GitHub
Actions cache endpoints, so they do not belong in the manual endpoint list.

Do not add a schedule, custom query suite, ignored upload failure, or any
permission beyond the three listed above. If Code Security is disabled for this
private repository, retain the fail-closed job and report the repository
setting as a prerequisite.

- [ ] **Step 2: Validate the CodeQL workflow and every action pin**

Run:

```bash
nix develop --command actionlint .github/workflows/codeql.yml
rg -nP 'uses:\s+[^\s]+@(?![0-9a-f]{40}(?:\s|$))' .github/workflows
```

Expected: actionlint passes and `rg` prints no unpinned `uses:` entries.

- [ ] **Step 3: Run the complete repository gates**

Run each command independently:

```bash
nix develop --command goreleaser check
nix develop --command go test -race -count=1 ./...
nix develop --command golangci-lint run ./...
nix develop --command prek run --all-files
nix develop --command actionlint
nix build --no-link .#default
nix develop --command goreleaser release --snapshot --clean
```

Expected: every command exits zero. The snapshot release is required because
the release workflow changed even though `.goreleaser.yaml` did not.

- [ ] **Step 4: Audit the final diff**

Confirm that all new workflows are least privilege, every retained job begins
with Harden-Runner, Windows ARM64 remains a GoReleaser target but not a CI test
matrix entry, the TruffleHog action is the StepSecurity-maintained fork, every
action uses a 40-character commit SHA, and no scrub-private-data work or
unrelated file changed.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/codeql.yml
git commit -m "ci: add CodeQL analysis"
```
