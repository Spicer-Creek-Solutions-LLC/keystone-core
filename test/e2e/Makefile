# E2E Test Makefile
# Usage: make <target>
#
# Run from repository root with: make -C test/e2e <target>
# Or run from test/e2e directory with: make <target>

.PHONY: all build test test-quick test-full test-allinone test-topology test-scenario \
        test-state test-events test-policy test-cluster \
        test-exec test-agent test-gitops test-ha test-ha-quick test-ipv6 test-ipv6-quick \
        test-ha-ipv6 test-ha-ipv6-quick \
        test-performance test-scale test-throughput test-chaos \
        clean clean-all logs logs-service logs-server logs-agents shell-server shell-agent \
        help images images-nocache up down restart status \
        ha-images ha-up ha-down ha-status ha-logs \
        ipv6-images ipv6-up ipv6-down ipv6-status ipv6-logs \
        ha-ipv6-images ha-ipv6-up ha-ipv6-down ha-ipv6-status ha-ipv6-logs

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

## Run full E2E test suite (all topologies)
test-full: images
	@echo "$(GREEN)=============================================$(NC)"
	@echo "$(GREEN)Running FULL E2E test suite (all topologies)$(NC)"
	@echo "$(GREEN)=============================================$(NC)"
	@echo ""
	@echo "$(GREEN)=== Phase 1: All-in-one topology tests ===$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout 600s ./topology/... -run "TestAllInOne"
	KSCORE_E2E_TESTS=1 KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout 600s ./scenarios/...
	$(COMPOSE_CMD) -f $(COMPOSE_FILE) -p $(PROJECT_NAME) down -v --remove-orphans
	@echo ""
	@echo "$(GREEN)=== Phase 2: HA cluster topology tests ===$(NC)"
	$(COMPOSE_CMD) -f topologies/ha-cluster/docker-compose.yml -p kscore-e2e-ha build
	KSCORE_E2E_TESTS=1 KSCORE_TOPOLOGY=ha-cluster KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout 900s ./topology/... -run "HACluster"
	$(COMPOSE_CMD) -f topologies/ha-cluster/docker-compose.yml -p kscore-e2e-ha down -v --remove-orphans
	@echo ""
	@echo "$(GREEN)=== Phase 3: IPv6 topology tests ===$(NC)"
	$(COMPOSE_CMD) -f $(IPV6_COMPOSE_FILE) -p $(IPV6_PROJECT_NAME) build
	KSCORE_E2E_TESTS=1 KSCORE_TOPOLOGY=ipv6 KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout 600s ./topology/... -run "IPv6"
	$(COMPOSE_CMD) -f $(IPV6_COMPOSE_FILE) -p $(IPV6_PROJECT_NAME) down -v --remove-orphans
	@echo ""
	@echo "$(GREEN)=== Phase 4: HA cluster IPv6 topology tests ===$(NC)"
	$(COMPOSE_CMD) -f $(HA_IPV6_COMPOSE_FILE) -p $(HA_IPV6_PROJECT_NAME) build
	KSCORE_E2E_TESTS=1 KSCORE_TOPOLOGY=ha-cluster-ipv6 KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout 900s ./topology/... -run "HAClusterIPv6"
	$(COMPOSE_CMD) -f $(HA_IPV6_COMPOSE_FILE) -p $(HA_IPV6_PROJECT_NAME) down -v --remove-orphans
	@echo ""
	@echo "$(GREEN)=== Phase 5: Performance tests ===$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_PERF_TESTS=1 KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout 1800s ./performance/...
	@echo ""
	@echo "$(GREEN)=============================================$(NC)"
	@echo "$(GREEN)Full E2E test suite COMPLETE$(NC)"
	@echo "$(GREEN)=============================================$(NC)"

## Run all-in-one topology tests only
test-allinone: images
	@echo "$(GREEN)Running all-in-one topology E2E tests...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout 600s ./topology/... -run "TestAllInOne"
	KSCORE_E2E_TESTS=1 KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout 600s ./scenarios/...

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

## Run remote execution tests
test-exec: images
	@echo "$(GREEN)Running remote execution E2E tests...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout $(TIMEOUT)s ./scenarios/... -run "Exec"

## Run agent lifecycle tests
test-agent: images
	@echo "$(GREEN)Running agent lifecycle E2E tests...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout $(TIMEOUT)s ./scenarios/... -run "Agent"

## Run GitOps webhook tests
test-gitops: images
	@echo "$(GREEN)Running GitOps webhook E2E tests...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout $(TIMEOUT)s ./scenarios/... -run "GitOps"

## Run HA cluster tests
test-ha: ha-images
	@echo "$(GREEN)Running HA cluster E2E tests...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_TOPOLOGY=ha-cluster KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout 900s ./topology/... -run "HACluster"
	$(COMPOSE_CMD) -f $(HA_COMPOSE_FILE) -p $(HA_PROJECT_NAME) down -v --remove-orphans

#------------------------------------------------------------------------------
# HA Cluster Environment
#------------------------------------------------------------------------------

# HA Cluster topology configuration
HA_COMPOSE_FILE := topologies/ha-cluster/docker-compose.yml
HA_PROJECT_NAME := kscore-e2e-ha

## Build HA cluster container images
ha-images: images
	@echo "$(GREEN)Building HA cluster E2E test images...$(NC)"
	$(COMPOSE_CMD) -f $(HA_COMPOSE_FILE) -p $(HA_PROJECT_NAME) build

## Start HA cluster test environment
ha-up: ha-images
	@echo "$(GREEN)Starting HA cluster E2E test environment...$(NC)"
	$(COMPOSE_CMD) -f $(HA_COMPOSE_FILE) -p $(HA_PROJECT_NAME) up -d --wait
	@echo "$(GREEN)HA cluster environment is ready$(NC)"
	@echo "  Server 1: localhost:8080 (gRPC), localhost:8081 (HTTP)"
	@echo "  Server 2: localhost:8082 (gRPC), localhost:8083 (HTTP)"
	@echo "  Server 3: localhost:8084 (gRPC), localhost:8085 (HTTP)"

## Stop HA cluster test environment
ha-down:
	@echo "$(YELLOW)Stopping HA cluster E2E test environment...$(NC)"
	$(COMPOSE_CMD) -f $(HA_COMPOSE_FILE) -p $(HA_PROJECT_NAME) down -v --remove-orphans

## Show HA cluster container status
ha-status:
	$(COMPOSE_CMD) -f $(HA_COMPOSE_FILE) -p $(HA_PROJECT_NAME) ps

## Show HA cluster container logs
ha-logs:
	$(COMPOSE_CMD) -f $(HA_COMPOSE_FILE) -p $(HA_PROJECT_NAME) logs -f

## Run quick HA cluster tests (skip image build)
test-ha-quick:
	@echo "$(GREEN)Running HA cluster E2E tests (skip build)...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_TOPOLOGY=ha-cluster KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout 900s ./topology/... -run "HACluster"

#------------------------------------------------------------------------------
# IPv6 Tests
#------------------------------------------------------------------------------

# IPv6 topology configuration
IPV6_COMPOSE_FILE := topologies/ipv6/docker-compose.yml
IPV6_PROJECT_NAME := kscore-e2e-ipv6

## Build IPv6 container images
ipv6-images: images
	@echo "$(GREEN)Building IPv6 E2E test images...$(NC)"
	$(COMPOSE_CMD) -f $(IPV6_COMPOSE_FILE) -p $(IPV6_PROJECT_NAME) build

## Start IPv6 test environment
ipv6-up: ipv6-images
	@echo "$(GREEN)Starting IPv6 E2E test environment...$(NC)"
	$(COMPOSE_CMD) -f $(IPV6_COMPOSE_FILE) -p $(IPV6_PROJECT_NAME) up -d --wait
	@echo "$(GREEN)IPv6 environment is ready$(NC)"

## Stop IPv6 test environment
ipv6-down:
	@echo "$(YELLOW)Stopping IPv6 E2E test environment...$(NC)"
	$(COMPOSE_CMD) -f $(IPV6_COMPOSE_FILE) -p $(IPV6_PROJECT_NAME) down -v --remove-orphans

## Show IPv6 container status
ipv6-status:
	$(COMPOSE_CMD) -f $(IPV6_COMPOSE_FILE) -p $(IPV6_PROJECT_NAME) ps

## Show IPv6 container logs
ipv6-logs:
	$(COMPOSE_CMD) -f $(IPV6_COMPOSE_FILE) -p $(IPV6_PROJECT_NAME) logs -f

## Run IPv6 E2E tests
test-ipv6: ipv6-images
	@echo "$(GREEN)Running IPv6 E2E tests...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_TOPOLOGY=ipv6 KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout $(TIMEOUT)s ./topology/... -run "IPv6"
	$(COMPOSE_CMD) -f $(IPV6_COMPOSE_FILE) -p $(IPV6_PROJECT_NAME) down -v --remove-orphans

## Run quick IPv6 tests (skip image build)
test-ipv6-quick:
	@echo "$(GREEN)Running IPv6 E2E tests (skip build)...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_TOPOLOGY=ipv6 KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout $(TIMEOUT)s ./topology/... -run "IPv6"

#------------------------------------------------------------------------------
# HA Cluster IPv6 Tests
#------------------------------------------------------------------------------

# HA Cluster IPv6 topology configuration
HA_IPV6_COMPOSE_FILE := topologies/ha-cluster-ipv6/docker-compose.yml
HA_IPV6_PROJECT_NAME := kscore-e2e-ha-ipv6

## Build HA cluster IPv6 container images
ha-ipv6-images: images
	@echo "$(GREEN)Building HA cluster IPv6 E2E test images...$(NC)"
	$(COMPOSE_CMD) -f $(HA_IPV6_COMPOSE_FILE) -p $(HA_IPV6_PROJECT_NAME) build

## Start HA cluster IPv6 test environment
ha-ipv6-up: ha-ipv6-images
	@echo "$(GREEN)Starting HA cluster IPv6 E2E test environment...$(NC)"
	$(COMPOSE_CMD) -f $(HA_IPV6_COMPOSE_FILE) -p $(HA_IPV6_PROJECT_NAME) up -d --wait
	@echo "$(GREEN)HA cluster IPv6 environment is ready$(NC)"
	@echo "  Network: fd00:2::/64 (IPv6-only)"
	@echo "  NATS nodes: [fd00:2::11-13]:4222"
	@echo "  etcd nodes: [fd00:2::21-23]:2379"
	@echo "  PostgreSQL: [fd00:2::30]:5432"
	@echo "  Servers: [fd00:2::101-103]:8080"
	@echo "  Agents: [fd00:2::201-205]"

## Stop HA cluster IPv6 test environment
ha-ipv6-down:
	@echo "$(YELLOW)Stopping HA cluster IPv6 E2E test environment...$(NC)"
	$(COMPOSE_CMD) -f $(HA_IPV6_COMPOSE_FILE) -p $(HA_IPV6_PROJECT_NAME) down -v --remove-orphans

## Show HA cluster IPv6 container status
ha-ipv6-status:
	$(COMPOSE_CMD) -f $(HA_IPV6_COMPOSE_FILE) -p $(HA_IPV6_PROJECT_NAME) ps

## Show HA cluster IPv6 container logs
ha-ipv6-logs:
	$(COMPOSE_CMD) -f $(HA_IPV6_COMPOSE_FILE) -p $(HA_IPV6_PROJECT_NAME) logs -f

## Run HA cluster IPv6 E2E tests
test-ha-ipv6: ha-ipv6-images
	@echo "$(GREEN)Running HA cluster IPv6 E2E tests...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_TOPOLOGY=ha-cluster-ipv6 KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout 900s ./topology/... -run "HAClusterIPv6"
	$(COMPOSE_CMD) -f $(HA_IPV6_COMPOSE_FILE) -p $(HA_IPV6_PROJECT_NAME) down -v --remove-orphans

## Run quick HA cluster IPv6 tests (skip image build)
test-ha-ipv6-quick:
	@echo "$(GREEN)Running HA cluster IPv6 E2E tests (skip build)...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_TOPOLOGY=ha-cluster-ipv6 KSCORE_SKIP_BUILD=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout 900s ./topology/... -run "HAClusterIPv6"

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

## Run throughput tests
test-throughput:
	@echo "$(GREEN)Running throughput tests...$(NC)"
	KSCORE_E2E_TESTS=1 KSCORE_PERF_TESTS=1 KSCORE_ROOT=$(shell cd ../.. && pwd) \
		go test $(GO_TEST_FLAGS) -timeout 1800s ./performance/... -run "Throughput"

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
	@echo "  test-full         Run FULL E2E test suite - all topologies (~60 min)"
	@echo "  test-allinone     Run all-in-one topology tests only"
	@echo "  test-topology     Run tests for specific topology"
	@echo "  test-scenario     Run a specific test scenario"
	@echo "  test-state        Run state management tests"
	@echo "  test-events       Run event system tests"
	@echo "  test-policy       Run policy enforcement tests"
	@echo "  test-cluster      Run clustering tests"
	@echo "  test-exec         Run remote execution tests"
	@echo "  test-agent        Run agent lifecycle tests"
	@echo "  test-gitops       Run GitOps webhook tests"
	@echo "  test-ha           Run HA cluster tests (3 servers, 5 agents)"
	@echo "  test-ha-quick     Run HA cluster tests (skip build)"
	@echo "  test-ipv6         Run IPv6 E2E tests"
	@echo "  test-ipv6-quick   Run IPv6 tests (skip build)"
	@echo "  test-ha-ipv6      Run HA cluster over IPv6 E2E tests"
	@echo "  test-ha-ipv6-quick Run HA cluster IPv6 tests (skip build)"
	@echo "  test-performance  Run all performance tests"
	@echo "  test-scale        Run agent scale tests"
	@echo "  test-throughput   Run throughput tests"
	@echo "  test-chaos        Run chaos tests"
	@echo ""
	@echo "All-in-One Environment Targets:"
	@echo "  images            Build container images"
	@echo "  up                Start test environment"
	@echo "  down              Stop test environment"
	@echo "  restart           Restart test environment"
	@echo "  status            Show container status"
	@echo ""
	@echo "HA Cluster Environment Targets:"
	@echo "  ha-images         Build HA cluster container images"
	@echo "  ha-up             Start HA cluster test environment"
	@echo "  ha-down           Stop HA cluster test environment"
	@echo "  ha-status         Show HA cluster container status"
	@echo "  ha-logs           Show HA cluster container logs"
	@echo ""
	@echo "IPv6 Environment Targets:"
	@echo "  ipv6-images       Build IPv6 container images"
	@echo "  ipv6-up           Start IPv6 test environment"
	@echo "  ipv6-down         Stop IPv6 test environment"
	@echo "  ipv6-status       Show IPv6 container status"
	@echo "  ipv6-logs         Show IPv6 container logs"
	@echo ""
	@echo "HA Cluster IPv6 Environment Targets:"
	@echo "  ha-ipv6-images    Build HA IPv6 container images"
	@echo "  ha-ipv6-up        Start HA cluster IPv6 test environment"
	@echo "  ha-ipv6-down      Stop HA cluster IPv6 test environment"
	@echo "  ha-ipv6-status    Show HA cluster IPv6 container status"
	@echo "  ha-ipv6-logs      Show HA cluster IPv6 container logs"
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
	@echo ""
	@echo "Full Test Suite (test-full) runs:"
	@echo "  1. All-in-one topology tests"
	@echo "  2. HA cluster topology tests"
	@echo "  3. IPv6 topology tests"
	@echo "  4. HA cluster IPv6 topology tests"
	@echo "  5. Performance tests"
