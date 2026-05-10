---
name: ms-generator
description: Generate a consistent Go microservice baseline from DECISIONS.md and Requirements.md. Use when a developer asks to scaffold a new service repo.
---

Goal:
Generate a production-oriented baseline Go microservice that strictly follows [DECISIONS.md](DECISIONS.md) and satisfies [Requirements.md](Requirements.md).

Required input:
- service_name
Optional input:
- module_path
- output_directory
- verify (default true)

Workflow:
1. If service_name is missing, ask for it.
2. Read [DECISIONS.md](DECISIONS.md) first, then [Requirements.md](Requirements.md).
3. Generate the full baseline repository with:
- Go module bootstrap
- cmd/service entrypoint
- internal packages for config, handlers, service, repository, middleware, errors
- db/migrations with SQL up and down files
- users sample vertical slice (model, repository, GET/POST/DELETE handlers)
- health endpoints (liveness/readiness with DB check)
- docs/openapi.yaml for users API
- Dockerfile multi-stage with scratch runtime
- docker-compose for local PostgreSQL
- Makefile targets: run, test, test-component, lint, fmt, build, docker-build, db-up, db-down, help
- CI pipeline with lint, test, build stages
- component test scaffolding with godog + testcontainers
4. Enforce architecture constraints from decisions:
- REST HTTP with chi
- PostgreSQL + golang-migrate + GORM
- standard error envelope and domain error mapping
- slog structured logging and OpenTelemetry hooks
- YAML config with env overrides and file-based secrets pattern
- no gRPC, no event broker patterns
5. Run verification when verify is true:
- go fmt or equivalent formatting
- lint checks
- unit tests
- component tests
- build success
- OpenAPI file presence and schema validation
6. Return:
- output path
- generated tree summary
- verification results
- missing or deferred items if any

Hard constraints:
- Treat [DECISIONS.md](DECISIONS.md) as source of truth for implementation choices.
- Treat [Requirements.md](Requirements.md) as source of truth for acceptance checks.
- Do not introduce alternate stacks unless explicitly requested by the user.
- Do not modify decision or requirements documents during generation.

Post-generation audit:
- Compare generated output against decisions and requirements.
- List any drift with exact file-level fixes.
- Apply fixes and report final compliance status.

Example invocation:
- "Use the ms-generator skill to scaffold service_name=payments-api"
- "Use the ms-generator skill with service_name=orders-api module_path=github.com/acme/orders-api output_directory=../orders-api verify=true"
