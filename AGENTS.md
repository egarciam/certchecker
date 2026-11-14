# AGENTS.md

Guidance for AI coding agents (OpenCode, Hermes, Claude, Codex, Copilot) working in
this repository. Humans are welcome to read it too.

## What this project is

`certchecker` is a Kubebuilder-scaffolded Kubernetes operator that watches
`kubernetes.io/tls` Secrets, parses certificate expiry, reports it through
`CertificateMonitor` status/conditions/metrics, and sends SMTP alerts.

- API group: `monitoring.egarciam.com/v1alpha1`, kind `CertificateMonitor`,
  short names `cmt` / `cmts`.
- Controller: `internal/controller/certificatemonitor_controller.go`.
- Email: `internal/email/sendmail.go` (gomail, config-driven).
- Deploy bundle: `dist/install.yaml` (generated — never hand-edit).

## Repository map

```
api/v1alpha1/            CertificateMonitor types, deepcopy, CRD markers
cmd/main.go              manager flags and reconciler wiring
internal/config/         env/flag-backed defaults for the node checker
internal/controller/     Reconcile, discovery, helpers, metrics, tests
internal/email/          SMTP Config + Send
internal/samples/        standalone examples
config/                  kustomize bases: crd, rbac, manager, samples, prometheus
test/                    envtest-e2e and MailHog fixtures
dist/install.yaml        generated single-file bundle
```

## Build, test, lint

Use the pinned Go toolchain. `go` on `PATH` can be newer than `GOROOT`, which
produces confusing errors — always prefer `/usr/local/go/bin/go` (Go 1.24.5).

```sh
export PATH=/usr/local/go/bin:$PATH

/usr/local/go/bin/go build ./...
/usr/local/go/bin/go vet ./...
make test            # manifests + generate + fmt + vet + envtest + unit tests
make lint            # golangci-lint v1.64.8 (do not "fix" it by bumping)
make build-installer # regenerate dist/install.yaml after API changes
```

Tests that need `envtest` require the pre-installed assets:

```sh
KUBEBUILDER_ASSETS="$(pwd)/bin/k8s/1.29.0-linux-amd64" \
  /usr/local/go/bin/go test ./api/... ./internal/... -count=1
```

Tool binaries are vendored under `bin/` (`controller-gen-v0.14.0`,
`kustomize-v5.3.0`, `golangci-lint-v1.64.8`, `k8s/1.29.0-linux-amd64`). Do not
download new versions casually.

## Conventions

- `main` is the integration branch. Work on a short-lived branch:
  `fix/...`, `feat/...`, `chore/...`, `refactor/...`; merge via PR.
- Do not force-push `main` or shared branches.
- Tag releases from `main` only.
- The operator writes Kubernetes Events for sends, send failures, missing
  recipients, and invalid SMTP configuration — add events for new failure modes.
- A misconfiguration must never abort certificate discovery: record a condition
  or event, skip the affected action, keep the status truthful.
- Unit tests use the controller-runtime fake client with
  `WithStatusSubresource`; inject `Now` and `MailSender` instead of sleeping or
  dialing real servers.
- New files may be hidden by the global gitignore in
  `/home/yayito/.dotfiles/.gitignore`; use `git add -f <path>` for new files
  (for example `.github/workflows/test.yml`, `internal/controller/metrics.go`).

## API and generated files

After changing `api/v1alpha1/certificatemonitor_types.go` run:

```sh
make manifests generate   # CRD in config/crd/bases + zz_generated.deepcopy.go
make build-installer      # dist/install.yaml
```

Commit the regenerated artifacts together with the code change.

## Current state (as of the v0.3.0 merge, PR #1)

Working and covered by tests:

- Cluster-wide TLS Secret discovery from a namespaced CR.
- Status summary, `LastChecked`, `ObservedGeneration`, conditions `Ready` /
  `Degraded` / `MailConfigured`, printer columns.
- Prometheus metrics for expiry timestamps, counts by status, emails sent/failed,
  and reconcile errors/duration.
- Secret + recipients ConfigMap watches; `checkInterval` requeue as safety net.
- Email is only marked sent after a successful delivery; failed sends retry.
- SMTP configuration: built-in defaults, then manager flags `--smtp-host`,
  `--smtp-port`, `--smtp-from`, then per-CR `smtpHost` / `smtpPort` /
  `smtpFrom` / `smtpTls` / `smtpCredentialsSecret` / `smtpCredentialsNamespace`.
- CI (`.github/workflows/test.yml`) runs unit/envtest and lint on Go 1.24.x.

Known gaps / next steps:

1. Duplicate alerts when more than one `CertificateMonitor` exists: documented,
   not prevented. A leader/ownership model is the likely fix.
2. `emailcooldown` → `emailCooldown` was a breaking rename; existing manifests must
   be updated (no conversion webhook).
3. Prometheus `ServiceMonitor` stays disabled in `config/default/kustomization.yaml`
   because it needs the Prometheus Operator CRDs.
4. External certificate checking (`discoverExternal`) is experimental: it creates
   checker Pods on control-plane nodes and only records that they were created.
5. Branch protection on `main` (require PR + green checks) is not configured.
6. A `v0.3.1` tag on the merged `36d7bdd` was considered but not created;
   `v0.3.0` points at the pre-merge `385a0d9`.

## Remote and credentials

`origin` uses SSH (`git@github.com:egarciam/certchecker.git`). If SSH fails with
`Permission denied (publickey)`, use an HTTPS remote with the authenticated `gh`
token for the push, or ask the user to unlock the key — do not force-push and do
not rewrite history.