# Go Microservice Template Requirements

## 1. Purpose

This document defines the product and technical requirements for a Go-based microservice template intended for internal team use. The template will provide a consistent starting point for building REST/HTTP services with operational defaults for containerization, CI/CD, observability, security, and database integration.

The template is intended to reduce setup time, establish shared engineering conventions, and improve service readiness for local development and deployment environments.

## 2. Audience

The primary audience for this document is engineers who will implement, maintain, and consume the microservice template.

## 3. Goals

The template must:

- Provide a clean starting point for a production-oriented Go microservice.
- Standardize the baseline structure and development workflow across the team.
- Support REST/HTTP APIs as the default service interface.
- Include operational hooks needed for packaging, deployment, and troubleshooting.
- Make common service concerns explicit rather than leaving them to ad hoc implementation.

## 4. Scope

### 4.1 In Scope

- A single-service Go template for REST/HTTP microservices with PostgreSQL persistence.
- Local development workflow and project bootstrap.
- Containerization support for building and running the service.
- CI/CD requirements for validation and delivery automation.
- Observability requirements covering logs, metrics, and tracing.
- Security requirements for configuration, secrets, and runtime hardening.
- Database integration requirements at the template level using PostgreSQL, SQL migrations, and GORM.
- API contract documentation via OpenAPI specification.
- Component-level testing using Gherkin scenarios and real PostgreSQL.

### 4.2 Out of Scope

- gRPC as a required interface.
- Event-driven or broker-specific messaging support.
- Multi-service orchestration or distributed system topology patterns.
- Organization-wide platform governance beyond the template itself.
- Public or open-source packaging obligations.
- Real integration tests with external services; component tests use mocked external dependencies.
- Deployment pipelines; CI produces artifacts while CD remains team-specific.

## 5. Assumptions

- The repository is greenfield and does not need to preserve existing conventions.
- The template will be used by an internal engineering team.
- CI/CD pipelines should be provider-neutral in structure while providing a concrete example implementation.
- PostgreSQL is the standard database for the template; services requiring alternative databases should adapt this template as a baseline.
- A service mesh or network policies will enforce inter-service security; the template does not implement application-level mTLS.

## 6. Functional Requirements

### 6.1 Service Bootstrap

- The template must provide a runnable service entry point.
- The template must support startup through a standard Go command.
- The template must include a clear application bootstrap path for configuration loading, dependency initialization (including database connections), server startup, and graceful shutdown.

### 6.2 HTTP Service Structure

- The template must support REST/HTTP as the default interface style.
- The template must use chi router for request routing and middleware composition.
- The template must define a routing layer that can host versioned API endpoints (e.g., `/v1/users`).
- The template must provide baseline middleware support for request logging, panic recovery, request ID injection, trace context propagation, and request-scoped context.
- The template must separate transport concerns from business logic.

### 6.3 Health and Lifecycle Endpoints

- The template must expose a liveness endpoint.
- The template must expose a readiness endpoint that verifies database connectivity.
- The template must support graceful shutdown for in-flight requests and resource cleanup including database connections.

### 6.4 Configuration

- The template must support configuration through YAML files with environment variable overrides.
- The template must define a consistent pattern for application settings such as ports, environment name, log level, and database connection settings.
- The template must support secrets injection via environment variables and `_FILE` suffix patterns for mounted secret files.
- The template must fail clearly when required configuration is missing or invalid.

### 6.5 API Baseline

- The template must provide a baseline structure for versioned API paths.
- The template must define a consistent error response approach for HTTP APIs in a standard envelope format.
- The template must support request validation and explicit status code handling.
- The template must include an OpenAPI 3.0 specification documenting the API contract.

### 6.6 Database Integration

- The template must require PostgreSQL as the database for the service.
- The template must define an integration point for persistence access using GORM.
- The template must include SQL migration support via golang-migrate with migrations stored in `db/migrations/`.
- The template must isolate database access behind a repository pattern so business logic is not tightly coupled to storage details.
- The template must support configuration of database connection settings without embedding secrets in source control, using environment variables (e.g., `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_USER`) and secret files (e.g., `DB_PASSWORD_FILE`).
- The template must apply migrations during service startup.
- The template must include a minimal end-to-end sample resource (e.g., users) demonstrating migration, model, repository, and handler patterns.

## 7. Non-Functional and Operational Requirements

### 7.1 Containerization

- The template must support building a container image for the service.
- The containerization approach must use multi-stage Dockerfile with a scratch final image for minimal runtime footprint.
- The containerization approach must support reproducible builds.
- The runtime image must avoid unnecessary tooling and assets.
- The container execution model must support configuration through environment variables.

### 7.2 CI/CD

- The template must define the checks that must run in continuous integration.
- Continuous integration must include formatting, linting, tests (unit and component), and build verification.
- The template must support an automated path from source change to deployable artifact.
- CI/CD requirements must be expressed through a concrete example implementation while remaining portable to other platforms.
- The template includes a GitLab CI pipeline as the reference implementation; teams may adapt to GitHub Actions, Jenkins, or other systems.

### 7.3 Observability

- The template must emit structured application logs using `slog` with JSON output.
- The template must provide OpenTelemetry integration points for service-level traces and metrics.
- The template must provide configurable exporters for traces and metrics to backends (e.g., Jaeger, Prometheus, Datadog).
- The template must support correlation between logs and request context via trace IDs and request IDs.
- Every log entry must include standardized fields: `trace_id`, `request_id`, and `timestamp`.
- Middleware must extract trace context from incoming requests and propagate it to all operations.

### 7.4 Security and Secrets

- The template must avoid hard-coded credentials and secrets.
- The template must define a secure configuration pattern for secret values via environment variable injection and file-based secrets.
- The template must support least-privilege assumptions for runtime access.
- The template must define a baseline for dependency and image scanning in delivery workflows.
- The template must support safe default behavior for error handling so internal implementation details are not exposed in error responses.
- The template must run on plain HTTP in development; TLS is optional and configured via environment variables (`TLS_ENABLED`, `TLS_CERT_FILE`, `TLS_KEY_FILE`).
- The template assumes inter-service communication is secured by network policies or service meshes; application-level mTLS is not required.

### 7.5 Local Developer Workflow

- The template must support local execution with minimal setup steps.
- The template must document the commands required to run, test, and package the service.
- The template must provide a Makefile with documented targets for common tasks (run, test, test-component, lint, fmt, build, docker-build, db-up, db-down).
- The template must support a predictable onboarding path for a new engineer.
- The template must include Docker Compose configuration for running PostgreSQL in local development.

## 8. Engineering Quality Requirements

### 8.1 Project Structure

- The template must define a clear and maintainable Go project layout following standard conventions.
- The project layout must organize code into: `cmd/service/` for entry point, `internal/` for private packages, `db/migrations/` for schema, `features/` for component tests, and `docs/` for documentation.
- The project layout must make the service entry point, internal packages, configuration concerns, database layer, and tests easy to locate.
- The layout should prefer conventions that scale to moderate service complexity without unnecessary indirection.

### 8.2 Code Quality

- The template must support automated formatting with `go fmt`.
- The template must define a consistent approach for error handling using domain-specific error types that map to HTTP status codes.
- The template must make dependency boundaries clear enough to support maintainable unit testing.
- The template must include linting configuration references (not enforced, but provided for consistency).

### 8.3 Testing

- The template must support unit testing as a baseline requirement using standard Go testing with table-driven patterns.
- The template must define how HTTP handlers and core service logic can be tested.
- The template must support component-level tests that exercise real HTTP, real PostgreSQL (via testcontainers), and mocked external dependencies.
- Component tests must use Gherkin-style scenarios via godog with feature files in `features/` and step definitions in `features/steps/`.
- Test databases must be created, migrated, and cleaned up for each scenario.

### 8.4 Dependency Management

- The template must use standard Go module dependency management.
- The template must support repeatable dependency resolution in automated environments.
- The template must avoid unnecessary framework or library lock-in except where it materially improves the baseline (e.g., chi for routing, GORM for data access, slog for structured logging, OpenTelemetry for observability).

### 8.5 Documentation Deliverables

- The template must be accompanied by clear usage documentation including README, architecture diagrams, and a developer guide.
- The template documentation must explain project structure, startup commands, configuration expectations, testing workflow, and packaging workflow.
- The template documentation must describe required environment variables, optional capabilities, and out-of-scope areas.
- The template must include an OpenAPI specification documenting the sample resource and API contract.

## 9. Optional Capabilities

The following capabilities may be added later, but are not required for the initial version of the template:

- Provider-specific observability tooling integration beyond OpenTelemetry.
- Support for additional databases alongside PostgreSQL.
- gRPC transport alongside REST/HTTP.
- Authentication and authorization extensions beyond the baseline service skeleton.
- Event-driven or message broker patterns.

## 10. Acceptance Checks

The template will satisfy this requirements document when the following conditions are true:

- A developer can create a new service from the template and run it locally using documented commands.
- The generated service exposes health endpoints (liveness and readiness) and supports graceful shutdown.
- The generated service can be built into a container image using the documented workflow.
- The generated service includes PostgreSQL integration with migrations applied at startup.
- The generated service includes a defined configuration pattern with secret-safe handling of database settings.
- The generated service includes a minimal end-to-end sample resource demonstrating the full stack.
- The generated service includes clear extension points for logs, metrics, and tracing via OpenTelemetry.
- The generated service can pass automated formatting, linting, unit test, component test, and build checks in CI.
- The template documentation clearly identifies mandatory capabilities, optional capabilities, and out-of-scope areas.
- The template includes an OpenAPI specification documenting the sample resource.

## 11. Open Decisions/Future Decisions

The following decisions remain intentionally open for a later revision:

- Whether to add gRPC support alongside REST/HTTP.
- Whether to standardize on a specific OpenTelemetry exporter (e.g., Jaeger, Datadog, Honeycomb).
- Whether to add support for additional databases (e.g., MySQL, MongoDB) alongside PostgreSQL.
- Whether to add event-sourcing or CQRS patterns.
