# go-api-boot - Production-Ready Go API Framework

[![codecov](https://codecov.io/gh/SaiNageswarS/go-api-boot/graph/badge.svg?token=J3NQ3PR0F9)](https://codecov.io/gh/SaiNageswarS/go-api-boot)
[![Go Report Card](https://goreportcard.com/badge/github.com/SaiNageswarS/go-api-boot)](https://goreportcard.com/report/github.com/SaiNageswarS/go-api-boot) 
[![Go Reference](https://pkg.go.dev/badge/github.com/SaiNageswarS/go-api-boot.svg)](https://pkg.go.dev/github.com/SaiNageswarS/go-api-boot)
[![GitHub release](https://img.shields.io/github/release/SaiNageswarS/go-api-boot.svg)](https://github.com/SaiNageswarS/go-api-boot/releases)
[![Go version](https://img.shields.io/github/go-mod/go-version/SaiNageswarS/go-api-boot)](https://github.com/SaiNageswarS/go-api-boot)
[![License](https://img.shields.io/github/license/SaiNageswarS/go-api-boot.svg)](https://github.com/SaiNageswarS/go-api-boot/blob/master/LICENSE)

> **Batteries‑included Go framework for building production‑grade gRPC + HTTP APIs – with generics, MongoDB ODM, cloud utilities, zero‑config HTTPS, Temporal workers, and a one‑line bootstrap CLI.**

> 🚀 **Featured in [Awesome Go](https://github.com/avelino/awesome-go)** — the curated list of high-quality Go frameworks.

**🎯 Keywords**: Go API framework, gRPC Go, HTTP server Go, MongoDB ODM Go, Go microservices, Go web framework, production Go APIs, Go cloud native, Temporal Go workers, Go dependency injection

---

## 📑 Table of Contents

1. [Overview](#overview)
2. [Key Features](#key-features)
3. [Quick Start](#quick-start)
4. [Project Structure](#project-structure)
5. [Core Modules](#core-modules)

   * [Server](#server)
   * [REST Controllers](#rest-controllers)
   * [ODM (MongoDB)](#odm-mongodb)

        * [Generic CRUD](#generic-crud)
        * [Creating & Ensuring Indexes](#creating--ensuring-indexes)
        * [Vector Search](#vector-search)
        * [Text Search](#text-search)
   * [Auth & JWT](#auth--jwt)
   * [Cloud Abstractions](#cloud-abstractions)
   * [Zero‑Config SSL/TLS](#zero-config-ssltls)
   * [MCP Tools](#mcp-tools)
   * [Temporal Workers](#temporal-workers)
6. [CLI Reference](#cli-reference)
7. [Examples](#examples)
8. [Contributing](#contributing)
9. [License](#license)

---

## Overview

**go‑api‑boot** eliminates the repetitive plumbing required to ship modern API services in Go.  With a single CLI command you get:

* A fully wired **gRPC** server that also serves **gRPC‑Web** and **REST** gateways – no Envoy sidecars or extra proxies.
* **Temporal workflow support** for long-running background jobs.
* **Generic ODM** for MongoDB with multi‑tenant support.
* Opinionated middlewares (JWT auth, logging, panic recovery) that you can opt out of per‑method.
* A relocatable **cloud toolkit** (Azure / GCP) for signed URLs, blob storage, secret resolution, etc.  
* **Zero-configuration HTTPS** – serve valid TLS certificates on day 0. 
* Built-In Dependency injection wiring customized for gRpc services, temporal workers, config, mongo client, and cloud abstractions. 

The result: you write business logic, not boilerplate.

---

## Key Features

* **First‑class gRPC & gRPC‑Web** – serve browsers natively without Envoy.
* **Generic ODM for MongoDB** – type‑safe generic multi-tenant Object Model - (`CollectionOf[T](client, tenant).FindOneById(id)`) with async helpers.
* **Vector & Text Search** – built‑in support for Atlas Vector Search and Atlas Search.
* **Index Management** – auto‑create and ensure classic, search, and vector indexes via `EnsureIndexes[T]`.
* **JWT Auth & Middleware Stack** – observability, logging, panic recovery pre‑wired.
* **Cloud Providers** – interchangeable Azure / GCP helpers for storage & secrets.
* **Zero‑Config SSL** – automatic Let’s Encrypt certificates with exponential back‑off and optional cloud-backed cache (SslCloudCache) for stateless containers.
* **MCP Tool Server** – expose [Model Context Protocol](https://modelcontextprotocol.io) tools alongside gRPC/REST on the same HTTP port, with full DI support.
* **Temporal Workflow Support** – run long-lived, fault-tolerant background jobs with native Temporal integration and DI-based worker registration.
* **Fluent Dependency Injection** – chainable, lifecycle-aware registration for gRPC services, Temporal workflows/activities, SSL providers, cloud abstractions, and more, all via a single builder API.
* **Bootstrap CLI** – scaffold full service, models, repos, services, Dockerfile, build scripts.
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

    mongo, _ := odm.GetClient()
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
        Provide(mongo).
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

#### REST Controllers

For pure REST APIs (without gRPC), use the `RestController` interface with full dependency injection support:

```go
// Define a REST controller
type UserController struct {
    repo *UserRepository
}

// Implement the RestController interface
func (c *UserController) Routes() []server.Route {
    return []server.Route{
        {Pattern: "/users", Method: "GET", Handler: c.listUsers},
        {Pattern: "/users", Method: "POST", Handler: c.createUser},
        {Pattern: "/users/{id}", Method: "GET", Handler: c.getUser},
    }
}

func (c *UserController) listUsers(w http.ResponseWriter, r *http.Request) {
    // handler implementation
}

func (c *UserController) createUser(w http.ResponseWriter, r *http.Request) {
    // handler implementation
}

func (c *UserController) getUser(w http.ResponseWriter, r *http.Request) {
    // handler implementation
}

// Factory function for DI
func ProvideUserController(repo *UserRepository) *UserController {
    return &UserController{repo: repo}
}
```

Register with the builder:

```go
boot, _ := server.New().
    GRPCPort(":50051").
    HTTPPort(":8080").
    Provide(userRepo).
    AddRestController(ProvideUserController).  // DI resolves dependencies
    Build()
```

**Key features:**
* **Dependency Injection** – Controller factories receive dependencies automatically.
* **Method Filtering** – Specify HTTP method per route; empty string allows all methods.
* **CORS Support** – All REST routes are automatically wrapped with the configured CORS handler.
* **Multiple Controllers** – Register as many controllers as needed; each gets its own DI resolution.

### ODM (MongoDB)

#### Generic CRUD

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
* Additionally use helpers like `HashedKey` to generate _id, `NewModelFrom[T any](proto interface{})` to copy values from proto to the model.
---

#### Creating & Ensuring Indexes

Use `EnsureIndexes[T]` at startup or in integration tests to:

1. **Create the collection** if missing.
2. **Apply classic MongoDB indexes** (B-tree, compound).
3. **Apply Atlas Search** (text) and **Atlas Vector Search** indexes.

```go
// In your setup code
if err := odm.EnsureIndexes[Profile](ctx, mongoClient, tenant); err != nil {
  log.Fatalf("failed to ensure indexes: %v", err)
}
```

This idempotent helper safely creates all indexes advertised by your models:

* Implement `Indexed` for classic indexes.
* Implement `SearchIndexed` for text search (TermSearchIndexSpec).
* Implement `VectorIndexed` for vector search (VectorIndexSpec).
* Refer [github.com/SaiNageswarS/go-api-boot/odm/odm_atlas_test.go](https://github.com/SaiNageswarS/go-api-boot/blob/master/odm/odm_atlas_test.go) for examples.

---

#### Vector Search

Built‑in support for **Atlas Vector Search**. Define vector index specs on your model:

```go
type Article struct {
    ID        string      `bson:"_id"`
    Title     string      `bson:"title"`
    Content   string      `bson:"content"`
    Embedding bson.Vector `bson:"embedding"` // 768-dim vector
}

func (a Article) Id() string { return a.ID }
func (a Article) CollectionName() string { return "articles" }

// Specify vector index on field "embedding"
// odm.EnsureIndexes would create this index automatically.
func (m Article) VectorIndexSpecs() []odm.VectorIndexSpec {
  return []odm.VectorIndexSpec{{
    Name: "contentVecIdx", Path: "embedding", Type: "vector", NumDimensions: 768,
    Similarity: "cosine",
  }}
}
```

Perform vector search:

```go
embedding := getEmbedding(...) // []float32
params := odm.VectorSearchParams{
  IndexName:     "contentVecIdx",
  Path:          "embedding",
  K:             5,
  NumCandidates: 20,
}
results, _ := async.Await(repo.VectorSearch(ctx, embedding, params))
for _, hit := range results {
  fmt.Println(hit.Doc, hit.Score)
}
```

---

#### Text Search

Leverage **Atlas Search** for full‑text queries. Register term search specs:

```go
type Article struct {
    ID        string      `bson:"_id"`
    Title     string      `bson:"title"`
    Content   string      `bson:"content"`
    Embedding bson.Vector `bson:"embedding"` // 768-dim vector
}

func (a Article) Id() string { return a.ID }
func (a Article) CollectionName() string { return "articles" }

// Specify term index on field "content" and "title".
// This allows full‑text search across "content" and "title" fields.
// odm.EnsureIndexes would create this index automatically.
func (m Article) TermSearchIndexSpecs() []odm.TermSearchIndexSpec {
  return []odm.TermSearchIndexSpec{{
    Name: "contentTextIdx", Paths: []string {"content", "title"},
  }}
}
```

Execute text search:

```go
params := odm.TermSearchParams{
  IndexName: "contentTextIdx",
  Path:      []string {"content", "title"},
  Limit:     10,
}
results, _ := async.Await(repo.TermSearch(ctx, "golang guides", params))
for _, hit := range results {
  fmt.Println(hit.Doc, hit.Score)
}
```

---

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

### MCP Tools

go-api-boot supports exposing [Model Context Protocol](https://modelcontextprotocol.io) (MCP) tools on the same HTTP port, served via the **Streamable HTTP** transport at `/mcp` (configurable). This lets AI agents and LLM clients discover and call your tools over a standard protocol.

```go
import (
    "context"
    "github.com/SaiNageswarS/go-api-boot/server"
    "github.com/modelcontextprotocol/go-sdk/mcp"
)

type GreetInput struct {
    Name string `json:"name" jsonschema:"the name to greet"`
}
type GreetOutput struct {
    Greeting string `json:"greeting"`
}

func SayHi(_ context.Context, _ *mcp.CallToolRequest, in GreetInput) (
    *mcp.CallToolResult, GreetOutput, error,
) {
    return nil, GreetOutput{Greeting: "Hi " + in.Name}, nil
}

func main() {
    boot, _ := server.New().
        GRPCPort(":50051").
        HTTPPort(":8080").
        WithMCP(
            &mcp.Implementation{Name: "my-service", Version: "v1.0.0"},
            nil,
        ).
        ConfigureMCP(func(s *mcp.Server) {
            mcp.AddTool(s, &mcp.Tool{
                Name:        "greet",
                Description: "say hello",
            }, SayHi)
        }).
        Build()

    boot.Serve(context.Background())
}
```

**With Dependency Injection** – implement the `MCPConfigurator` interface and register via `AddMCPConfigurator`:

```go
type MyMCPTools struct {
    repo *UserRepository
}

func (m *MyMCPTools) ConfigureMCP(s *mcp.Server) {
    mcp.AddTool(s, &mcp.Tool{Name: "getUser", Description: "fetch user"}, m.getUser)
}

// Factory for DI
func ProvideMyMCPTools(repo *UserRepository) *MyMCPTools {
    return &MyMCPTools{repo: repo}
}

// Register
boot, _ := server.New().
    GRPCPort(":50051").HTTPPort(":8080").
    Provide(userRepo).
    WithMCP(&mcp.Implementation{Name: "my-service", Version: "v1.0.0"}, nil).
    AddMCPConfigurator(ProvideMyMCPTools).
    Build()
```

**Key features:**
* Tools, resources, and prompts via the official [go-sdk](https://github.com/modelcontextprotocol/go-sdk).
* Served on the same HTTP port – no extra listener.
* Full dependency injection for MCP configurators.
* Configurable path (`MCPPath("/custom")`) and HTTP options (`MCPHTTPOptions(...)`).

---

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

* **Medicine RAG** - A retrieval-augmented generation full-stack application for doctors -> [https://github.com/SaiNageswarS/medicine-rag](https://github.com/SaiNageswarS/medicine-rag)
* **Agent Boot** – AI agent framework → [github.com/SaiNageswarS/agent-boot](https://github.com/SaiNageswarS/agent-boot)
* **Kotlang/authGo** – real‑world auth service → [github.com/Kotlang/authGo](https://github.com/Kotlang/authGo)

---

## Contributing

PRs and issues are welcome!

1. Fork ➡️ hack ➡️ PR.
2. Run `make test lint` – zero lint errors.
3. Add unit / integration tests for new features.

---

## License

Apache‑2.0 – see [LICENSE](LICENSE) for details.

---

**Built with ❤️ for developers by [Sai Nageswar S](https://prism.apiboot.com/sainageswar/).**
