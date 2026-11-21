# go-api-boot - Production-Ready Go API Framework

[![codecov](https://codecov.io/gh/SaiNageswarS/go-api-boot/graph/badge.svg?token=J3NQ3PR0F9)](https://codecov.io/gh/SaiNageswarS/go-api-boot)
[![Go Report Card](https://goreportcard.com/badge/github.com/SaiNageswarS/go-api-boot)](https://goreportcard.com/report/github.com/SaiNageswarS/go-api-boot) 
[![Go Reference](https://pkg.go.dev/badge/github.com/SaiNageswarS/go-api-boot.svg)](https://pkg.go.dev/github.com/SaiNageswarS/go-api-boot)
[![GitHub release](https://img.shields.io/github/release/SaiNageswarS/go-api-boot.svg)](https://github.com/SaiNageswarS/go-api-boot/releases)
[![Go version](https://img.shields.io/github/go-mod/go-version/SaiNageswarS/go-api-boot)](https://github.com/SaiNageswarS/go-api-boot)
[![License](https://img.shields.io/github/license/SaiNageswarS/go-api-boot.svg)](https://github.com/SaiNageswarS/go-api-boot/blob/master/LICENSE)

> **Batteries‑included Go framework for building production‑grade gRPC + HTTP APIs – with generics, a MongoDB & Postgres ODM, cloud utilities, zero‑config HTTPS, Temporal workers, and a one‑line bootstrap CLI.**

> 🚀 **Featured in [Awesome Go](https://github.com/avelino/awesome-go)** — the curated list of high-quality Go frameworks.

**🎯 Keywords**: Go API framework, gRPC Go, HTTP server Go, MongoDB ODM Go, Postgres ODM Go, Go microservices, Go web framework, production Go APIs, Go cloud native, Temporal Go workers, Go dependency injection

---

## 📑 Table of Contents

1. [Overview](#overview)
2. [Key Features](#key-features)
3. [Quick Start](#quick-start)
4. [Project Structure](#project-structure)
5. [Core Modules](#core-modules)

   * [Server](#server)
   * [REST Controllers](#rest-controllers)
   * [ODM (MongoDB & Postgres)](#odm-mongodb--postgres)

        * [Indexes](#indexes)
        * [MongoDB backend](#mongodb-backend)
        * [Postgres backend](#postgres-backend)
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
* **Generic ODM** for MongoDB and Postgres with multi‑tenant support.
* Opinionated middlewares (JWT auth, logging, panic recovery) that you can opt out of per‑method.
* A relocatable **cloud toolkit** (Azure / GCP) for signed URLs, blob storage, secret resolution, etc.  
* **Zero-configuration HTTPS** – serve valid TLS certificates on day 0. 
* Built-In Dependency injection wiring customized for gRpc services, temporal workers, config, mongo client, and cloud abstractions. 

The result: you write business logic, not boilerplate.

---

## Key Features

* **First‑class gRPC & gRPC‑Web** – serve browsers natively without Envoy.
* **MongoDB ODM and Postgres ORM** – type‑safe, multi-tenant and async, each native to its database: `mongodb.CollectionOf[T](client, tenant).Find(ctx, bson.M{...})` and `postgres.TableOf[T](db, tenant).Where("score > ?", 89).Find(ctx)`.
* **Vector & Text Search** – Atlas Vector Search and Atlas Search on MongoDB; pgvector and ParadeDB (BM25) on Postgres.
* **Index Management** – declare indexes on the model; `mongodb.EnsureCollection[T]` / `postgres.EnsureTable[T]` create (and on Postgres reconcile) them.
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

    mongo, _ := mongodb.ConnectFromEnv(context.Background()) // MONGO_URI
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

### ODM (MongoDB & Postgres)

Two multi-tenant data backends, each with an API native to its database: MongoDB collections queried with filter documents, and Postgres tables queried with SQL.

| Package | Purpose |
|---|---|
| `odm` | shared: `Model`, `Timestamps`, index specs, `SearchHit`, errors |
| `odm/mongodb` | MongoDB / Atlas: filter documents (`bson.M`), aggregation, Atlas Search, Atlas Vector Search |
| `odm/postgres` | Postgres: typed tables, SQL conditions, pgvector, ParadeDB BM25 search |

Every IO method is asynchronous and returns a channel with an `async.Result`, so independent queries run concurrently. Await one with `async.Await`, or several with `async.AwaitAll`.

Both backends share:

* **Models:** a struct with `Id()` and `CollectionName()` (the collection or table name).
* **Schema setup:** `mongodb.EnsureCollection[T]` and `postgres.EnsureTable[T]` create the collection or table and its declared indexes. Schema changes are a deployment step (startup, migration, tenant onboarding), never a side effect of runtime calls.
* **`Save`:** an upsert by `Id()` that always stamps creation and update times, in Unix seconds (set the clock with `WithTimer`). Embed `odm.Timestamps` to read them; add `bson:",inline"` on MongoDB.
* **Errors:** read `odm/<backend>: <operation> <tenant>.<name>: <cause>` and wrap the cause, so `errors.Is` matches `odm.ErrNotFound`, `odm.ErrDuplicateKey` (unique index), `errors.ErrUnsupported` (index features a backend lacks) and driver errors.
* **Helpers:** `HashedKey` derives ids from data fields; `NewModelFrom[T]` copies a proto into a model.

#### Indexes

```go
func (m Movie) Indexes() []odm.IndexSpec {
    return []odm.IndexSpec{
        {Keys: []odm.IndexKey{odm.Key("title")}, Unique: true},
        {Keys: []odm.IndexKey{odm.DescKey("year"), odm.Key("rating")}},
    }
}
func (m Movie) TermSearchIndexes() []odm.TermSearchIndexSpec {
    return []odm.TermSearchIndexSpec{{Name: "plotIndex", Paths: []string{"title", "plot"}}}
}
func (m Movie) VectorIndexes() []odm.VectorIndexSpec {
    return []odm.VectorIndexSpec{{Name: "plotEmbedding", Path: "embedding", Dimensions: 768, Similarity: odm.Cosine}}
}
```

Keys and paths name fields on MongoDB and columns on Postgres. Search scores are comparable across backends: cosine and dotProduct give `(1 + similarity) / 2`, euclidean `1 / (1 + distance)`; text search scores are BM25.

#### MongoDB backend

```go
type Movie struct {
    odm.Timestamps `bson:",inline"` // createdOn, updatedOn
    ID     string   `bson:"_id"`
    Title  string   `bson:"title"`
    Year   int      `bson:"year"`
    Genres []string `bson:"genres"`
}
func (m Movie) Id() string             { return m.ID }
func (m Movie) CollectionName() string { return "movies" }

client, err := mongodb.Connect(ctx, os.Getenv("MONGO_URI"))
err = mongodb.EnsureCollection[Movie](ctx, client, tenant)
movies := mongodb.CollectionOf[Movie](client, tenant)

_, err = async.Await(movies.Save(ctx, movie))
m, err := async.Await(movies.FindByID(ctx, id))
recent, err := async.Await(movies.Find(ctx,
    bson.M{"year": bson.M{"$gte": 2020}, "genres": "drama"},
    mongodb.WithSort(mongodb.Desc("year")), mongodb.WithLimit(10)))
genres, err := async.Await(mongodb.DistinctAs[string](ctx, movies, "genres", nil))

hits, err := async.Await(movies.VectorSearch(ctx, embedding, mongodb.VectorSearchParams{
    IndexName: "plotEmbedding", K: 5, Filter: bson.M{"year": bson.M{"$gte": 2000}},
}))
hits, err = async.Await(movies.TermSearch(ctx, "space crew", mongodb.TermSearchParams{IndexName: "plotIndex", Limit: 10}))
```

* **Tenants:** a tenant is a database.
* **Save:** top-level fields replace the stored ones, and fields absent from the model are kept. It stamps `createdOn` on insert and `updatedOn` on every save, atomically, whether or not the model has those fields.
* **Raw access:** `Aggregate(ctx, mongo.Pipeline)` runs a pipeline, and `Raw()` returns the driver collection.
* **Search:** needs Atlas, or the local Atlas image (`mongodb/mongodb-atlas-local`). Atlas builds search indexes asynchronously; `mongodb.WaitForSearchIndexes[T]` waits until they serve queries. `VectorIndexSpec.FilterPaths` declares the fields a vector search may pre-filter on.
* **Index changes:** `EnsureCollection` recreates regular indexes whose definition changed, and creates missing search indexes. To change a search index, give it a new name.

#### Postgres backend

```go
type Profile struct {
    ID        string    `db:"id"`
    Name      string    `db:"name"`
    City      string    `db:"city"`
    Score     float64   `db:"score"`
    Tags      []string  `db:"tags"`      // text[]
    Address   Address   `db:"address"`   // jsonb
    Embedding []float32 `db:"embedding"` // vector(N), from its VectorIndexSpec
    odm.Timestamps                        // created_on, updated_on
}
func (p Profile) Id() string             { return p.ID }
func (p Profile) CollectionName() string { return "profiles" }

db, err := postgres.Open(ctx, os.Getenv("POSTGRES_URI"))
err = postgres.EnsureTable[Profile](ctx, db, tenant)
profiles := postgres.TableOf[Profile](db, tenant)

top, err := async.Await(profiles.
    Where("score > ?", 89).
    Where("city = ?", city).
    OrderBy("score DESC").
    Limit(10).
    Find(ctx))

_, err = async.Await(profiles.Save(ctx, profile))
p, err := async.Await(profiles.FindByID(ctx, id))
n, err := async.Await(profiles.Where("? = ANY(tags)", "vip").Count(ctx))
hits, err := async.Await(profiles.Where("city = ?", city).VectorSearch(ctx, "byEmbedding", embedding, 5))
```

* **Tables:** a tenant is a schema, and a model is a table whose columns are its fields. A column is named by the field's `db` tag, or else the field name in snake_case. Fields of embedded structs are flattened in.
* **Column types:** Go types map to SQL types: strings, numbers, booleans and `time.Time` to scalar columns, slices of them to arrays, `[]float32` with a vector index to `vector(N)`, and anything else to `jsonb` (via `encoding/json`). Non-pointer columns are `NOT NULL` with the type's zero value as default.
* **Queries:**
  * `Where`, `OrderBy`, `Limit` and `Offset` build an immutable query, so a base query can be shared and extended. `Find`, `First`, `Count`, `Exists`, `Delete`, `VectorSearch` and `TermSearch` run it.
  * Conditions are SQL with `?` placeholders bound to arguments; write `??` for a literal `?` (the jsonb key operator). Pass a slice to match a list: `Where("city = ANY(?)", cities)`.
  * The SQL in `Where` and `OrderBy` runs as written, so it must come from your code; pass user values as arguments.
* **Save:** one `INSERT … ON CONFLICT (id) DO UPDATE` statement: it replaces the row with the same `Id()`, sets `created_on` on insert and `updated_on` on every save.
* **Timestamps:** every table has bigint `created_on` and `updated_on` columns, even when the model doesn't map them, so `Where("updated_on > ?", t)` always works.
* **EnsureTable:**
  * Creates the table, and adds the columns of new fields.
  * Refuses to change an existing column's type, since that needs a data migration.
  * Reconciles indexes: their names carry a hash of the definition, so a changed definition replaces the old index, and indexes no longer declared are dropped (only ones it created).
* **Missing tables:** reads of a table that doesn't exist find nothing; writes to it fail with a hint to run `EnsureTable`.
* **Vector search:** uses [pgvector](https://github.com/pgvector/pgvector) HNSW (`halfvec` for scalar quantization or more than 2000 dimensions). Rows without an embedding aren't searched. With `Where` conditions, it scans the index iteratively (pgvector 0.8+) so it still returns k rows.
* **Text search:** needs [ParadeDB](https://www.paradedb.com/docs)'s `pg_search`. Run the `paradedb/paradedb` image (it includes pgvector), or install `pg_search` with `shared_preload_libraries`. Without it, `EnsureTable` and `TermSearch` return `postgres.ErrPgSearchNotInstalled`. One ParadeDB index per table covers every searched text column, and words are stemmed in English by default (`postgres.WithStemmer`).
* **Raw access:** `db.Pool()` returns the `*pgxpool.Pool`, for extensions, migrations and SQL the ORM doesn't cover.
* **Unsupported:** `TTL` indexes.

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
