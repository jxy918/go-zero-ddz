#!/bin/bash

# ============================================
#   go-zero-ddz Start Script
# ============================================

cd "$(dirname "$0")"

echo "============================================"
echo "  go-zero-ddz Start Script"
echo "============================================"
echo ""

# [1/4] Check ports
echo "[1/4] Checking ports..."

PORT_8080_PID=$(lsof -ti :8080 2>/dev/null || ss -tlnp "sport = :8080" 2>/dev/null | grep -oP 'pid=\K[0-9]+' | head -1 || echo "")
PORT_8888_PID=$(lsof -ti :8888 2>/dev/null || ss -tlnp "sport = :8888" 2>/dev/null | grep -oP 'pid=\K[0-9]+' | head -1 || echo "")

if [ -n "$PORT_8080_PID" ]; then
    echo "  [ERROR] Port 8080 in use by PID: $PORT_8080_PID"
    echo "  Please run ./stop.sh first."
    exit 1
fi

if [ -n "$PORT_8888_PID" ]; then
    echo "  [ERROR] Port 8888 in use by PID: $PORT_8888_PID"
    echo "  Please run ./stop.sh first."
    exit 1
fi

echo "  [OK] Port 8080 available"
echo "  [OK] Port 8888 available"
echo ""

# [2/4] Build binaries
echo "[2/4] Building binaries..."

echo "  Building game-service ..."
if ! go build -o server ./app/game/cmd/server/; then
    echo "  [ERROR] game-service build failed"
    exit 1
fi
echo "  [OK] server built"

echo "  Building user-api ..."
if ! go build -o user ./app/user/; then
    echo "  [ERROR] user-api build failed"
    exit 1
fi
echo "  [OK] user built"
echo ""

# [3/4] Start services
echo "[3/4] Starting services..."

echo "  Starting game-service (port 8080)..."
./server -f app/game/etc/game-local.yaml > game-service.log 2>&1 &
GAME_PID=$!
echo $GAME_PID > .game.pid
sleep 2
echo "  [OK] game-service started (PID: $GAME_PID)"

echo "  Starting user-api (port 8888)..."
./user -f app/user/etc/user-api.yaml > user-api.log 2>&1 &
USER_PID=$!
echo $USER_PID > .user.pid
sleep 2
echo "  [OK] user-api started (PID: $USER_PID)"
echo ""

# [4/4] Wait for services
echo "[4/4] Waiting for services ready..."

GAME_READY=false
USER_READY=false

for i in {1..10}; do
    sleep 1
    
    if [ "$GAME_READY" = false ]; then
        if lsof -ti :8080 >/dev/null 2>&1 || ss -tln "sport = :8080" 2>/dev/null | grep -q LISTEN; then
            GAME_READY=true
        fi
    fi
    
    if [ "$USER_READY" = false ]; then
        if lsof -ti :8888 >/dev/null 2>&1 || ss -tln "sport = :8888" 2>/dev/null | grep -q LISTEN; then
            USER_READY=true
        fi
    fi
    
    if [ "$GAME_READY" = true ] && [ "$USER_READY" = true ]; then
        break
    fi
done

echo ""
echo "============================================"
echo "  Service Status"
echo "============================================"
echo ""

if [ "$GAME_READY" = true ]; then
    echo "  [RUNNING] game-service (PID: $GAME_PID)"
    echo "    Web:       http://localhost:8080/"
    echo "    WebSocket: ws://localhost:8080/ws"
    echo "    Logs:      game-service.log"
else
    echo "  [STARTING] game-service (port 8080 not ready yet)"
fi

if [ "$USER_READY" = true ]; then
    echo "  [RUNNING] user-api (PID: $USER_PID)"
    echo "    API:       http://localhost:8888/"
    echo "    Logs:      user-api.log"
else
    echo "  [STARTING] user-api (port 8888 not ready yet)"
fi

echo ""
echo "Tip: run ./stop.sh to stop all services"
echo ""

exit 0
