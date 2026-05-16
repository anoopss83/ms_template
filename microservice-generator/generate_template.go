package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

var requiredDecisions = map[string]string{
	"postgresql":    "Decision: PostgreSQL as Default Database",
	"migrations":    "Decision: SQL-First Migrations via `golang-migrate`",
	"gorm":          "Decision: GORM for Application Data Access",
	"router":        "Decision: `chi` Router over `net/http`",
	"openapi":       "Decision: OpenAPI for API Contract",
	"testing":       "Decision: Unit Tests + BDD Component Tests",
	"observability": "Decision: OpenTelemetry + `slog`",
	"gitlab":        "Decision: GitLab CI Pipeline",
	"docker":        "Decision: Multi-Stage Dockerfile, `scratch` Final Image",
	"config":        "Decision: YAML Configuration with Environment Variable Overrides",
	"makefile":      "Decision: `Makefile` for Common Tasks",
	"principles":    "Decision: Engineering Principles (DRY and SOLID)",
	"dry":           "Decision: DRY by Default in Shared Layers",
	"solid":         "Decision: SOLID-Oriented Service Boundaries",
}

type generatorConfig struct {
	serviceName string
	decisions   string
	outputDir   string
	force       bool
	verify      bool
	verifyLevel string
}

type verificationStep struct {
	name    string
	command []string
	env     []string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := parseFlags()
	scriptDir, err := executableDir()
	if err != nil {
		return err
	}
	workspaceDir := filepath.Dir(scriptDir)

	decisionsPath := cfg.decisions
	if decisionsPath == "" {
		decisionsPath = filepath.Join(workspaceDir, "DECISIONS.md")
	}
	if err := validateDecisions(decisionsPath); err != nil {
		return err
	}

	serviceName, displayName, err := normalizeServiceName(cfg.serviceName)
	if err != nil {
		return err
	}

	outputDir := cfg.outputDir
	if outputDir == "" {
		outputDir = filepath.Join(filepath.Dir(workspaceDir), serviceName)
	}
	if err := ensureWritableOutput(outputDir, cfg.force); err != nil {
		return err
	}

	files := buildFileMap(serviceName, displayName)
	for relativePath, content := range files {
		if err := writeFile(outputDir, relativePath, content); err != nil {
			return err
		}
	}

	tidyMessage, err := runGoModTidy(outputDir)
	if err != nil {
		return err
	}

	verificationSummary := "Verification skipped."
	if cfg.verify {
		verificationSummary, err = runVerification(outputDir, serviceName, cfg.verifyLevel)
		if err != nil {
			return err
		}
	}

	fmt.Printf("Generated microservice scaffold at: %s\n", outputDir)
	fmt.Println("Created files:")
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		fmt.Printf("- %s\n", path)
	}
	if tidyMessage != "" {
		fmt.Println(tidyMessage)
	}
	if verificationSummary != "" {
		fmt.Println(verificationSummary)
	}

	return nil
}

func parseFlags() generatorConfig {
	var cfg generatorConfig
	flag.StringVar(&cfg.serviceName, "service-name", "", "Microservice name, for example payments-api.")
	flag.StringVar(&cfg.decisions, "decisions", "", "Path to DECISIONS.md. Defaults to the workspace root beside this script.")
	flag.StringVar(&cfg.outputDir, "output-dir", "", "Optional explicit output directory. Defaults to a sibling of the current ms_template directory.")
	flag.BoolVar(&cfg.force, "force", false, "Overwrite an existing output directory.")
	flag.BoolVar(&cfg.verify, "verify", true, "Run post-generation verification commands.")
	flag.StringVar(&cfg.verifyLevel, "verify-level", "quick", "Verification depth: quick or full.")
	flag.Parse()

	if strings.TrimSpace(cfg.serviceName) == "" {
		fmt.Fprintln(os.Stderr, "error: --service-name is required")
		flag.Usage()
		os.Exit(2)
	}

	return cfg
}

func executableDir() (string, error) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("could not determine generator source path")
	}
	resolvedPath, err := filepath.EvalSymlinks(sourcePath)
	if err != nil {
		return "", err
	}
	return filepath.Dir(resolvedPath), nil
}

func validateDecisions(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("DECISIONS.md not found: %s", path)
		}
		return err
	}

	text := string(content)
	missing := make([]string, 0)
	for key, marker := range requiredDecisions {
		if !strings.Contains(text, marker) {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		if missingContainsEngineeringPrinciples(missing) {
			return fmt.Errorf("DECISIONS.md is missing required decisions: %s. Add the Engineering Principles section with DRY/SOLID markers before running the generator", strings.Join(missing, ", "))
		}
		return fmt.Errorf("DECISIONS.md is missing required decisions: %s", strings.Join(missing, ", "))
	}
	return nil
}

func missingContainsEngineeringPrinciples(missing []string) bool {
	for _, key := range missing {
		switch key {
		case "principles", "dry", "solid":
			return true
		}
	}
	return false
}

func normalizeServiceName(raw string) (string, string, error) {
	cleaned := strings.TrimSpace(strings.ToLower(raw))
	re := regexp.MustCompile(`[^a-z0-9_-]+`)
	cleaned = re.ReplaceAllString(cleaned, "-")
	re = regexp.MustCompile(`[-_]{2,}`)
	cleaned = strings.Trim(re.ReplaceAllString(cleaned, "-"), "-")
	if cleaned == "" {
		return "", "", errors.New("service name must contain letters or numbers")
	}
	parts := strings.Split(cleaned, "-")
	for i, part := range parts {
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return cleaned, strings.Join(parts, " "), nil
}

func ensureWritableOutput(outputDir string, force bool) error {
	info, err := os.Stat(outputDir)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("output path exists and is not a directory: %s", outputDir)
		}
		entries, readErr := os.ReadDir(outputDir)
		if readErr != nil {
			return readErr
		}
		if len(entries) > 0 && !force {
			return fmt.Errorf("output directory already exists and is not empty: %s. Use --force to overwrite", outputDir)
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.MkdirAll(outputDir, 0o755)
}

func writeFile(baseDir, relativePath, content string) error {
	path := filepath.Join(baseDir, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	trimmed := strings.TrimRight(content, "\n") + "\n"
	return os.WriteFile(path, []byte(trimmed), 0o644)
}

func runGoModTidy(outputDir string) (string, error) {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return "warning: generated project was created without dependency resolution because `go` is not available; run `go mod tidy` inside the generated service.", nil
	}

	cmd := exec.Command(goPath, "mod", "tidy")
	cmd.Dir = outputDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go mod tidy failed in %s: %w\n%s", outputDir, err, strings.TrimSpace(string(output)))
	}

	return "Resolved dependency versions with `go mod tidy` during generation.", nil
}

func runVerification(outputDir, serviceName, verifyLevel string) (string, error) {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return "warning: verification skipped because `go` is not available.", nil
	}

	steps, err := verificationSteps(goPath, serviceName, verifyLevel)
	if err != nil {
		return "", err
	}

	completed := make([]string, 0, len(steps))
	for _, step := range steps {
		cmd := exec.Command(step.command[0], step.command[1:]...)
		cmd.Dir = outputDir
		if len(step.env) > 0 {
			cmd.Env = append(os.Environ(), step.env...)
		}
		output, runErr := cmd.CombinedOutput()
		if runErr != nil {
			return "", fmt.Errorf("verification failed at step %q: %w\n%s", step.name, runErr, strings.TrimSpace(string(output)))
		}
		completed = append(completed, step.name)
	}

	if verifyLevel == "full" {
		if err := verifyArchitectureLayout(outputDir, serviceName); err != nil {
			return "", err
		}
		completed = append(completed, "architecture layout")
	}

	return fmt.Sprintf("Verification passed (%s): %s.", verifyLevel, strings.Join(completed, ", ")), nil
}

func verifyArchitectureLayout(outputDir, serviceName string) error {
	requiredFiles := []string{
		filepath.ToSlash(filepath.Join("cmd", serviceName, "main.go")),
		"internal/config/config.go",
		"internal/httpserver/server.go",
		"internal/users/users.go",
		"internal/repository/user_repository.go",
		"db/migrations/001_create_users_table.up.sql",
		"docs/openapi.yaml",
	}

	for _, relPath := range requiredFiles {
		absPath := filepath.Join(outputDir, relPath)
		if _, err := os.Stat(absPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("full verification failed: required layering file missing: %s", relPath)
			}
			return fmt.Errorf("full verification failed while checking %s: %w", relPath, err)
		}
	}

	usersPath := filepath.Join(outputDir, "internal", "users", "users.go")
	usersContent, err := os.ReadFile(usersPath)
	if err != nil {
		return fmt.Errorf("full verification failed while reading users layer: %w", err)
	}
	usersText := string(usersContent)
	if !strings.Contains(usersText, "type Repository interface") {
		return errors.New("full verification failed: users layer must depend on a repository interface")
	}
	if !strings.Contains(usersText, "type Service struct") {
		return errors.New("full verification failed: users layer must define a service boundary")
	}
	if !strings.Contains(usersText, "func NewHandler") {
		return errors.New("full verification failed: users layer must expose a handler constructor")
	}

	repositoryPath := filepath.Join(outputDir, "internal", "repository", "user_repository.go")
	repositoryContent, err := os.ReadFile(repositoryPath)
	if err != nil {
		return fmt.Errorf("full verification failed while reading repository layer: %w", err)
	}
	repositoryText := string(repositoryContent)
	if !strings.Contains(repositoryText, "package repository") {
		return errors.New("full verification failed: repository implementation must stay isolated in internal/repository")
	}
	if !strings.Contains(repositoryText, "type UserRepository struct") {
		return errors.New("full verification failed: repository layer must define a concrete repository implementation")
	}

	return nil
}

func verificationSteps(goPath, serviceName, verifyLevel string) ([]verificationStep, error) {
	steps := []verificationStep{
		{
			name:    "unit tests",
			command: []string{goPath, "test", "./..."},
		},
		{
			name:    "service build",
			command: []string{goPath, "build", "./cmd/" + serviceName},
		},
	}

	switch verifyLevel {
	case "quick":
		return steps, nil
	case "full":
		steps = append(steps, verificationStep{
			name:    "component tests",
			command: []string{goPath, "test", "./features/...", "-count=1"},
			env:     []string{"ENABLE_COMPONENT_TESTS=1"},
		})
		return steps, nil
	default:
		return nil, fmt.Errorf("unsupported verify level %q; use quick or full", verifyLevel)
	}
}

func renderTemplate(tpl string, values map[string]string) string {
	replacements := []string{
		"{{BT}}", "`",
	}
	for key, value := range values {
		replacements = append(replacements, "{{"+key+"}}", value)
	}
	return strings.NewReplacer(replacements...).Replace(strings.TrimSpace(tpl))
}

func buildFileMap(serviceName, displayName string) map[string]string {
	values := map[string]string{
		"MODULE":       serviceName,
		"SERVICE_NAME": serviceName,
		"DISPLAY_NAME": displayName,
	}

	return map[string]string{
		"go.mod":             renderTemplate(goModTemplate, values),
		".gitignore":         renderTemplate(gitignoreTemplate, values),
		"Makefile":           renderTemplate(makefileTemplate, values),
		"Dockerfile":         renderTemplate(dockerfileTemplate, values),
		"docker-compose.yml": renderTemplate(dockerComposeTemplate, values),
		".gitlab-ci.yml":     renderTemplate(gitlabCITemplate, values),
		"config/config.yaml": renderTemplate(configYAMLTemplate, values),
		filepath.ToSlash(filepath.Join("cmd", serviceName, "main.go")): renderTemplate(mainTemplate, values),
		"internal/config/config.go":                                    renderTemplate(configGoTemplate, values),
		"internal/observability/logger.go":                             renderTemplate(observabilityTemplate, values),
		"internal/httpserver/server.go":                                renderTemplate(httpServerTemplate, values),
		"internal/users/users.go":                                      renderTemplate(usersTemplate, values),
		"internal/users/users_test.go":                                 renderTemplate(usersTestTemplate, values),
		"internal/repository/user_repository.go":                       renderTemplate(repositoryTemplate, values),
		"internal/repository/migrations.go":                            renderTemplate(repositoryMigrationsTemplate, values),
		"db/migrations/001_create_users_table.up.sql":                  renderTemplate(migrationUpTemplate, values),
		"db/migrations/001_create_users_table.down.sql":                renderTemplate(migrationDownTemplate, values),
		"db/migrations/embed.go":                                       renderTemplate(migrationEmbedTemplate, values),
		"features/users.feature":                                       renderTemplate(featureTemplate, values),
		"features/component_test.go":                                   renderTemplate(componentTestTemplate, values),
		"features/steps/component_suite.go":                            renderTemplate(componentSuiteTemplate, values),
		"features/steps/user_steps.go":                                 renderTemplate(userStepsTemplate, values),
		"features/steps/health_steps.go":                               renderTemplate(healthStepsTemplate, values),
		"docs/openapi.yaml":                                            renderTemplate(openAPITemplate, values),
		"README.md":                                                    renderTemplate(readmeTemplate, values),
	}
}

const goModTemplate = `
module {{MODULE}}

go 1.22.0
`

const gitignoreTemplate = `
.DS_Store
bin/
dist/
.idea/
.vscode/
.env
coverage.out
test-results/
`

const makefileTemplate = `
SERVICE_NAME := {{SERVICE_NAME}}
APP_ENTRY := ./cmd/{{SERVICE_NAME}}
TAGS ?=

.PHONY: help deps deps-update verify verify-full run test test-component fmt lint build docker-build db-up db-down

help:
	@echo "make deps           Resolve module dependencies"
	@echo "make deps-update    Update dependencies to newer compatible versions"
	@echo "make verify         Run quick post-generation verification"
	@echo "make verify-full    Run deeper verification including component tests"
	@echo "make run            Run the service locally"
	@echo "make test           Run unit tests"
	@echo "make test-component Run full component tests (optional: TAGS='@component')"
	@echo "make fmt            Format source files"
	@echo "make lint           Run vet as the default lint check"
	@echo "make build          Build the service binary"
	@echo "make docker-build   Build the container image"
	@echo "make db-up          Start PostgreSQL"
	@echo "make db-down        Stop PostgreSQL"

deps:
	go mod tidy

deps-update:
	go get -u ./...
	go mod tidy

verify:
	go test ./...
	go build $(APP_ENTRY)


verify-full: verify
	ENABLE_COMPONENT_TESTS=1 COMPONENT_TAGS="$(TAGS)" go test ./features/... -count=1

run:
	go run $(APP_ENTRY)

test:
	go test ./...

test-component:
	ENABLE_COMPONENT_TESTS=1 COMPONENT_TAGS="$(TAGS)" go test ./features/... -count=1

fmt:
	gofmt -w $(shell find . -name '*.go' -type f)

lint:
	go vet ./...

build:
	go build -o bin/$(SERVICE_NAME) $(APP_ENTRY)

docker-build:
	docker build -t $(SERVICE_NAME):local .

db-up:
	docker compose up -d postgres

db-down:
	docker compose down
`

const dockerfileTemplate = `
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/{{SERVICE_NAME}} ./cmd/{{SERVICE_NAME}}

FROM scratch
COPY --from=build /out/{{SERVICE_NAME}} /{{SERVICE_NAME}}
EXPOSE 8080
ENTRYPOINT ["/{{SERVICE_NAME}}"]
`

const dockerComposeTemplate = `
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: app
      POSTGRES_USER: app
      POSTGRES_PASSWORD: app
    ports:
      - "5432:5432"
    volumes:
      - postgres-data:/var/lib/postgresql/data

volumes:
  postgres-data:
`

const gitlabCITemplate = `
stages:
  - lint
  - test
  - build

variables:
  GO_VERSION: "1.22"
  SERVICE_NAME: "{{SERVICE_NAME}}"

lint:
  stage: lint
  image: golang:${GO_VERSION}
  script:
		- go mod tidy
    - go fmt ./...
    - go vet ./...

test:
  stage: test
  image: golang:${GO_VERSION}
  services:
	- name: docker:27-dind
	  alias: docker
  variables:
	DOCKER_HOST: tcp://docker:2375
	DOCKER_TLS_CERTDIR: ""
  script:
		- go mod tidy
	- go test ./...
	- ENABLE_COMPONENT_TESTS=1 go test ./features/... -count=1

build:
  stage: build
  image: docker:27
  services:
    - docker:27-dind
  script:
    - docker build -t $SERVICE_NAME:$CI_COMMIT_SHORT_SHA .
`

const configYAMLTemplate = `
service:
  name: {{SERVICE_NAME}}
  environment: dev
  http_port: 8080
  tls_enabled: false
  tls_cert_file: ""
  tls_key_file: ""

database:
  host: localhost
  port: 5432
  name: app
  user: app
  password: app
  sslmode: disable

observability:
  log_level: info
  otel_endpoint: ""
`

const mainTemplate = `
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"{{MODULE}}/db/migrations"
	"{{MODULE}}/internal/config"
	apphttp "{{MODULE}}/internal/httpserver"
	"{{MODULE}}/internal/observability"
	"{{MODULE}}/internal/repository"
	"{{MODULE}}/internal/users"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger := observability.NewLogger(cfg.Service.Name, cfg.Observability.LogLevel)
	if err := repository.RunMigrations(cfg.Database.DSN(), migrations.Files); err != nil {
		logger.Error("failed to apply migrations", slog.String("error", err.Error()))
		os.Exit(1)
	}

	repo, err := repository.NewUserRepository(cfg.Database.DSN())
	if err != nil {
		logger.Error("failed to connect to database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer func() {
		if closeErr := repo.Close(); closeErr != nil {
			logger.Error("failed to close database", slog.String("error", closeErr.Error()))
		}
	}()

	handler := users.NewHandler(users.NewService(repo), logger)
	server := apphttp.NewServer(cfg, logger, handler, repo)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("server stopped with error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}
}
`

const configGoTemplate = `
package config

import (
	"fmt"

	"github.com/ilyakaznacheev/cleanenv"
)

type DatabaseConfig struct {
	Host     string {{BT}}yaml:"host" env:"SERVICE_DATABASE_HOST" env-default:"localhost"{{BT}}
	Port     int    {{BT}}yaml:"port" env:"SERVICE_DATABASE_PORT" env-default:"5432"{{BT}}
	Name     string {{BT}}yaml:"name" env:"SERVICE_DATABASE_NAME" env-default:"app"{{BT}}
	User     string {{BT}}yaml:"user" env:"SERVICE_DATABASE_USER" env-default:"app"{{BT}}
	Password string {{BT}}yaml:"password" env:"SERVICE_DATABASE_PASSWORD" env-default:"app"{{BT}}
	SSLMode  string {{BT}}yaml:"sslmode" env:"SERVICE_DATABASE_SSLMODE" env-default:"disable"{{BT}}
}

type Config struct {
	Service struct {
		Name        string {{BT}}yaml:"name" env:"SERVICE_NAME" env-default:"service"{{BT}}
		Environment string {{BT}}yaml:"environment" env:"SERVICE_ENVIRONMENT" env-default:"dev"{{BT}}
		HTTPPort    int    {{BT}}yaml:"http_port" env:"SERVICE_HTTP_PORT" env-default:"8080"{{BT}}
		TLSEnabled  bool   {{BT}}yaml:"tls_enabled" env:"SERVICE_TLS_ENABLED" env-default:"false"{{BT}}
		TLSCertFile string {{BT}}yaml:"tls_cert_file" env:"SERVICE_TLS_CERT_FILE"{{BT}}
		TLSKeyFile  string {{BT}}yaml:"tls_key_file" env:"SERVICE_TLS_KEY_FILE"{{BT}}
	} {{BT}}yaml:"service"{{BT}}
	Database DatabaseConfig {{BT}}yaml:"database"{{BT}}
	Observability struct {
		LogLevel     string {{BT}}yaml:"log_level" env:"SERVICE_OBSERVABILITY_LOG_LEVEL" env-default:"info"{{BT}}
		OTELEndpoint string {{BT}}yaml:"otel_endpoint" env:"SERVICE_OBSERVABILITY_OTEL_ENDPOINT"{{BT}}
	} {{BT}}yaml:"observability"{{BT}}
}

func Load() (Config, error) {
	var cfg Config
	if err := cleanenv.ReadConfig("config/config.yaml", &cfg); err != nil {
		return Config{}, err
	}
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Address() string {
	return fmt.Sprintf(":%d", c.Service.HTTPPort)
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		d.Host,
		d.Port,
		d.Name,
		d.User,
		d.Password,
		d.SSLMode,
	)
}
`

const observabilityTemplate = `
package observability

import (
	"log/slog"
	"os"
	"strings"
)

func NewLogger(serviceName string, levelName string) *slog.Logger {
	level := new(slog.LevelVar)
	switch strings.ToLower(levelName) {
	case "debug":
		level.Set(slog.LevelDebug)
	case "warn":
		level.Set(slog.LevelWarn)
	case "error":
		level.Set(slog.LevelError)
	default:
		level.Set(slog.LevelInfo)
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(handler).With(slog.String("service", serviceName))
}
`

const httpServerTemplate = `
package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"{{MODULE}}/internal/config"
	"{{MODULE}}/internal/users"
)

type ReadinessChecker interface {
	Ready(ctx context.Context) error
}

type Server struct {
	server *http.Server
}

func NewServer(cfg config.Config, logger *slog.Logger, handler *users.Handler, readiness ReadinessChecker) *Server {
	return &Server{
		server: &http.Server{
			Addr:              cfg.Address(),
			Handler:           NewHandler(logger, handler, readiness),
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

func NewHandler(logger *slog.Logger, handler *users.Handler, readiness ReadinessChecker) http.Handler {
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID)
	router.Use(chimiddleware.RealIP)
	router.Use(chimiddleware.Recoverer)
	router.Use(requestLogger(logger))

	router.Get("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	router.Get("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if readiness != nil {
			if err := readiness.Ready(ctx); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte("not ready"))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	router.Route("/v1/users", func(r chi.Router) {
		r.Get("/", handler.List)
		r.Post("/", handler.Create)
		r.Delete("/{id}", handler.Delete)
	})

	return router
}

func (s *Server) Start() error {
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			next.ServeHTTP(w, r)
			logger.Info("request completed",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("request_id", chimiddleware.GetReqID(r.Context())),
				slog.String("trace_id", ""),
				slog.String("duration", time.Since(started).String()),
			)
		})
	}
}
`

const usersTemplate = `
package users

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type User struct {
	ID        string    {{BT}}json:"id"{{BT}}
	Email     string    {{BT}}json:"email"{{BT}}
	Name      string    {{BT}}json:"name"{{BT}}
	CreatedAt time.Time {{BT}}json:"created_at"{{BT}}
}

var (
	ErrValidation = errors.New("validation failed")
	ErrConflict   = errors.New("conflict")
	ErrNotFound   = errors.New("not found")
)

type Repository interface {
	List(rctx *http.Request) ([]User, error)
	Create(rctx *http.Request, user User) (User, error)
	Delete(rctx *http.Request, id string) error
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(r *http.Request) ([]User, error) {
	return s.repo.List(r)
}

func (s *Service) Create(r *http.Request, email string, name string) (User, error) {
	if email == "" || name == "" {
		return User{}, fmt.Errorf("%w: email and name are required", ErrValidation)
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return User{}, fmt.Errorf("%w: email must be a valid email address", ErrValidation)
	}
	user := User{
		ID:        uuid.NewString(),
		Email:     email,
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}
	return s.repo.Create(r, user)
}

func (s *Service) Delete(r *http.Request, id string) error {
	return s.repo.Delete(r, id)
}

type Handler struct {
	service *Service
	logger  *slog.Logger
}

func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	users, err := h.service.List(r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Email string {{BT}}json:"email"{{BT}}
		Name  string {{BT}}json:"name"{{BT}}
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}
	user, err := h.service.Create(r, payload.Email, payload.Name)
	if err != nil {
		switch {
		case errors.Is(err, ErrValidation):
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		case errors.Is(err, ErrConflict):
			writeError(w, http.StatusConflict, "CONFLICT", "user already exists")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		}
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "id is required")
		return
	}
	if err := h.service.Delete(r, id); err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, map[string]any{
		"status": status,
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}
`

const repositoryTemplate = `
package repository

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"{{MODULE}}/internal/users"
)

type userRow struct {
	ID        string    {{BT}}gorm:"column:id"{{BT}}
	Email     string    {{BT}}gorm:"column:email"{{BT}}
	Name      string    {{BT}}gorm:"column:name"{{BT}}
	CreatedAt time.Time {{BT}}gorm:"column:created_at"{{BT}}
}

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(dsn string) (*UserRepository, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	return &UserRepository{db: db}, nil
}

func (r *UserRepository) List(_ *http.Request) ([]users.User, error) {
	var records []userRow
	if err := r.db.Table("users").Order("created_at desc").Find(&records).Error; err != nil {
		return nil, err
	}
	result := make([]users.User, 0, len(records))
	for _, record := range records {
		result = append(result, users.User{
			ID:        record.ID,
			Email:     record.Email,
			Name:      record.Name,
			CreatedAt: record.CreatedAt,
		})
	}
	return result, nil
}

func (r *UserRepository) Create(_ *http.Request, user users.User) (users.User, error) {
	payload := map[string]any{
		"id":         user.ID,
		"email":      user.Email,
		"name":       user.Name,
		"created_at": user.CreatedAt,
	}
	if err := r.db.Table("users").Create(payload).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return users.User{}, users.ErrConflict
		}
		return users.User{}, err
	}
	return user, nil
}

func (r *UserRepository) Delete(_ *http.Request, id string) error {
	result := r.db.Table("users").Where("id = ?", id).Delete(&userRow{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return users.ErrNotFound
	}
	return nil
}

func (r *UserRepository) Ready(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (r *UserRepository) Close() error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (r *UserRepository) TruncateUsers(ctx context.Context) error {
	return r.db.WithContext(ctx).Exec("TRUNCATE TABLE users").Error
}
`

const repositoryMigrationsTemplate = `
package repository

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
	migrate "github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func RunMigrations(dsn string, files embed.FS) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	sourceDriver, err := iofs.New(files, ".")
	if err != nil {
		return err
	}

	databaseDriver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return err
	}

	migrationRunner, err := migrate.NewWithInstance("iofs", sourceDriver, "postgres", databaseDriver)
	if err != nil {
		return err
	}

	if err := migrationRunner.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}

	sourceErr, databaseErr := migrationRunner.Close()
	if sourceErr != nil {
		return sourceErr
	}
	return databaseErr
}
`

const migrationUpTemplate = `
CREATE TABLE IF NOT EXISTS users (
	id TEXT PRIMARY KEY,
	email TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`

const migrationDownTemplate = `
DROP TABLE IF EXISTS users;
`

const migrationEmbedTemplate = `
package migrations

import "embed"

//go:embed *.sql
var Files embed.FS
`

const featureTemplate = `
@component
Feature: users API
	Scenario: liveness succeeds
		Given the service is running
		When I request the liveness endpoint
		Then the response status should be 200

	Scenario: readiness succeeds
		Given the service is running
		When I request the readiness endpoint
		Then the response status should be 200

  Scenario: create a user
		Given the service is running
		When I create a user named "Ada Lovelace" with email "ada@example.com"
		Then the response status should be 201
		And the response body should contain name "Ada Lovelace"
		And the response body should contain email "ada@example.com"

	Scenario: list users includes created records
		Given the service is running
		When I create a user named "Ada Lovelace" with email "ada@example.com"
		And I create a user named "Grace Hopper" with email "grace@example.com"
		And I list users
		Then the response status should be 200
		And the response body should include user email "ada@example.com"
		And the response body should include user email "grace@example.com"

	Scenario: delete an existing user
		Given the service is running
		When I create a user named "Ada Lovelace" with email "ada@example.com"
		And I remember the created user id
		And I delete the remembered user
		Then the response status should be 204
		When I list users
		Then the response status should be 200
		And the response body should not include user email "ada@example.com"

	Scenario: reject invalid create payloads
		Given the service is running
		When I create a user named "" with email "bad-email"
		Then the response status should be 400
		And the error code should be "VALIDATION_ERROR"

	Scenario: reject duplicate emails
		Given the service is running
		When I create a user named "Ada Lovelace" with email "ada@example.com"
		And I create a user named "Ada Lovelace" with email "ada@example.com"
		Then the response status should be 409
		And the error code should be "CONFLICT"

	Scenario: delete returns not found for unknown users
		Given the service is running
		When I delete user with id "missing-user"
		Then the response status should be 404
		And the error code should be "NOT_FOUND"
`

const componentTestTemplate = `
package features

import (
	"os"
	"testing"

	"{{MODULE}}/features/steps"
)

func TestComponentScenarios(t *testing.T) {
	if os.Getenv("ENABLE_COMPONENT_TESTS") != "1" {
		t.Skip("component tests disabled; set ENABLE_COMPONENT_TESTS=1 to run them")
	}
	steps.RunComponentSuite(t)
}
`

const componentSuiteTemplate = `
package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"{{MODULE}}/db/migrations"
	apphttp "{{MODULE}}/internal/httpserver"
	"{{MODULE}}/internal/repository"
	"{{MODULE}}/internal/users"
)

type componentSuite struct {
	t         *testing.T
	ctx       context.Context
	cancel    context.CancelFunc
	container testcontainers.Container
	repo      *repository.UserRepository
	server    *httptest.Server
	client    *http.Client
	logBuffer bytes.Buffer

	lastResponse   *http.Response
	lastBody       []byte
	rememberedUser string
}

func RunComponentSuite(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	suite := &componentSuite{
		t:      t,
		ctx:    ctx,
		cancel: cancel,
	}

	if err := suite.start(); err != nil {
		t.Fatalf("start component suite: %v", err)
	}
	defer suite.stop()

	options := godog.Options{
		Format:   "pretty",
		Paths:    []string{"users.feature"},
		TestingT: t,
	}
	if tags := strings.TrimSpace(os.Getenv("COMPONENT_TAGS")); tags != "" {
		options.Tags = tags
	}

	status := godog.TestSuite{
		Name:                "component",
		ScenarioInitializer: suite.InitializeScenario,
		Options:             &options,
	}.Run()

	if status != 0 {
		suite.dumpLogs()
		t.Fatalf("component scenarios failed with exit code %d", status)
	}
}

func (s *componentSuite) start() error {
	request := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_DB":       "app",
			"POSTGRES_USER":     "app",
			"POSTGRES_PASSWORD": "app",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(s.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: request,
		Started:          true,
	})
	if err != nil {
		return fmt.Errorf("docker is required for component tests: %w", err)
	}
	s.container = container

	dsn, err := s.databaseDSN()
	if err != nil {
		return err
	}

	if err := repository.RunMigrations(dsn, migrations.Files); err != nil {
		return err
	}

	repo, err := repository.NewUserRepository(dsn)
	if err != nil {
		return err
	}
	s.repo = repo

	logger := slog.New(slog.NewJSONHandler(&s.logBuffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
	handler := users.NewHandler(users.NewService(repo), logger)
	s.server = httptest.NewServer(apphttp.NewHandler(logger, handler, repo))
	s.client = s.server.Client()
	return nil
}

func (s *componentSuite) stop() {
	if s.server != nil {
		s.server.Close()
	}
	if s.repo != nil {
		_ = s.repo.Close()
	}
	if s.container != nil {
		_ = s.container.Terminate(s.ctx)
	}
	s.cancel()
}

func (s *componentSuite) InitializeScenario(sc *godog.ScenarioContext) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return ctx, s.resetScenario()
	})

	registerUserSteps(sc, s)
	registerHealthSteps(sc, s)
}

func (s *componentSuite) resetScenario() error {
	s.lastResponse = nil
	s.lastBody = nil
	s.rememberedUser = ""
	return s.repo.TruncateUsers(s.ctx)
}

func (s *componentSuite) doRequest(method string, path string, body io.Reader) error {
	if s.lastResponse != nil && s.lastResponse.Body != nil {
		_ = s.lastResponse.Body.Close()
	}
	request, err := http.NewRequestWithContext(s.ctx, method, s.server.URL+path, body)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		_ = response.Body.Close()
		return err
	}
	_ = response.Body.Close()

	s.lastResponse = response
	s.lastBody = payload
	return nil
}

func (s *componentSuite) parseResponseObject() (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(s.lastBody, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *componentSuite) parseResponseArray() ([]map[string]any, error) {
	var payload []map[string]any
	if err := json.Unmarshal(s.lastBody, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *componentSuite) databaseDSN() (string, error) {
	host, err := s.container.Host(s.ctx)
	if err != nil {
		return "", err
	}
	port, err := s.container.MappedPort(s.ctx, "5432/tcp")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("host=%s port=%s dbname=app user=app password=app sslmode=disable", host, port.Port()), nil
}

func (s *componentSuite) dumpLogs() {
	if s.logBuffer.Len() > 0 {
		s.t.Logf("service logs:\n%s", s.logBuffer.String())
	}
	if s.container == nil {
		return
	}
	reader, err := s.container.Logs(s.ctx)
	if err != nil {
		s.t.Logf("read postgres logs: %v", err)
		return
	}
	defer reader.Close()
	payload, err := io.ReadAll(reader)
	if err != nil {
		s.t.Logf("read postgres logs payload: %v", err)
		return
	}
	s.t.Logf("postgres logs:\n%s", string(payload))
}
`

const userStepsTemplate = `
package steps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cucumber/godog"
)

func registerUserSteps(sc *godog.ScenarioContext, suite *componentSuite) {
	sc.Step(` + "`^the service is running$`" + `, suite.theServiceIsRunning)
	sc.Step(` + "`^I create a user named \"([^\"]*)\" with email \"([^\"]*)\"$`" + `, suite.iCreateAUserNamedWithEmail)
	sc.Step(` + "`^I list users$`" + `, suite.iListUsers)
	sc.Step(` + "`^I remember the created user id$`" + `, suite.iRememberTheCreatedUserID)
	sc.Step(` + "`^I delete the remembered user$`" + `, suite.iDeleteTheRememberedUser)
	sc.Step(` + "`^I delete user with id \"([^\"]*)\"$`" + `, suite.iDeleteUserWithID)
	sc.Step(` + "`^the response status should be (\\d+)$`" + `, suite.theResponseStatusShouldBe)
	sc.Step(` + "`^the response body should contain name \"([^\"]*)\"$`" + `, suite.theResponseBodyShouldContainName)
	sc.Step(` + "`^the response body should contain email \"([^\"]*)\"$`" + `, suite.theResponseBodyShouldContainEmail)
	sc.Step(` + "`^the response body should include user email \"([^\"]*)\"$`" + `, suite.theResponseBodyShouldIncludeUserEmail)
	sc.Step(` + "`^the response body should not include user email \"([^\"]*)\"$`" + `, suite.theResponseBodyShouldNotIncludeUserEmail)
	sc.Step(` + "`^the error code should be \"([^\"]*)\"$`" + `, suite.theErrorCodeShouldBe)
}

func (s *componentSuite) theServiceIsRunning() error {
	if s.server == nil {
		return fmt.Errorf("service server is not running")
	}
	return nil
}

func (s *componentSuite) iCreateAUserNamedWithEmail(name string, email string) error {
	payload, err := json.Marshal(map[string]string{
		"name":  name,
		"email": email,
	})
	if err != nil {
		return err
	}
	return s.doRequest(http.MethodPost, "/v1/users/", bytes.NewReader(payload))
}

func (s *componentSuite) iListUsers() error {
	return s.doRequest(http.MethodGet, "/v1/users/", nil)
}

func (s *componentSuite) iRememberTheCreatedUserID() error {
	payload, err := s.parseResponseObject()
	if err != nil {
		return err
	}
	id, ok := payload["id"].(string)
	if !ok || id == "" {
		return fmt.Errorf("response body does not contain an id")
	}
	s.rememberedUser = id
	return nil
}

func (s *componentSuite) iDeleteTheRememberedUser() error {
	if s.rememberedUser == "" {
		return fmt.Errorf("no remembered user id is available")
	}
	return s.iDeleteUserWithID(s.rememberedUser)
}

func (s *componentSuite) iDeleteUserWithID(id string) error {
	return s.doRequest(http.MethodDelete, "/v1/users/"+id, nil)
}

func (s *componentSuite) theResponseStatusShouldBe(status int) error {
	if s.lastResponse == nil {
		return fmt.Errorf("no response is available")
	}
	if s.lastResponse.StatusCode != status {
		return fmt.Errorf("expected status %d, got %d with body %s", status, s.lastResponse.StatusCode, string(s.lastBody))
	}
	return nil
}

func (s *componentSuite) theResponseBodyShouldContainName(name string) error {
	payload, err := s.parseResponseObject()
	if err != nil {
		return err
	}
	if payload["name"] != name {
		return fmt.Errorf("expected response name %q, got %v", name, payload["name"])
	}
	return nil
}

func (s *componentSuite) theResponseBodyShouldContainEmail(email string) error {
	payload, err := s.parseResponseObject()
	if err != nil {
		return err
	}
	if payload["email"] != email {
		return fmt.Errorf("expected response email %q, got %v", email, payload["email"])
	}
	return nil
}

func (s *componentSuite) theResponseBodyShouldIncludeUserEmail(email string) error {
	users, err := s.parseResponseArray()
	if err != nil {
		return err
	}
	for _, entry := range users {
		if entry["email"] == email {
			return nil
		}
	}
	return fmt.Errorf("response body does not include user email %q", email)
}

func (s *componentSuite) theResponseBodyShouldNotIncludeUserEmail(email string) error {
	users, err := s.parseResponseArray()
	if err != nil {
		return err
	}
	for _, entry := range users {
		if entry["email"] == email {
			return fmt.Errorf("response body unexpectedly includes user email %q", email)
		}
	}
	return nil
}

func (s *componentSuite) theErrorCodeShouldBe(code string) error {
	payload, err := s.parseResponseObject()
	if err != nil {
		return err
	}
	errorPayload, ok := payload["error"].(map[string]any)
	if !ok {
		return fmt.Errorf("response body does not contain an error object")
	}
	if errorPayload["code"] != code {
		return fmt.Errorf("expected error code %q, got %v", code, errorPayload["code"])
	}
	return nil
}
`

const healthStepsTemplate = `
package steps

import (
	"net/http"

	"github.com/cucumber/godog"
)

func registerHealthSteps(sc *godog.ScenarioContext, suite *componentSuite) {
	sc.Step(` + "`^I request the liveness endpoint$`" + `, suite.iRequestTheLivenessEndpoint)
	sc.Step(` + "`^I request the readiness endpoint$`" + `, suite.iRequestTheReadinessEndpoint)
}

func (s *componentSuite) iRequestTheLivenessEndpoint() error {
	return s.doRequest(http.MethodGet, "/health/live", nil)
}

func (s *componentSuite) iRequestTheReadinessEndpoint() error {
	return s.doRequest(http.MethodGet, "/health/ready", nil)
}
`

const usersTestTemplate = `
package users

import (
	"errors"
	"net/http"
	"testing"
)

type repositoryStub struct {
	createFn func(rctx *http.Request, user User) (User, error)
}

func (r repositoryStub) List(_ *http.Request) ([]User, error) {
	return nil, nil
}

func (r repositoryStub) Create(rctx *http.Request, user User) (User, error) {
	if r.createFn != nil {
		return r.createFn(rctx, user)
	}
	return user, nil
}

func (r repositoryStub) Delete(_ *http.Request, _ string) error {
	return nil
}

func TestServiceCreateValidatesInput(t *testing.T) {
	tests := []struct {
		name  string
		email string
		user  string
		err   error
	}{
		{name: "missing name", email: "ada@example.com", user: "", err: ErrValidation},
		{name: "invalid email", email: "bad-email", user: "Ada", err: ErrValidation},
	}

	service := NewService(repositoryStub{})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.Create(nil, test.email, test.user)
			if !errors.Is(err, test.err) {
				t.Fatalf("expected error %v, got %v", test.err, err)
			}
		})
	}
}

func TestServiceCreateReturnsRepositoryErrors(t *testing.T) {
	service := NewService(repositoryStub{
		createFn: func(_ *http.Request, _ User) (User, error) {
			return User{}, ErrConflict
		},
	})

	_, err := service.Create(nil, "ada@example.com", "Ada Lovelace")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict error, got %v", err)
	}
}
`

const openAPITemplate = `
openapi: 3.0.3
info:
  title: {{DISPLAY_NAME}} API
  version: 1.0.0
paths:
  /v1/users/:
    get:
      summary: List users
      responses:
        '200':
          description: User list
    post:
      summary: Create user
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [email, name]
              properties:
                email:
                  type: string
                  format: email
                name:
                  type: string
      responses:
        '201':
          description: User created
        '400':
          description: Invalid create payload
        '409':
          description: Duplicate email conflict
  /v1/users/{id}:
    delete:
      summary: Delete user
      parameters:
        - in: path
          name: id
          required: true
          schema:
            type: string
      responses:
        '204':
          description: User deleted
        '404':
          description: User not found
  /health/live:
    get:
      summary: Liveness probe
      responses:
        '200':
          description: Service is alive
  /health/ready:
    get:
      summary: Readiness probe
      responses:
        '200':
          description: Service is ready
`

const readmeTemplate = `
# {{DISPLAY_NAME}}

Generated from {{BT}}ms_template/DECISIONS.md{{BT}}.

## Local development

- {{BT}}make deps{{BT}} resolves dependencies for the current source tree.
- {{BT}}make deps-update{{BT}} refreshes dependencies to newer compatible versions.
- {{BT}}make db-up{{BT}} starts PostgreSQL.
- {{BT}}make run{{BT}} starts the service.
- {{BT}}make test{{BT}} runs unit tests.
- {{BT}}make test-component{{BT}} runs the full component suite. Docker must be available because the suite uses {{BT}}testcontainers-go{{BT}}.
- Optional filtering: {{BT}}make test-component TAGS='@component'{{BT}}.

## Key paths

- {{BT}}cmd/{{SERVICE_NAME}}/main.go{{BT}} - service entry point
- {{BT}}internal/{{BT}} - application code
- {{BT}}db/migrations/{{BT}} - SQL migrations
- {{BT}}features/{{BT}} - BDD component tests
- {{BT}}docs/openapi.yaml{{BT}} - API contract

## API semantics

- Creating a user with invalid input returns {{BT}}400{{BT}} with the standard error envelope.
- Creating a user with a duplicate email returns {{BT}}409{{BT}} with error code {{BT}}CONFLICT{{BT}}.
- Deleting a missing user returns {{BT}}404{{BT}} with error code {{BT}}NOT_FOUND{{BT}}.
`
