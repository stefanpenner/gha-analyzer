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
  local healthy_count=0

  while [ $count -lt $timeout ]; do
    # Count healthy services, suppress errors, default to 0
    healthy_count=$(docker compose -f deploy/docker-compose.yml ps --format json 2>/dev/null | grep '"Health":"healthy"' | wc -l | tr -d ' ' || echo "0")
    healthy_count=${healthy_count:-0}

    if [ $healthy_count -eq 3 ]; then
      echo ""
      echo "✅ All services are healthy!"
      return 0
    fi

    sleep 1
    ((count++))
    printf "."
  done

  echo ""
  echo "❌ Timeout waiting for services to be healthy."
  docker compose -f deploy/docker-compose.yml ps
  return 1
}

case $COMMAND in
  up)
    echo "🚀 Starting observability stack..."
    if [ -n "$REPO_URL" ] && [ -n "$RUNNER_TOKEN" ]; then
      echo "🏃 Starting with self-hosted runner..."
      docker compose -f deploy/docker-compose.yml --profile runner up -d
    else
      echo "📊 Starting without self-hosted runner (observability only)."
      echo "   To use the self-hosted runner, run: REPO_URL=... RUNNER_TOKEN=... ./run.sh up"
      docker compose -f deploy/docker-compose.yml up -d
    fi
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
