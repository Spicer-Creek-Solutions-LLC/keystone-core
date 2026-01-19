#!/bin/bash
#
# Keystone Core Telemetry Gateway Quick Start
#
# This script sets up the complete observability stack with sensible defaults.
#
# Usage:
#   ./quick-start.sh              # Full stack with auto-generated password
#   ./quick-start.sh --minimal    # Just gateway and NATS
#   ./quick-start.sh --stop       # Stop all services
#   ./quick-start.sh --clean      # Stop and remove all data
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Check for docker and docker-compose
check_prerequisites() {
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed. Please install Docker first."
        exit 1
    fi

    if ! docker compose version &> /dev/null && ! command -v docker-compose &> /dev/null; then
        log_error "Docker Compose is not installed. Please install Docker Compose first."
        exit 1
    fi
}

# Determine docker compose command
get_compose_cmd() {
    if docker compose version &> /dev/null; then
        echo "docker compose"
    else
        echo "docker-compose"
    fi
}

# Generate a random password
generate_password() {
    if command -v openssl &> /dev/null; then
        openssl rand -base64 24 | tr -d '/+=' | head -c 24
    else
        head -c 24 /dev/urandom | base64 | tr -d '/+=' | head -c 24
    fi
}

# Create required directories
setup_directories() {
    mkdir -p grafana/provisioning/datasources
    mkdir -p grafana/provisioning/dashboards
}

# Create Grafana datasource provisioning
create_grafana_datasources() {
    cat > grafana/provisioning/datasources/datasources.yaml << 'EOF'
apiVersion: 1

datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://prometheus:9090
    isDefault: true
    editable: false

  - name: Loki
    type: loki
    access: proxy
    url: http://loki:3100
    editable: false

  - name: Tempo
    type: tempo
    access: proxy
    url: http://tempo:3200
    editable: false
    jsonData:
      tracesToLogsV2:
        datasourceUid: loki
        spanEndTimeShift: "1h"
        filterByTraceID: true
        filterBySpanID: true
      tracesToMetrics:
        datasourceUid: prometheus
      serviceMap:
        datasourceUid: prometheus
      nodeGraph:
        enabled: true
      lokiSearch:
        datasourceUid: loki
EOF
    log_info "Created Grafana datasource provisioning"
}

# Start minimal setup
start_minimal() {
    local COMPOSE_CMD=$(get_compose_cmd)
    log_info "Starting minimal gateway setup..."
    $COMPOSE_CMD -f docker-compose.minimal.yml up -d

    log_info "Waiting for services to be ready..."
    sleep 5

    echo ""
    log_info "Gateway is ready!"
    echo ""
    echo "  Gateway:  http://localhost:9091"
    echo "  Metrics:  http://localhost:9091/metrics"
    echo "  Health:   http://localhost:9091/health"
    echo ""
}

# Start full stack
start_full() {
    local COMPOSE_CMD=$(get_compose_cmd)

    # Generate password if not set
    if [ -z "$GRAFANA_ADMIN_PASSWORD" ]; then
        export GRAFANA_ADMIN_PASSWORD=$(generate_password)
        log_info "Generated Grafana admin password"
    fi

    setup_directories
    create_grafana_datasources

    log_info "Starting full observability stack..."
    $COMPOSE_CMD up -d

    log_info "Waiting for services to be ready..."
    sleep 10

    echo ""
    log_info "Observability stack is ready!"
    echo ""
    echo "  Gateway:    http://localhost:8080"
    echo "  Grafana:    http://localhost:3000"
    echo "  Prometheus: http://localhost:9090"
    echo "  Loki:       http://localhost:3100"
    echo "  Tempo:      http://localhost:3200"
    echo ""
    echo "  Grafana credentials:"
    echo "    Username: admin"
    echo "    Password: $GRAFANA_ADMIN_PASSWORD"
    echo ""
    log_warn "Save the Grafana password - it will not be shown again!"
    echo ""
}

# Stop services
stop_services() {
    local COMPOSE_CMD=$(get_compose_cmd)
    log_info "Stopping services..."

    if [ -f docker-compose.minimal.yml ]; then
        $COMPOSE_CMD -f docker-compose.minimal.yml down 2>/dev/null || true
    fi
    $COMPOSE_CMD down 2>/dev/null || true

    log_info "Services stopped"
}

# Clean up everything
clean_all() {
    local COMPOSE_CMD=$(get_compose_cmd)
    log_warn "This will remove all data volumes!"
    read -p "Are you sure? (y/N) " -n 1 -r
    echo

    if [[ $REPLY =~ ^[Yy]$ ]]; then
        log_info "Stopping and cleaning up..."

        if [ -f docker-compose.minimal.yml ]; then
            $COMPOSE_CMD -f docker-compose.minimal.yml down -v 2>/dev/null || true
        fi
        $COMPOSE_CMD down -v 2>/dev/null || true

        rm -rf grafana/provisioning

        log_info "Cleanup complete"
    else
        log_info "Cleanup cancelled"
    fi
}

# Show status
show_status() {
    local COMPOSE_CMD=$(get_compose_cmd)
    echo ""
    log_info "Service status:"
    $COMPOSE_CMD ps 2>/dev/null || $COMPOSE_CMD -f docker-compose.minimal.yml ps 2>/dev/null || echo "No services running"
    echo ""
}

# Main
check_prerequisites

case "${1:-}" in
    --minimal|-m)
        start_minimal
        ;;
    --stop|-s)
        stop_services
        ;;
    --clean|-c)
        clean_all
        ;;
    --status)
        show_status
        ;;
    --help|-h)
        echo "Keystone Core Telemetry Gateway Quick Start"
        echo ""
        echo "Usage: $0 [option]"
        echo ""
        echo "Options:"
        echo "  (none)      Start full observability stack"
        echo "  --minimal   Start minimal setup (gateway + NATS only)"
        echo "  --stop      Stop all services"
        echo "  --clean     Stop and remove all data"
        echo "  --status    Show service status"
        echo "  --help      Show this help"
        echo ""
        ;;
    *)
        start_full
        ;;
esac
