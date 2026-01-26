#!/bin/bash

set -e

COMMAND=$1
SHIFT_ARGS=${@:2}

# Function to check if the stack is up
is_stack_up() {
  if docker compose -f deploy/docker-compose.yml ps | grep -q "running"; then
    return 0
  else
    return 1
  fi
}

# Function to wait for services to be healthy
wait_for_healthy() {
  echo "⏳ Waiting for services to be healthy..."
  local timeout=60
  local count=0
  until [ "$(docker compose -f deploy/docker-compose.yml ps --format json | grep -c '"Health":"healthy"')" -eq 3 ] || [ $count -eq $timeout ]; do
    sleep 1
    ((count++))
    printf "."
  done
  echo ""
  if [ $count -eq $timeout ]; then
    echo "❌ Timeout waiting for services to be healthy."
    return 1
  fi
  echo "✅ All services are healthy!"
}

case $COMMAND in
  up)
    echo "🚀 Starting observability stack..."
    if [ -z "$REPO_URL" ] || [ -z "$RUNNER_TOKEN" ]; then
      echo "⚠️  REPO_URL or RUNNER_TOKEN not set. Runner service will likely fail to start."
      echo "   To use the self-hosted runner, run: REPO_URL=... RUNNER_TOKEN=... ./run.sh up"
    fi
    docker compose -f deploy/docker-compose.yml up -d
    wait_for_healthy
    echo "✅ Stack is up! Grafana: http://localhost:3000"
    ;;
  down)
    echo "🛑 Stopping observability stack..."
    docker compose -f deploy/docker-compose.yml down
    ;;
  status)
    echo "📊 Stack Status:"
    docker compose -f deploy/docker-compose.yml ps
    ;;
  cli)
    echo "🔍 Running one-off analysis..."
    # Automatically add local collector if stack is up
    EXTRA_FLAGS=""
    if is_stack_up; then
      echo "💡 Local observability stack detected, traces will be sent to Tempo."
    fi
    go run cmd/gha-analyzer/main.go $SHIFT_ARGS $EXTRA_FLAGS
    ;;
  server)
    echo "📡 Starting webhook server..."
    go run cmd/gha-server/main.go
    ;;
  dashboard)
    if [[ "$OSTYPE" == "darwin"* ]]; then
      open http://localhost:3000/d/gha-analyzer
    else
      echo "Grafana Dashboard: http://localhost:3000/d/gha-analyzer"
    fi
    ;;
  simulate)
    echo "🧪 Running E2E simulation..."
    go run cmd/simulate/main.go
    ;;
  *)
    echo "Usage: ./run.sh {up|down|status|cli|server|dashboard|simulate}"
    exit 1
    ;;
esac
