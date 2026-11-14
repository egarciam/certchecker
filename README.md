# certchecker

A Kubernetes operator that monitors the expiry of `kubernetes.io/tls` Secrets and sends
email alerts for certificates that are expiring or already expired.

## How it works

`certchecker` reconciles `CertificateMonitor` custom resources from the
`monitoring.egarciam.com/v1alpha1` API group. On each reconciliation it:

1. Lists every `kubernetes.io/tls` Secret in the cluster.
2. Parses the first certificate in `tls.crt` and reads its `NotAfter` date.
3. Classifies the certificate as `valid`, `expiring` (within
   `spec.expiryThresholdDays`), or `expired`.
4. Records expiring and expired certificates in `.status.monitoredCertificates`
   and aggregate counts in `.status.summary`.
5. Optionally sends one email per recipient for each certificate that needs
   attention, respecting `spec.emailCooldown`.

The controller also watches TLS Secrets and the recipients ConfigMap, so rotating
a certificate or updating recipients triggers a reconcile immediately. The
`spec.checkInterval` timer remains as a safety net.

## Monitoring scope and duplicate alerts

The `CertificateMonitor` resource is namespaced, but **certificate discovery is
cluster-wide**. Run a single `CertificateMonitor` per cluster. Multiple instances
inspect the same Secrets and can send duplicate notifications.

Keep the CR and its recipients ConfigMap in the same namespace, or set
`spec.emailRecipientsNamespace` explicitly. Namespace-scoped discovery is a
possible future mode; the current behavior is intentional.

### External certificate checks (experimental)

Setting `spec.discoverExternal: true` starts a certificate checker Pod on each
control-plane node. This mode requires:

- the `node-cert-checker` ServiceAccount,
- the `node-cert-checker` image,
- a `cert-paths-config` ConfigMap with `CERT_DIRS`, `DEBUG`,
  `DEFAULT_WARNING_DAYS`, and `DEFAULT_CHECK_INTERVAL_MINUTES`.

External results are reported through node annotations; the controller currently
records only that the checker Pods were created. Treat this mode as experimental.

## Configuration

```yaml
apiVersion: monitoring.egarciam.com/v1alpha1
kind: CertificateMonitor
metadata:
  name: certificatemonitor-sample
  namespace: default
spec:
  discoverInternal: true      # enable cluster-wide TLS Secret discovery
  discoverExternal: false     # experimental node certificate checker pods
  sendMail: true              # send email alerts
  checkInterval: 3600         # seconds between periodic checks (default 86400)
  emailCooldown: 86400        # seconds before re-alerting for one certificate
  expiryThresholdDays: 30     # days before expiry to report "expiring"
  emailRecipientsConfigMap: email-recipients-config
  emailRecipientsNamespace: default
```

| Field | Default | Description |
| --- | --- | --- |
| `discoverInternal` | `false` | Enables TLS Secret discovery. |
| `discoverExternal` | `false` | Enables experimental node certificate checker Pods. |
| `sendMail` | `false` | Enables email notifications. |
| `checkInterval` | `86400` | Seconds between scheduled checks. |
| `emailCooldown` | `86400` | Seconds to wait before re-notifying about the same certificate. `0` disables the cooldown. |
| `expiryThresholdDays` | `30` | Days before expiry at which a certificate is reported as expiring. |
| `emailRecipientsConfigMap` | `email-recipients-config` | ConfigMap containing the `emails` key. |
| `emailRecipientsNamespace` | CR namespace | Namespace of the recipients ConfigMap. |

### Recipients ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: email-recipients-config
  namespace: default
data:
  emails: '["oncall@example.com", "platform@example.com"]'
```

The `emails` value must be a JSON array of strings. If the ConfigMap is missing or
invalid, certificate discovery still runs; the monitor reports a warning event and
sets the `MailConfigured` condition to `False`.

### SMTP

Email delivery currently targets a MailHog-compatible SMTP server at
`mailhog-service.default.svc.cluster.local:1025`, without authentication, from
`no-reply@example.com`. A development deployment is available in
`test/mailhog/deployment.yaml`:

```sh
kubectl apply -f test/mailhog/deployment.yaml
```

Parameterizing the SMTP host, port, sender, and credentials is a planned
follow-up. Until then, adapt `sendMail` in `internal/controller/helper.go` or run a
compatible SMTP relay.

## Status

`kubectl get cmt` shows a compact view:

```
NAME                        READY   TOTAL   EXPIRING   EXPIRED   AGE
certificatemonitor-sample   True    12      2          0         5m
```

The status also contains:

- `.status.summary` – `total`, `valid`, `expiring`, `expired`, and `errors`.
- `.status.monitoredCertificates` – name, status, expiry, namespace, and email
  bookkeeping for expiring/expired certificates.
- `.status.lastChecked` – timestamp of the last successful discovery.
- `.status.observedGeneration` – generation last reconciled.
- `.status.conditions` – `Ready`, `Degraded`, and `MailConfigured`.

`EmailSent` and `LastEmailSentAt` are only updated after a successful delivery, so
failed sends are retried on the next reconcile.

## Metrics

The manager exposes Prometheus metrics on its metrics endpoint:

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `certchecker_certificate_expiry_timestamp_seconds` | gauge | `namespace`, `secret` | Certificate expiry as a Unix timestamp. |
| `certchecker_certificates` | gauge | `status` | Number of certificates by status. |
| `certchecker_emails_sent_total` | counter | `status` | Emails sent. |
| `certchecker_emails_failed_total` | counter | `status` | Email failures. |
| `certchecker_reconcile_errors_total` | counter | | Failed reconciliations. |
| `certchecker_reconcile_duration_seconds` | histogram | | Reconciliation duration. |

A `ServiceMonitor` is provided in `config/prometheus`. It is commented out in
`config/default/kustomization.yaml` because it requires the Prometheus Operator
CRDs. Enable the `../prometheus` resource there after installing the operator.

## Install

Build and push the manager image, then deploy:

```sh
make docker-build docker-push IMG=<registry>/check-certs:<tag>
make deploy IMG=<registry>/check-certs:<tag>
```

Alternatively, install the consolidated bundle:

```sh
kubectl apply -f dist/install.yaml
```

The bundle installs the CRD, RBAC, and the manager into the
`check-certs-system` namespace. It references the `controller:latest` image;
set the image with `make build-installer IMG=<registry>/check-certs:<tag>`
before publishing.

Create a monitor:

```sh
kubectl apply -k config/samples/
```

Uninstall:

```sh
kubectl delete -k config/samples/
make undeploy
make uninstall
```

## Development

Prerequisites: Go 1.21+, Docker, kubectl, and a Kubernetes cluster for deployment.

```sh
make manifests       # regenerate CRDs and RBAC
make generate        # regenerate deepcopy code
make fmt vet         # format and vet
make test            # envtest + unit tests
make lint            # golangci-lint
make run             # run the controller locally
```

If a newer Go toolchain is on `PATH` but `GOROOT` points at another version, run
the Make targets with the matching `go` binary (for example
`PATH=/usr/local/go/bin:$PATH make test`).

`Tiltfile` provides a live-reload development loop with Tilt.

## CI

`.github/workflows/test.yml` runs `make test` and `make lint` on pushes to `main`
and on pull requests. Protect `main` in the repository settings so both jobs are
required before merge.

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0
(<http://www.apache.org/licenses/LICENSE-2.0>).
