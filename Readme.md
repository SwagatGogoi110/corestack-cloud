# CoreStack

Local cloud emulators, one repository. Each cloud is an independent Go module that stands up a single HTTP endpoint speaking that cloud's wire protocols, so SDKs, CLIs, and terraform can run against it locally with no cloud account.

> **New here?** Start with [Instructions.md](Instructions.md) - the step-by-step runbook for building, running, and validating this package on a fresh machine. The short version: from the repo root, run `make ci` .

| Provider | Module | Services | Status | Docs |
|---|---|--:|---|---|
| [aws](aws/) | `github.com/corestack/corestack` | 61 | Code-complete (see aws/docs/VALIDATION.md) | [aws/README.md](aws/README.md) |
| [azure](azure/) | `github.com/corestack/corestack` | 25 | Code-complete (see azure/docs/VALIDATION.md) | [azure/README.md](azure/README.md) |
| [gcp](gcp/) | `github.com/corestack/corestack` | 18 | All services ported (REST surface); gRPC-primary services (pubsub, firestore, datastore) done as REST. gcs is a partial reference port. | [gcp/README.md](gcp/README.md) |

## Layout

```
corestack/
   Instructions.md       # getting-started runbook - read this first
   Makefile              # `make ci`, build-all, test-all, per-provider targets
   go.work               # ties the three modules together for local dev (go 1.23)
   aws/                  # AWS emulator module
   azure/                # Azure emulator module
   gcp/                  # GCP emulator modue
```

Each provider is a **standalone module**: it keeps its own `go.mod`, builds and tests on its own, and can be extracted from the monorepo unchanged. The `go.work` file only affects the local multi-module tooling; it doesn't alter ny module's identity.

## Quick start

Per provider (recommended - honors each module's own toolchain):

```sh
cd gcp   && make build && make vet && make test #GCP
cd aws   && make build && make vet && make test #AWS
cd azure && make build && make vet && make test #AZURE
```

or from the root, across providers:

```sh
make ci              # full gate: fmt-check + build + vet + test (all three)
make build-all       # build aws + azure + gcp
make test-all        # test aws + azure + gcp
make aws-smoke       # AWS offline end-to-end gate
```

`make ci` is the single pre-push / CI command. There's no platform specific CI config yet - that target is the contract, so a future Github Actions or GitLab file need only run `make ci`. It needs no network or Docker: just Go 1.23+.

Each provider also carries a **routing-invariant tests** that asserts no two services fight over the same route - dispatch-key collisions in AWS (`internal/server`), duplicate `ServiceType()`s in Azure (`internal/core`), and overlapping path claims in GCP (`cmd/corestack-gcp`). These caught real collisions during the port (e.g. GCP kafka cs pubsub on `/clusters/.../topics`) and now run under `go test` so regressions fail the build.