.PHONY: build build-ui build-extproc-budget build-extproc-ratelimit build-ui-server run test lint clean docker-build docker-build-extproc-budget docker-build-extproc-ratelimit docker-build-ui docker-build-all deploy undeploy deploy-all deploy-extproc-budget deploy-extproc-ratelimit deploy-ui undeploy-all migrate migrate-002 port-forward port-forward-all port-forward-extproc port-forward-ui help ui-dev

# Variables
BINARY_NAME=quota-management
IMAGE_REPO?=
IMAGE_PREFIX := $(if $(IMAGE_REPO),$(IMAGE_REPO)/,)
DOCKER_IMAGE=quota-management
VERSION?=latest
NAMESPACE?=agentgateway-system
PLATFORMS?=linux/amd64,linux/arm64

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt

# Build Go binary
build:
	$(GOBUILD) -o bin/$(BINARY_NAME) -v ./cmd/server

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags="-w -s" -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/server

# Build UI
build-ui:
	cd ui && bun install && bun build

# Run locally (server only)
run: build
	DATABASE_URL=postgres://budget:budget@localhost:5432/budget_management?sslmode=disable \
	./bin/$(BINARY_NAME)

# Run locally with UI
run-with-ui: build build-ui
	DATABASE_URL=postgres://budget:budget@localhost:5432/budget_management?sslmode=disable \
	UI_PATH=ui/dist \
	./bin/$(BINARY_NAME)

# UI development server (with proxy to backend)
ui-dev:
	cd ui && bun dev

# Test
test:
	$(GOTEST) -v -race ./...

test-coverage:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

# Lint
lint:
	golangci-lint run ./...

fmt:
	$(GOFMT) -w -s .

# Clean
clean:
	rm -rf bin/
	rm -rf ui/dist/
	rm -f coverage.out coverage.html

# Dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy
	cd ui && bun install

# Docker build (includes UI)
docker-build:
	docker build -t $(IMAGE_PREFIX)$(DOCKER_IMAGE):$(VERSION) .

# Docker multi-arch build (requires working buildx)
docker-build-multiarch:
	docker buildx build --platform $(PLATFORMS) --load -t $(IMAGE_PREFIX)$(DOCKER_IMAGE):$(VERSION) .

docker-run:
	docker run --rm -it \
		-e DATABASE_URL=postgres://budget:budget@host.docker.internal:5432/budget_management?sslmode=disable \
		-p 4444:4444 \
		-p 8080:8080 \
		-p 9090:9090 \
		$(IMAGE_PREFIX)$(DOCKER_IMAGE):$(VERSION)

# Kubernetes deployment
deploy:
	kubectl apply -f deploy/postgres.yaml
	@echo "Waiting for PostgreSQL to be ready..."
	kubectl wait --for=condition=ready pod -l app=quota-management-postgres -n $(NAMESPACE) --timeout=120s
	sed 's|image: quota-management:latest|image: $(IMAGE_PREFIX)$(DOCKER_IMAGE):$(VERSION)|' \
		deploy/deployment.yaml | kubectl apply -f -
	kubectl rollout status deployment/quota-management -n $(NAMESPACE) --timeout=60s

undeploy:
	kubectl delete -f deploy/deployment.yaml --ignore-not-found
	kubectl delete -f deploy/postgres.yaml --ignore-not-found

# Local development with Docker Compose
dev-up:
	docker-compose up -d postgres
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 5
	@echo "Running migrations..."
	PGPASSWORD=budget psql -h localhost -U budget -d budget_management -f migrations/001_initial.sql || true
	@echo "Starting server..."
	DATABASE_URL=postgres://budget:budget@localhost:5432/budget_management?sslmode=disable \
	$(GOCMD) run ./cmd/server

dev-down:
	docker-compose down -v

# Database migrations
migrate:
	@echo "Running migrations..."
	PGPASSWORD=budget psql -h localhost -U budget -d budget_management -f migrations/001_initial.sql

# Port forward for local testing
port-forward:
	kubectl port-forward -n $(NAMESPACE) svc/quota-management 4444:4444 8080:8080 9090:9090

# Build ext-proc binaries
build-extproc-budget:
	$(GOBUILD) -o bin/quota-budget-extproc -v ./cmd/extproc-budget

build-extproc-ratelimit:
	$(GOBUILD) -o bin/quota-ratelimit-extproc -v ./cmd/extproc-ratelimit

# Build UI server binary
build-ui-server:
	$(GOBUILD) -o bin/quota-ui -v ./cmd/ui

# Docker - two container builds
docker-build-extproc-budget:
	docker buildx build --platform $(PLATFORMS) --load -f Dockerfile.extproc-budget -t $(IMAGE_PREFIX)quota-budget-extproc:$(VERSION) .

docker-build-extproc-ratelimit:
	docker buildx build --platform $(PLATFORMS) --load -f Dockerfile.extproc-ratelimit -t $(IMAGE_PREFIX)quota-ratelimit-extproc:$(VERSION) .

docker-build-ui:
	docker buildx build --platform $(PLATFORMS) --load -f Dockerfile.ui -t $(IMAGE_PREFIX)quota-management-ui:$(VERSION) .

docker-build-all: docker-build-extproc-budget docker-build-extproc-ratelimit docker-build-ui

# Deploy - two container
deploy-all:
	kubectl apply -f deploy/postgres.yaml
	@echo "Waiting for PostgreSQL to be ready..."
	kubectl wait --for=condition=ready pod -l app=quota-management-postgres -n $(NAMESPACE) --timeout=120s
	$(MAKE) deploy-extproc-budget
	$(MAKE) deploy-extproc-ratelimit
	sed 's|image: quota-management-ui:latest|image: $(IMAGE_PREFIX)quota-management-ui:$(VERSION)|' \
		deploy/ui-deployment.yaml | kubectl apply -f -
	kubectl apply -f deploy/ui-service.yaml

deploy-extproc-budget:
	sed 's|image: quota-budget-extproc:latest|image: $(IMAGE_PREFIX)quota-budget-extproc:$(VERSION)|' \
		deploy/extproc-deployment.yaml | kubectl apply -f -
	kubectl apply -f deploy/extproc-service.yaml

deploy-extproc-ratelimit:
	sed 's|image: quota-ratelimit-extproc:latest|image: $(IMAGE_PREFIX)quota-ratelimit-extproc:$(VERSION)|' \
		deploy/ratelimit-deployment.yaml | kubectl apply -f -
	kubectl apply -f deploy/ratelimit-service.yaml

deploy-ui:
	sed 's|image: quota-management-ui:latest|image: $(IMAGE_PREFIX)quota-management-ui:$(VERSION)|' \
		deploy/ui-deployment.yaml | kubectl apply -f -
	kubectl apply -f deploy/ui-service.yaml

undeploy-all:
	kubectl delete -f deploy/extproc-deployment.yaml --ignore-not-found
	kubectl delete -f deploy/extproc-service.yaml --ignore-not-found
	kubectl delete -f deploy/ui-deployment.yaml --ignore-not-found
	kubectl delete -f deploy/ui-service.yaml --ignore-not-found
	kubectl delete -f deploy/postgres.yaml --ignore-not-found

# Port forward - two container
port-forward-all:
	@echo "Forwarding ext-proc (4444) and UI (8080)..."
	kubectl port-forward -n $(NAMESPACE) svc/quota-management-extproc 4444:4444 9090:9090 &
	kubectl port-forward -n $(NAMESPACE) svc/quota-management-ui 8080:8080 9091:9091

port-forward-extproc:
	kubectl port-forward -n $(NAMESPACE) svc/quota-management-extproc 4444:4444 9090:9090

port-forward-ui:
	kubectl port-forward -n $(NAMESPACE) svc/quota-management-ui 8080:8080 9091:9091

# Help
help:
	@echo "Quota Management Server"
	@echo ""
	@echo "Usage:"
	@echo "  make build           Build the server binary"
	@echo "  make build-ui        Build the UI"
	@echo "  make run             Build and run the server locally"
	@echo "  make run-with-ui     Build and run server with UI"
	@echo "  make ui-dev          Start UI development server"
	@echo "  make test            Run tests"
	@echo "  make lint            Run linter"
	@echo "  make clean           Clean build artifacts"
	@echo "  make deps            Download all dependencies (Go + UI)"
	@echo "  make docker-build    Build Docker image (multi-arch, includes UI)"
	@echo "  Push image: from repository root, make push-quota-management"
	@echo "  make deploy          Deploy to Kubernetes"
	@echo "  make undeploy        Remove from Kubernetes"
	@echo "  make dev-up          Start local development environment"
	@echo "  make dev-down        Stop local development environment"
	@echo "  make migrate         Run database migrations"
	@echo "  make port-forward    Port forward services for local testing"
	@echo "  make help            Show this help"
