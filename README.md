# go-api-boot
[![codecov](https://codecov.io/gh/SaiNageswarS/go-api-boot/branch/master/graph/badge.svg)](https://codecov.io/gh/SaiNageswarS/go-api-boot)
[![Go Report Card](https://goreportcard.com/badge/github.com/SaiNageswarS/go-api-boot)](https://goreportcard.com/report/github.com/SaiNageswarS/go-api-boot) [![Go Reference](https://pkg.go.dev/badge/github.com/SaiNageswarS/go-api-boot.svg)](https://pkg.go.dev/github.com/SaiNageswarS/go-api-boot)


> **Batteries‑included framework for building production‑grade gRPC + HTTP APIs in Go – with generics, MongoDB ODM, cloud utilities, zero‑config HTTPS, workers, and a one‑line bootstrap CLI.**

---

## 📑 Table of Contents

1. [Overview](#overview)
2. [Why go‑api‑boot?](#why-go-api-boot)
3. [Key Features](#key-features)
4. [Quick Start](#quick-start)

   1. [Bootstrap a New Service](#bootstrap-a-new-service)
   2. [Running Locally](#running-locally)
5. [Project Structure](#project-structure)
6. [Core Modules](#core-modules)

   * [Server](#server)
   * [ODM (MongoDB)](#odm-mongodb)
   * [Auth & JWT](#auth--jwt)
   * [Cloud Abstractions](#cloud-abstractions)
   * [Zero‑Config SSL/TLS](#zero-config-ssltls)
   * [Temporal Workers](#temporal-workers)
   * [LLM Clients](#llm-clients)
7. [CLI Reference](#cli-reference)
8. [Examples](#examples)
9. [Contributing](#contributing)
10. [License](#license)

---

## Overview

**go‑api‑boot** eliminates the repetitive plumbing required to ship modern API services in Go.  With a single CLI command you get:

* A fully wired **gRPC** server that also serves **gRPC‑Web** and **REST** gateways – no Envoy sidecars or extra proxies.
* MongoDB repositories implemented with **Go 1.22 generics**.
* Opinionated middlewares (JWT auth, logging, panic recovery) that you can opt out of per‑method.
* A relocatable **cloud toolkit** (Azure / GCP) for signed URLs, blob storage, secret resolution, etc.  
* **Zero-configuration HTTPS** – serve valid TLS certificates on day 0. 
* Built-In Dependency injection wiring customized for gRpc services. 

The result: you write business logic, not boilerplate.

---

## Why go‑api‑boot?

| Challenge                                               | Typical Effort          | **With go‑api‑boot**                    |
| ------------------------------------------------------- | ----------------------- | --------------------------------------- |
| Spin up gRPC+gRPC‑Web server, CORS, healthz, Prometheus | Days                    | `go-api-boot bootstrap …` – seconds     |
| MongoDB persistence with generics                | Days                    | One-liner via `CollectionOf[T]` |
| Secure service with JWT, skip for selected methods      | Manual interceptors     | Built‑in interceptors and `AuthFuncOverride`           |
| Signed S3 / Blob URLs                                   | SDK boilerplate         | One‑liner via `cloud.Cloud` interface   |
| Automatic HTTPS certificates with a shared cloud cache                            | External infrastructure | `server.WithSSL(true)` + `SslCloudCache` – seconds                  |

---

## Key Features

* **First‑class gRPC & gRPC‑Web** – serve browsers natively without Envoy.
* **Generic ODM for MongoDB** – type‑safe generic multi-tenant Object Model - (`CollectionOf[T](client, tenant).FindOneById(id)`) with async helpers.
* **JWT Auth & Middleware Stack** – observability, logging, panic recovery pre‑wired.
* **Cloud Providers** – interchangeable Azure / GCP helpers for storage & secrets.
* **Zero‑Config SSL** – automatic Let’s Encrypt certificates with exponential back‑off and optional cloud-backed cache (SslCloudCache) for stateless containers.
* **Built-in Dependency Injection** – no Google Wire or codegen, with lifecycle-aware gRPC registration.
* **Bootstrap CLI** – scaffold full service, models, repos, services, Dockerfile, build scripts.
* **Temporal Workflow Support** – run long-lived, fault-tolerant background jobs with native Temporal integration and DI-based worker registration.
---

## Quick Start

### Bootstrap a New Service

```bash
# Install the CLI once
$ go install github.com/SaiNageswarS/go-api-boot/cmd/go-api-boot@latest

# Scaffold a new service in ./quizService
$ go-api-boot bootstrap github.com/yourname/quizService proto
```

Generated layout ⤵️

```
quizService/
├── cmd/...
├── db/              # repositories
├── generated/       # proto stubs (via build script)
├── services/        # business logic
├── Dockerfile       # multistage build
└── config.ini       # config
```

### Running Locally

```bash
# Generate proto code & build binary
$ ./build.sh

# Export secrets (or use .env / Azure Key Vault)
$ export MONGO_URI=mongodb://localhost:27017
$ export ACCESS_SECRET=supersecret
$ export DOMAIN=api.example.com       # required for SSL
# (optional) use cloud cache for certs
$ export SSL_BUCKET=my-cert-bucket    # bucket / container name

# Start the server – gRPC :50051, HTTP :8081 (HTTPS if --ssl)
$ ./build/quizService
```

---

## Core Modules

### Server

```go
// main.go
package main

func main() {
    // Load secrets and config
    dotenv.LoadEnv()

    // load config file
    var ccfg *config.AppConfig 
    config.LoadConfig("config.ini", ccfg) // loads [dev] or [prod] section based on env var - `ENV=dev` or `ENV=prod`

    // Pick a cloud provider – all implement cloud.Cloud
    cloudFns := cloud.ProvideAzure(ccfg) // or cloud.ProvideGCP(cfg)
    // load secrets from Keyvault/SecretManader
    cloudFns.LoadSecretsIntoEnv(context.Background())

    boot, _ := server.New().
        GRPCPort(":50051").        // or ":0" for dynamic
        HTTPPort(":8080").
        EnableSSL(server.CloudCacheProvider(cfg, cloudFns)).
        // Dependency injection
        Provide(cfg).
        ProvideAs(cloudFns, (*cloud.Cloud)(nil)).
        // Register gRPC service impls
        RegisterService(server.Adapt(pb.RegisterLoginServer), ProvideLoginService).
        Build()

    ctx, cancel := context.WithCancel(context.Background())
    // catch SIGINT ‑> cancel
    _ = boot.Serve(ctx)
}
```

```
// AppConfig.go

package config

type AppConfig struct {
	BootConfig  `ini:",extends"`
	CustomField string `env:"CUSTOM-FIELD" ini:"custom_field"`
}

```

```ini
;; config.ini

[dev]
custom_field=3
azure_storage_account=testaccount

[prod]
custom_field=5
azure_storage_account=prodaccount
```

* gRPC, gRPC‑Web, and optional REST gateway on the same port.
* Middleware registry (unary + stream) to plug in OpenTelemetry, Prometheus, etc.

### ODM (MongoDB)

```go
// Model
type Profile struct {
    ID    string `bson:"_id"`
    Name  string `bson:"name"`
}
func (p Profile) Id() string { return p.ID }
func (p Profile) CollectionName() string { return "profile" }

// Query
client, err := odm.GetClient(ccfg)
profile, err := async.Await(odm.CollectionOf[Profile](client, tenant).FindOneById(context.Background(), id))
```

Async helpers return `<-chan T` + `<-chan error` for fan‑out concurrency.

### Auth & JWT

* HS256 by default – override via env vars or secrets manager.
* Skip auth per‑method:

```go
func (s *LoginService) AuthFuncOverride(ctx context.Context, method string) (context.Context, error) {
    return ctx, nil // public endpoint
}
```

### Cloud Abstractions

```go
var cloudFns cloud.Cloud = cloud.ProvideAzure(ccfg)
filePath, err := cloudFns.DownloadFile(context, bucket, key)
```

Switch provider with one line – signatures stay identical. Cloud access Secrets such as ClientId, TenantId, ClientSecret for Azure or ServiceAccount.json for GCP are loaded from environment variables.

Other configs like azure stoage account name, keyvault name or GCP projectId are loaded from the config file (ini) lazily as and when the resources are used.


### Zero‑Config SSL/TLS

There are two ways to persist the Let’s Encrypt certificates:

1. **Local** autocert.DirCache("certs") – good for single-node dev / on-prem.
2. **Distributed cache with** SslCloudCache – perfect for Docker / Kubernetes where the container filesystem is ephemeral.

```go
boot, _ := server.New().
    GRPCPort(":50051").HTTPPort(":8080").
    EnableSSL(server.DirCache("certs"))  // local cache
    Build()
```

* ACME challenge handled internally, exponential back‑off for cloud IP propagation.
* Just expose port 80 and 443 in your container spec.
* SslCloudCache streams certificates to the chosen object store (S3, Azure Blob, GCS).
* Multiple replicas of your service instantly share the same certs – no race conditions, no volume mounts.
* Exponential back-off is applied automatically while waiting for DNS / IP propagation.

### Temporal Workers

go-api-boot provides first-class support for running **Temporal workers** alongside your gRPC/HTTP services using the same dependency injection system. You can:

* Register **workflows and activities** via a simple hook.
* Automatically connect to Temporal server (local, Docker, or Temporal Cloud).
* Share configuration and lifecycle across the whole service.

```go
import (
    "github.com/SaiNageswarS/go-api-boot/server"
    "go.temporal.io/sdk/client"
    "go.temporal.io/sdk/worker"
)

boot, _ := server.New().
    GRPCPort(":50051").
    HTTPPort(":8080").
    WithTemporal("MY_TASK_QUEUE", &client.Options{
        HostPort: "temporal:7233", // or "localhost:7233" if running locally
    }).
    // ProvideIndexerActivities is a function whose dependencies will be injected
    RegisterTemporalActivity(ProvideIndexerActivities).  
    RegisterTemporalWorkflow(IndexPdfFileWorkflow).
    Build()

boot.Serve(context.Background())
```

### LLM Clients

go-api-boot offers lightweight clients to interact with LLM APIs like Anthropic Claude, enabling seamless integration of intelligent completions into your services without external SDKs or complexity.

```go
import "github.com/SaiNageswarS/go-api-boot/prompts"

client, err := prompts.ProvideAnthropicClient()
if err != nil {
    logger.Fatal("Failed getting LLM client:", zap.Error(err))
}

resp, err := client.GenerateInference(&prompts.AnthropicRequest{
    Model:     "claude-2",
    MaxTokens: 100,
    System:    "You are a helpful assistant.",
    Temperature: 0.7,
    Messages: []prompts.Message{
        {Role: "user", Content: "Summarize this document"},
    },
})
```

---

## CLI Reference

| Command                             | Description                          |
| ----------------------------------- | ------------------------------------ |
| `bootstrap <modulePath> <protoDir>` | Scaffold a new project               |
| `repository <ModelName>`            | Generate model + repository in `db/` |
| `service <ServiceName>`             | Generate skeleton gRPC service       |

Run with `-h` for full flags.

---

## Examples

* **Kotlang/authGo** – real‑world auth service built with go‑api‑boot → [https://github.com/Kotlang/authGo](https://github.com/Kotlang/authGo), [https://github.com/SaiNageswarS/agent-boot](https://github.com/SaiNageswarS/agent-boot)

---

## Contributing

PRs and issues are welcome!

1. Fork ➡️ hack ➡️ PR.
2. Run `make test lint` – zero lint errors.
3. Add unit / integration tests for new features.

---

## License

Apache‑2.0 – see [https://github.com/SaiNageswarS/go-api-boot/blob/master/LICENSE](LICENSE) for details.
