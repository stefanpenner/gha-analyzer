#!/bin/bash

set -e

COMMAND=$1
SHIFT_ARGS=${@:2}

case $COMMAND in
  up)
    echo "🚀 Starting observability stack..."
    cd deploy && docker-compose up -d
    echo "✅ Stack is up! Grafana: http://localhost:3000"
    ;;
  down)
    echo "🛑 Stopping observability stack..."
    cd deploy && docker-compose down
    ;;
  cli)
    echo "🔍 Running one-off analysis..."
    go run cmd/gha-analyzer/main.go $SHIFT_ARGS
    ;;
  server)
    echo "📡 Starting webhook server..."
    go run cmd/gha-server/main.go
    ;;
  dashboard)
    open http://localhost:3000
    ;;
  *)
    echo "Usage: ./run.sh {up|down|cli|server|dashboard}"
    exit 1
    ;;
esac
