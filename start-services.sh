#!/bin/bash
set -e

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$PROJECT_DIR"

# Load environment variables
if [ -f .env ]; then
  export $(cat .env | grep -v '^#' | xargs)
fi

echo "🚀 Starting VPN microservices..."

# 1. Start Podman containers (PostgreSQL + sing-box)
echo "📦 Starting Podman containers..."
podman-compose -f compose.yaml up -d

echo "⏳ Waiting for PostgreSQL to be ready..."
sleep 3
until pg_isready -h 127.0.0.1 -p 5432 -U master_user 2>/dev/null; do
  echo "Waiting for PostgreSQL..."
  sleep 1
done

echo "✅ PostgreSQL is ready"
echo "✅ sing-box worker is running"

# Initialize test node in database
echo "📝 Initializing test node in database..."
NODE_TOKEN="${NODE_JOIN_TOKEN:-test-join-token-123}"
podman exec sim_db psql -U master_user -d vpn_core -c \
  "INSERT INTO nodes (name, address, node_type, status, join_token) \
   VALUES ('test-worker-1', '127.0.0.1:8444', 'reality', 'active', '$NODE_TOKEN') \
   ON CONFLICT (join_token) DO NOTHING;"

# 2. Start Rust vpn-core
echo "🦀 Starting vpn-core (Rust)..."
cd "$PROJECT_DIR/vpn-core"
cargo build --release 2>&1 | grep -E "(Compiling|Finished|error)" &
CORE_PID=$!
cd "$PROJECT_DIR"

# Wait for Core to finish building
wait $CORE_PID 2>/dev/null || true

# Start Core if build succeeded
if [ -f "$PROJECT_DIR/vpn-core/target/release/vpn-core" ] || [ -f "$PROJECT_DIR/vpn-core/target/debug/vpn-core" ]; then
  cd "$PROJECT_DIR/vpn-core"
  cargo run --release &
  CORE_RUN_PID=$!
  cd "$PROJECT_DIR"
  echo "✅ vpn-core is running (PID: $CORE_RUN_PID)"

  # Wait for vpn-core to start listening
  echo "⏳ Waiting for vpn-core to start..."
  sleep 3
  until nc -z 127.0.0.1 8443 2>/dev/null; do
    echo "Waiting for vpn-core on port 8443..."
    sleep 1
  done
  echo "✅ vpn-core is listening"
fi

# 3. Start Go services (bot, nup) - только после vpn-core
echo "🐹 Starting bot (Go)..."
cd "$PROJECT_DIR/bot"
go run . &
BOT_PID=$!
cd "$PROJECT_DIR"

echo "🐹 Starting nup (Go)..."
cd "$PROJECT_DIR/nup"
NUP_CONFIG_PATH="$PROJECT_DIR/sim_etc_singbox/config.json" go run . &
NUP_PID=$!
cd "$PROJECT_DIR"

echo ""
echo "=================================="
echo "✅ All services started!"
echo "=================================="
echo ""
echo "Services running:"
echo "  - PostgreSQL:   127.0.0.1:5432 (in podman)"
echo "  - sing-box:     127.0.0.1:2026 (in podman)"
echo "  - vpn-core:     (Rust, check logs)"
echo "  - bot:          (Go, PID: $BOT_PID)"
echo "  - nup:          (Go, PID: $NUP_PID)"
echo ""
echo "Stop services: kill $BOT_PID $NUP_PID && podman-compose -f compose.yaml down"
echo ""

wait
