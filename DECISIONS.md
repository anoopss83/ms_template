# Go Microservice Template — Architectural Decisions

This document details the specific technology choices, patterns, and design decisions for the Go microservice template, resolving the open questions from [Requirements.md](Requirements.md).

## Database & Persistence

### Decision: PostgreSQL as Default Database

**Rationale:** Standardizing on PostgreSQL removes abstraction overhead, enables concrete local development setup, and provides a shared reference point for configuration, migrations, and testing. Services that require a different database can use this template as a baseline and adapt it, but the default must be specific.

**Implementation:**
- PostgreSQL is the required database for the template.
- Local development uses PostgreSQL via Docker Compose.
- Connection configuration follows the pattern: `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_USER`, `DB_PASSWORD` (or `DB_PASSWORD_FILE` for mounted secrets).

### Decision: SQL-First Migrations via `golang-migrate`

**Rationale:** Migrations are in scope (per Requirements.md 6.6). SQL-first migrations keep schema changes explicit and reviewable, and they remain portable across different application stacks. `golang-migrate` is mature, lightweight, and widely adopted in Go teams.

**Implementation:**
- Migrations live in `db/migrations/` directory.
- Migration files follow the naming convention: `001_create_users_table.up.sql` and `001_create_users_table.down.sql`.
- Migrations are applied during service startup via the migration runner.
- The template includes a `make db-up` and `make db-down` commands for local development.

### Decision: GORM for Application Data Access

**Rationale:** Once PostgreSQL and SQL migrations are standardized, application-level data access must also be defined to prevent drift in transaction handling, query organization, and testing. GORM provides a well-adopted, productive abstraction while remaining compatible with SQL-first schema management.

**Implementation:**
- GORM is the ORM used by service business logic for database queries and transactions.
- Migrations remain the authoritative schema definition; GORM models follow the schema.
- The template includes a sample repository pattern showing how to isolate database access behind a clean boundary.

### Decision: Minimal End-to-End Sample Resource

**Rationale:** A template without a concrete vertical slice leaves too much ambiguity about repository structure, transaction handling, and handler-to-database flow. One minimal example provides a reference without turning the template into a sample application.

**Implementation:**
- The template includes a sample `users` resource with:
  - A migration creating a `users` table.
  - A GORM model and repository layer.
  - HTTP handlers for GET, POST, and DELETE operations.
  - Component tests exercising the full stack.
- Services should remove or extend this sample; it is not intended to remain in production services.

## HTTP & API

### Decision: `chi` Router over `net/http`

**Rationale:** Standard library `net/http` is a solid foundation, but `chi` adds clean middleware support and route grouping without introducing a heavy framework. For a template, this balances functionality and maintainability.

**Implementation:**
- The service uses `chi` for HTTP routing and middleware composition.
- Routes are organized by version: `/v1/users`, `/v1/...`.
- Middleware chain includes: request logging, panic recovery, request ID injection, trace context propagation, and structured logging.
- Handlers are defined as `http.HandlerFunc` wrapped by middleware.

### Decision: OpenAPI for API Contract

**Rationale:** With REST/HTTP as the default interface and a sample vertical slice, leaving the contract informal creates drift. OpenAPI provides a machine-readable specification for clients and documentation generators.

**Implementation:**
- The template includes an OpenAPI 3.0 specification in `docs/openapi.yaml`.
- The sample `users` resource is fully documented in the OpenAPI spec.
- The spec is validated as part of CI/CD to ensure it stays current.
- Tooling for generating client SDKs or documentation from the spec is optional but encouraged.

### Decision: Standardized Error Response Format

**Rationale:** Consistent error responses make client error handling predictable and reduce ambiguity in error propagation.

**Implementation:**
- Error responses follow a standard envelope:
  ```json
  {
    "status": 400,
    "error": {
      "code": "VALIDATION_ERROR",
      "message": "Invalid user input"
    }
  }
  ```
- Domain errors are defined as types that map to HTTP status codes via middleware.
- A recovery middleware catches panics and returns a `500` error in the standard format.
- The template includes an error package with common error types: `BadRequest`, `NotFound`, `Conflict`, `InternalError`.

## Testing

### Decision: Unit Tests + BDD Component Tests

**Rationale:** Integration tests with real external services add complexity and fragility to templates. Component tests that exercise real HTTP, real PostgreSQL (via testcontainers), and mocked external dependencies provide strong coverage while remaining fast and deterministic.

**Implementation:**
- Unit tests use standard Go testing with table-driven patterns.
- Component tests use `godog` for Gherkin-style scenarios.
- Component tests run against an in-process HTTP server with:
  - Real PostgreSQL instance spun up via `testcontainers-go`.
  - External API dependencies mocked (e.g., third-party payment gateways, analytics).
- Feature files live in `features/` and follow naming convention: `users.feature`.
- Step definitions live in `features/steps/` and are organized by domain.
- Test database is created, migrated, and cleaned up for each scenario.

### Example Test Command
```bash
make test           # Run unit tests
make test-component # Run component tests via godog
```

## Observability

### Decision: OpenTelemetry + `slog`

**Rationale:** OpenTelemetry is becoming the de facto standard for distributed tracing and metrics. `slog` (Go 1.21+) provides structured logging that is stdlib, forward-compatible, and plays well with OpenTelemetry context.

**Implementation:**
- The service initializes OpenTelemetry trace and metric providers at startup.
- Traces are exported to a configurable backend (e.g., Jaeger, Datadog, Honeycomb).
- Metrics are exported to a configurable backend (e.g., Prometheus, Datadog).
- Structured logging uses `slog` with a JSON handler for production environments.

### Decision: Standardized Logging Fields

**Rationale:** Consistent field names and trace linking make logs queryable and correlatable with traces.

**Implementation:**
- Every log includes:
  - `trace_id`: OpenTelemetry trace ID, injected via context.
  - `request_id`: Generated per HTTP request, injected via middleware.
  - `timestamp`: Automatically added by `slog`.
  - Additional contextual fields as needed (user ID, resource ID, etc.).
- Middleware extracts trace context from incoming requests and propagates it to all operations.
- All external API calls propagate trace headers for distributed tracing.

## Transport Security

### Decision: Plain HTTP in Development, TLS Required in Production

**Rationale:** Cloud-native applications typically offload TLS termination to infrastructure layers (ingress controllers, load balancers, service meshes). Requiring TLS at the application level during development adds friction without providing security benefit in local environments.

**Implementation:**
- Service runs on plain HTTP by default (`http://localhost:8080`).
- In production, TLS must be enabled via configuration (see below).
- Environment-specific configs distinguish dev (plain HTTP) from prod (TLS required).

### Decision: Service Mesh Handles Inter-Service mTLS

**Rationale:** Modern cloud-native deployments assume a service mesh (Istio, Linkerd, etc.) enforces mTLS between services. The template should not duplicate this capability at the application level.

**Implementation:**
- The template does NOT include application-level mTLS implementation.
- Services communicate internally via plain HTTP; the mesh intercepts and secures the traffic.
- If a service mesh is not present, services must be isolated by network policy.
- mTLS is not a concern for the template unless services communicate outside the mesh.

### Decision: Upstream TLS Termination by Default, Optional Application-Level TLS

**Rationale:** The cloud-native pattern is to terminate TLS at the ingress layer (API gateway, load balancer, ingress controller). This centralizes certificate management and simplifies services. However, for services that require direct TLS termination (e.g., non-HTTP protocols, special compliance requirements), the template can support it.

**Implementation:**
- **Default (recommended):** Service runs plain HTTP internally; ingress/load balancer handles HTTPS with clients.
- **Optional:** Service can be configured to handle its own TLS via environment variables:
  - `TLS_ENABLED=true`
  - `TLS_CERT_FILE=/etc/tls/server.crt`
  - `TLS_KEY_FILE=/etc/tls/server.key`
- If `TLS_ENABLED=true` and cert/key files are provided, the service binds to HTTPS instead of HTTP.
- Certificates are provided as mounted files (Kubernetes secrets mounted as volumes).

## Operations & Infrastructure

### Decision: GitLab CI Pipeline

**Rationale:** GitLab CI provides a concrete working example while remaining transparent and portable to other platforms. Checks are framed provider-neutrally so teams can adapt to GitHub Actions, Jenkins, etc. if needed.

**Implementation:**
- `.gitlab-ci.yml` defines stages: lint, test, build.
- **Lint stage:** runs `golangci-lint`, `go fmt`, and `go vet`.
- **Test stage:** runs unit tests and component tests.
- **Build stage:** builds the Docker image and (optionally) pushes to a registry.
- CI/CD runs on every push and merge request.
- Deployment is left to CD; the template does not prescribe deployment targets.

### Decision: Multi-Stage Dockerfile, `scratch` Final Image

**Rationale:** Lean runtime images reduce attack surface, improve deployment speed, and follow production best practices.

**Implementation:**
- Dockerfile uses two stages:
  - **Build stage:** Uses a Go build image, builds the binary, runs tests.
  - **Runtime stage:** Copies the binary to `scratch`, defines entrypoint.
- Final image is minimal, typically < 20MB for a Go service.
- Build caching is optimized to leverage Docker layer caching.

## Configuration & Secrets

### Decision: YAML Configuration with Environment Variable Overrides

**Rationale:** YAML provides readable, versionable configuration files. Environment variable overrides enable runtime customization for different deployment environments without duplicating configuration.

**Implementation:**
- Configuration is defined in `config/config.yaml` (or `config/{ENV}.yaml` for environment-specific overrides).
- At startup, the service loads the base config, then overlays environment variables.
- Environment variable naming follows the pattern: `SERVICE_SECTION_KEY` (e.g., `SERVICE_DATABASE_URL`).
- Secrets are never stored in config files; they are injected via:
  - Environment variables from secret management systems (e.g., Kubernetes secrets, Vault).
  - `_FILE` suffix for file-based secrets (e.g., `DB_PASSWORD_FILE=/run/secrets/db_password`).

## Developer Experience

### Decision: Standard Go Project Layout

**Rationale:** A clear, conventional structure reduces cognitive load and makes the template scalable to moderate complexity.

**Implementation:**
- `cmd/service/` — Service entry point.
- `internal/` — Private service packages (models, handlers, repositories, middleware).
- `internal/handler/` — HTTP handlers.
- `internal/repository/` — Data access layer.
- `internal/service/` — Business logic.
- `internal/config/` — Configuration loading.
- `db/migrations/` — SQL migrations.
- `features/` — Gherkin feature files for component tests.
- `docs/` — OpenAPI specification and architecture diagrams.
- `Makefile` — Documented developer commands.
- `.golangci.yml` — Linting configuration (left to service discretion, not enforced by template).

### Decision: `Makefile` for Common Tasks

**Rationale:** A Makefile serves as executable documentation and reduces friction for new engineers.

**Implementation:**
Documented targets include:
- `make run` — Run the service locally.
- `make test` — Run unit tests.
- `make test-component` — Run component tests.
- `make lint` — Run linting checks (for reference; teams configure independently).
- `make fmt` — Format code.
- `make build` — Build the binary.
- `make docker-build` — Build Docker image.
- `make db-up` — Start local PostgreSQL.
- `make db-down` — Stop local PostgreSQL.
- `make help` — Show all available targets.

### Decision: Comprehensive Documentation

**Rationale:** Clear documentation accelerates onboarding and reduces support burden.

**Implementation:**
- **README.md:** Overview, local setup, commands, quick walkthrough of adding a new endpoint.
- **Architecture Diagram:** Visual representation of component relationships (in `docs/`).
- **Developer Guide:** Detailed instructions on testing, debugging, and extending the template.

### Decision: Semantic Versioning for the Template

**Rationale:** Versioned templates allow teams to stay on a stable baseline while being able to upgrade intentionally.

**Implementation:**
- Template versions follow semantic versioning: `v1.0.0`, `v1.1.0`, etc.
- Releases are tagged in version control with release notes documenting changes.
- Breaking changes increment the major version; new features increment the minor version; bug fixes increment the patch version.

## Authentication & Authorization

### Decision: No Built-In Auth, but Extension Points Required

**Rationale:** Authentication models vary across services (JWT, OAuth, service-to-service tokens, etc.). The template should not mandate one model but should make adding auth straightforward.

**Implementation:**
- The template includes a `context.Context` middleware that extracts and propagates identity information.
- Handlers can access user identity via `context.Value(UserContextKey)`.
- The template includes a commented example of JWT middleware that services can enable or replace.
- Authorization is left to handlers; the template does not prescribe a policy engine.

## Excluded Capabilities

The following are explicitly out of scope for this template version:

- **gRPC or other transports:** REST/HTTP only.
- **Event-driven or broker patterns:** No message queues or event streaming.
- **Multi-service orchestration:** Single service template.
- **Provider-specific observability integration:** OpenTelemetry is abstracted; exporters are configurable.
- **Real integration tests:** Component tests with mocked external APIs only.
- **Deployment pipelines:** CI builds artifacts; CD is left to teams.

## Future Decisions

The following remain open for future template revisions:

- Whether to add gRPC support alongside REST/HTTP.
- Whether to standardize on a specific OpenTelemetry exporter (e.g., Jaeger).
- Whether to add event-sourcing or CQRS patterns.
- Whether to add support for additional databases (e.g., MySQL, MongoDB) alongside PostgreSQL.
