# E2E Test Makefile
# Usage: make <target>
#
# Run from repository root with: make -C test/e2e <target>
# Or run from test/e2e directory with: make <target>

.PHONY: all build test test-quick test-full test-topology test-scenario \
        clean logs shell help images up down

# Default target
all: test-quick

# Configuration
COMPOSE_FILE ?= containers/docker-compose.yml
PROJECT_NAME ?= kscore-e2e
TIMEOUT ?= 300
GO_TEST_FLAGS ?= -v -count=1

# Detect container runtime (Docker or Podman)
CONTAINER_RUNTIME := $(shell command -v docker 2>/dev/null || command -v podman 2>/dev/null)
COMPOSE_CMD := $(shell if command -v docker >/dev/null 2>&1; then echo "docker compose"; elif command -v podman-compose >/dev/null 2>&1; then echo "podman-compose"; fi)

# Colors for output
GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
NC := \033[0m # No Color

#------------------------------------------------------------------------------
# Build Targets
#------------------------------------------------------------------------------

## Build container images for E2E testing
images:
	@echo "$(GREEN)Building E2E test images...$(NC)"
	$(COMPOSE_CMD) -f $(COMPOSE_FILE) -p $(PROJECT_NAME) build

## Build images without cache
images-nocache:
	@echo "$(GREEN)Building E2E test images (no cache)...$(NC)"
	$(COMPOSE_CMD) -f $(COMPOSE_FILE) -p $(PROJECT_NAME) build --no-cache

#------------------------------------------------------------------------------
# Container Lifecycle
#------------------------------------------------------------------------------

## Start the test environment
up: images
	@echo "$(GREEN)Starting E2E test environment...$(NC)"
	$(COMPOSE_CMD) -f $(COMPOSE_FILE) -p $(PROJECT_NAME) up -d --wait
	@echo "$(GREEN)Environment is ready$(NC)"

## Stop and remove the test environment
down:
	@echo "$(YELLOW)Stopping E2E test environment...$(NC)"
	$(COMPOSE_CMD) -f $(COMPOSE_FILE) -p $(PROJECT_NAME) down -v --remove-orphans

## Restart the test environment
restart: down up

## Show container status
status:
	$(COMPOSE_CMD) -f $(COMPOSE_FILE) -p $(PROJECT_NAME) ps

#------------------------------------------------------------------------------
# Test Targets
#------------------------------------------------------------------------------

## Run quick smoke tests (all-in-one topology, basic scenarios)
test-quick: images
	@echo "$(GREEN)Running quick E2E smoke tests...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout $(TIMEOUT)s ./topology/... -run "TestAllInOne"

## Run full E2E test suite
test-full: images
	@echo "$(GREEN)Running full E2E test suite...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout 600s ./...

## Run tests for a specific topology
## Usage: make test-topology TOPOLOGY=allinone
test-topology: images
ifndef TOPOLOGY
	$(error TOPOLOGY is not set. Usage: make test-topology TOPOLOGY=allinone)
endif
	@echo "$(GREEN)Running tests for topology: $(TOPOLOGY)...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout $(TIMEOUT)s ./topology/... -run "Test$(TOPOLOGY)"

## Run a specific test scenario
## Usage: make test-scenario SCENARIO=AgentRegistration
test-scenario: images
ifndef SCENARIO
	$(error SCENARIO is not set. Usage: make test-scenario SCENARIO=AgentRegistration)
endif
	@echo "$(GREEN)Running scenario: $(SCENARIO)...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout $(TIMEOUT)s ./... -run "$(SCENARIO)"

## Run state management tests
test-state: images
	@echo "$(GREEN)Running state management E2E tests...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout $(TIMEOUT)s ./scenarios/... -run "State"

## Run event system tests
test-events: images
	@echo "$(GREEN)Running event system E2E tests...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout $(TIMEOUT)s ./scenarios/... -run "Event"

## Run policy enforcement tests
test-policy: images
	@echo "$(GREEN)Running policy enforcement E2E tests...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout $(TIMEOUT)s ./scenarios/... -run "Policy"

## Run clustering tests
test-cluster: images
	@echo "$(GREEN)Running clustering E2E tests...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout 600s ./scenarios/... -run "Cluster"

#------------------------------------------------------------------------------
# Performance Tests
#------------------------------------------------------------------------------

## Run performance tests
test-performance:
	@echo "$(GREEN)Running performance tests...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_PERF_TESTS=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout 1800s ./performance/...

## Run agent scale tests
test-scale:
	@echo "$(GREEN)Running agent scale tests...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_PERF_TESTS=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout 1800s ./performance/... -run "Scale"

#------------------------------------------------------------------------------
# Chaos Tests
#------------------------------------------------------------------------------

## Run chaos tests (requires more resources)
test-chaos:
	@echo "$(RED)Running chaos tests (this may affect system stability)...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_CHAOS_TESTS=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout 1800s ./chaos/...

#------------------------------------------------------------------------------
# Debugging & Logs
#------------------------------------------------------------------------------

## Show logs from all containers
logs:
	$(COMPOSE_CMD) -f $(COMPOSE_FILE) -p $(PROJECT_NAME) logs -f

## Show logs from a specific service
## Usage: make logs-service SERVICE=server
logs-service:
ifndef SERVICE
	$(error SERVICE is not set. Usage: make logs-service SERVICE=server)
endif
	$(COMPOSE_CMD) -f $(COMPOSE_FILE) -p $(PROJECT_NAME) logs -f $(SERVICE)

## Show server logs
logs-server:
	$(COMPOSE_CMD) -f $(COMPOSE_FILE) -p $(PROJECT_NAME) logs -f server

## Show agent logs
logs-agents:
	$(COMPOSE_CMD) -f $(COMPOSE_FILE) -p $(PROJECT_NAME) logs -f agent-web-1 agent-web-2 agent-db-1

## Open shell in server container
shell-server:
	$(COMPOSE_CMD) -f $(COMPOSE_FILE) -p $(PROJECT_NAME) exec server /bin/sh

## Open shell in agent container
## Usage: make shell-agent AGENT=agent-web-1
shell-agent:
ifndef AGENT
	$(error AGENT is not set. Usage: make shell-agent AGENT=agent-web-1)
endif
	$(COMPOSE_CMD) -f $(COMPOSE_FILE) -p $(PROJECT_NAME) exec $(AGENT) /bin/sh

#------------------------------------------------------------------------------
# Cleanup
#------------------------------------------------------------------------------

## Clean up test environment and artifacts
clean: down
	@echo "$(YELLOW)Cleaning up E2E test artifacts...$(NC)"
	rm -rf reports/*.json reports/*.html
	$(CONTAINER_RUNTIME) image prune -f --filter "label=kscore.e2e=true" 2>/dev/null || true

## Deep clean - remove all E2E images
clean-all: clean
	@echo "$(RED)Removing all E2E test images...$(NC)"
	$(CONTAINER_RUNTIME) rmi kscore-server:e2e-test 2>/dev/null || true
	$(CONTAINER_RUNTIME) rmi kscore-agent:e2e-test 2>/dev/null || true

#------------------------------------------------------------------------------
# Help
#------------------------------------------------------------------------------

## Show this help message
help:
	@echo "Keystone Core E2E Test Makefile"
	@echo ""
	@echo "Usage: make <target>"
	@echo ""
	@echo "Test Targets:"
	@echo "  test-quick        Run quick smoke tests (~5 min)"
	@echo "  test-full         Run full E2E test suite (~30 min)"
	@echo "  test-topology     Run tests for specific topology"
	@echo "  test-scenario     Run a specific test scenario"
	@echo "  test-state        Run state management tests"
	@echo "  test-events       Run event system tests"
	@echo "  test-policy       Run policy enforcement tests"
	@echo "  test-cluster      Run clustering tests"
	@echo "  test-performance  Run performance tests"
	@echo "  test-chaos        Run chaos tests"
	@echo ""
	@echo "Environment Targets:"
	@echo "  images            Build container images"
	@echo "  up                Start test environment"
	@echo "  down              Stop test environment"
	@echo "  restart           Restart test environment"
	@echo "  status            Show container status"
	@echo ""
	@echo "Debugging Targets:"
	@echo "  logs              Show all container logs"
	@echo "  logs-server       Show server logs"
	@echo "  logs-agents       Show agent logs"
	@echo "  shell-server      Open shell in server"
	@echo "  shell-agent       Open shell in agent"
	@echo ""
	@echo "Cleanup Targets:"
	@echo "  clean             Clean up environment"
	@echo "  clean-all         Deep clean (remove images)"
	@echo ""
	@echo "Environment Variables:"
	@echo "  TOPOLOGY          Topology name for test-topology"
	@echo "  SCENARIO          Scenario name for test-scenario"
	@echo "  SERVICE           Service name for logs-service"
	@echo "  AGENT             Agent name for shell-agent"
	@echo "  TIMEOUT           Test timeout in seconds (default: 300)"
