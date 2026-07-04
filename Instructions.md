# INSTRUCTIONS - getting corestack-cloud running

A step-by-step runbook for taking this package from a fresh checkout to a running, validated set of local cloud emulators. Read this first; the `README.md` is the overview and the per-provider `docs/` folders have the deep detail.

---

## 0. What this is

`corestack-cloud` is a monorepo of three independent local cloud emulators, each a self-contained Go module:

| Provider | Module path | Default port | Services |
|---|---|---:|---:|
| `aws/`     | `github.com/corestack/corestack` | 4566 | 61
| `azure/`   | `github.com/corestack/corestack` | 4577 | 25
| `aws/`     | `github.com/corestack/corestack` | 4588 | 18

They emulate the ReST/JSON control-plan surfaces of each cloud so you can point real SDKs, `gcloud`/`aws`/`az` CLIs, and Terraform at `localhost` instead of the real thing. The modules are independent - none imports another - and each can be lifted out of the monorepo unchanged.

> **Read this before you start.** Every line of this code was written and audited *without a compiler or Docker in the loop*. It is structurally complete and internally consistent, but this working environment is where it gets compiled and for the **first time**. Expect the first build to surface a round of fixes - that is normal and expected, not a sign the package is broken. The steps below are ordered so the fastest failures come first.

---

## 1. Prerequisites

- **Go 1.23 or newer.** The workspace (`go.work`) pins 1.23 - the highest of the three modules (aws 1.23, azure 1.22, gcp 1.23). Check with `go version`.
- **make** (for the convenience targets; everything can also be run with raw `go` commands if you prefer)
- **Docker** - *optional*, only needed to run the container images or the AWS/Azure container-based services. Not needed to build or test.
- No network access is required for build, vet, or test. The code is pure Go standard library with no external module dependencies.

---

## 2. First build - do this first

From the repo root:

```sh
make ci
```

This is the single gate. It runs, fail-fast, across all three modules:

1. `fmt-check` - verifies everything is gofmt-clean
2. `build-all` - compile every module (`go build`)
3. `vet-all`   - `go vet` on every module
4. `test-all`  - `go test ./...` on every module, including the routing-invariant tests.

If `make ci` finishes clean, the package is healthy end-to-end. If it doesn't, that's expected on a first compile - see **Section 6** for how to read and fix the common cases. Fix, re-run `make ci`, repeat until green.

### Doing it per module instead

if you'd rather isolate one provider (and honor its own Go pin):

```sh
cd gcp   && make build && make vet && make test
cd aws   && make build && make vet && make test
cd azure && make build && make vet && make test
```

Start with **gcp** - it's the newest code and the most likely to need a fix.

---

## 3. Run an emulator locally (no Docker)

Each module builds a single binary and serves one HTTP front door.

```sh
# GCP on :4588
cd gcp && make run

# AWS on :4566
cd aws && make run

# Azure on :4577
cd azure && make run
```

Override the listen address with the provider's env var if a port is taken:
`CORESTACK_GCP_LISTEN=:8088` (GCP), `CORESTACK_LISTEN=:8066` (AWS). Azure reads its port from config (`internal/config`, default 4577)

### Point a client at it

GCP example (GCS):

```sh
export STORAGE_EMMULATOR_HOAT=http://localhost:4588
curl -X POST 'http://localhost:4588/storage/v1/b?project=my-proj' \
   -H 'Content-Type: application/json' -d '{"name": "my-bucket"}'
curl 'http://localhost:4588/storage/v1/b?project=my-proj'
```

Health check:

```sh
curl http://localhost:4588/health            #GCP -> {"status":"ok", ...}
curl http://localhost:4577/health            #Azure -> also /_corestack/health
curl http://localhost:4588/_corestack/health #GCP -> (uses the /_corestack path)
```

---

## 4. Run in Docker

Each provider has `docker/Dockerfile` + `docker-compose.yml` and Makefile targets:

```sh
cd gcp && make compose-up   # build + run detached on :4588
cd gcp && make compose-down # stop and remove
```

Same for `aws/` and `azure/`.

**Important difference between providers:**

- **AWS and Azure** mount the host Docker socket so their container-backed services (databases, brokers, etc.) can launch backing containers on demand. Their compose files run the service as root to read the socet.
- **GCP** does **not** - its container-tier services (Cloud Run, Cloud SQL, Managed Kafka) are *control-plane only*; the actual container is external-by-design. So the GCP image needs no socket and runs unprivileged.

See each provider's `docs/DOCKER.md` (where present) for the detailes.

---

## 5. Validate

After it builds and runs:

- **AWS** has an offline end-to-end smoke gate: `cd aws && make smoke`. It also has a `make compat` target.
- Each provider's **`docs/VALIDATION.md`** (AWS and Azure) gives the ordered first-run sequence - compile -> unit -> smoke -> run -> containers. Follow that for the most thorough check.
- The **routing-invariant tests** run as part of `make test` / `make ci`. They assert no two services in a provider fight over the same route. if one fails, the failure message names the colliding services or the unrouted path directly - it is pointing at a real registration bug, not a flaky test.

---

## 6. What to expect on the first compile and how to fix it)

This code has never been through a Go compiler. The most likely first-build issues, and what they mean:

- **`declated and not used`** - an unused local variable. The audit caught package-level unused *imports*, but the compiler is stricter about locals. Delete the variable or use `_`.
- **type mismatch at a call site** - e.g. a helper returning `map[string]any` where a struct was expected. Read the line the compiler points to; these are usually a one-line fix.
- **`go vet` warnings** - printf-style format mismatches, etc. Not fatal to a build, but `make ci` runs vet, so fix them to get a clean gate.
- **a routing-invariant test failing** - read it literally. GCP: "COLLISION - claimed by [a b]" means two services' `ClaimsPath` both match a path; fix one predicate. Azure: a duplicate `ServiceType()`. AWS: two services registering the same dispatch key.

General approach: run `make ci`, fix the first error it reports, re-run. Because the modules are independent, a failure in one doesn't block the others - you can work them one at a time with `cd <provider> && make build`.

---

## 7. Layout reference

```
corestack-cloud/
   Instructions.md         <- You are here
   Readme.md               overview + status
   Makefile                root: `make ci`, build-all, test-all, per-provider targets
   go.work                 ties the three modules together for local dev (go 1.23)
   aws/                    AWS emulator module
   azure/                  Azure emulator module (two binaries: corestack-az, floci-az)
   gcp/                    GCP emulator module
      cmd/corestack-gcp/   entry point (registers services, serves: 4588)
      internal/core/       storage, error envelope, router
      internal/services/   one pacakge per emulated service
      docs/                MIGRATION_PLAN.md, DOCKER.md
      Makefile             build / run / test / image / compose-up
```

---

The single highest-value next step is simply: **run `make ci`.**