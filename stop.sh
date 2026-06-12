#!/bin/bash

# ============================================
#   go-zero-ddz Stop Script
# ============================================

cd "$(dirname "$0")"

echo "============================================"
echo "  go-zero-ddz Stop Script"
echo "============================================"
echo ""

# [1/3] Stop game-service
echo "[1/3] Stopping game-service..."

GAME_STOPPED=false

# Try PID file
if [ -f .game.pid ]; then
    GAME_PID=$(cat .game.pid)
    if kill -0 "$GAME_PID" 2>/dev/null; then
        kill "$GAME_PID" 2>/dev/null
        sleep 1
        if kill -0 "$GAME_PID" 2>/dev/null; then
            kill -9 "$GAME_PID" 2>/dev/null
        fi
        echo "  [OK] server stopped (PID: $GAME_PID)"
        GAME_STOPPED=true
    fi
    rm -f .game.pid
fi

# Try process name
if [ "$GAME_STOPPED" = false ]; then
    if pkill -f "./server" 2>/dev/null; then
        echo "  [OK] server process stopped"
        GAME_STOPPED=true
    fi
fi

# Try port
if [ "$GAME_STOPPED" = false ]; then
    PORT_PID=$(lsof -ti :8080 2>/dev/null || ss -tlnp "sport = :8080" 2>/dev/null | grep -oP 'pid=\K[0-9]+' | head -1 || echo "")
    if [ -n "$PORT_PID" ]; then
        kill "$PORT_PID" 2>/dev/null
        sleep 1
        if kill -0 "$PORT_PID" 2>/dev/null; then
            kill -9 "$PORT_PID" 2>/dev/null
        fi
        echo "  [OK] Killed process on port 8080 (PID: $PORT_PID)"
        GAME_STOPPED=true
    fi
fi

if [ "$GAME_STOPPED" = false ]; then
    echo "  [INFO] game-service was not running"
fi

echo ""

# [2/3] Stop user-api
echo "[2/3] Stopping user-api..."

USER_STOPPED=false

# Try PID file
if [ -f .user.pid ]; then
    USER_PID=$(cat .user.pid)
    if kill -0 "$USER_PID" 2>/dev/null; then
        kill "$USER_PID" 2>/dev/null
        sleep 1
        if kill -0 "$USER_PID" 2>/dev/null; then
            kill -9 "$USER_PID" 2>/dev/null
        fi
        echo "  [OK] user stopped (PID: $USER_PID)"
        USER_STOPPED=true
    fi
    rm -f .user.pid
fi

# Try process name
if [ "$USER_STOPPED" = false ]; then
    if pkill -f "./user" 2>/dev/null; then
        echo "  [OK] user process stopped"
        USER_STOPPED=true
    fi
fi

# Try port
if [ "$USER_STOPPED" = false ]; then
    PORT_PID=$(lsof -ti :8888 2>/dev/null || ss -tlnp "sport = :8888" 2>/dev/null | grep -oP 'pid=\K[0-9]+' | head -1 || echo "")
    if [ -n "$PORT_PID" ]; then
        kill "$PORT_PID" 2>/dev/null
        sleep 1
        if kill -0 "$PORT_PID" 2>/dev/null; then
            kill -9 "$PORT_PID" 2>/dev/null
        fi
        echo "  [OK] Killed process on port 8888 (PID: $PORT_PID)"
        USER_STOPPED=true
    fi
fi

if [ "$USER_STOPPED" = false ]; then
    echo "  [INFO] user-api was not running"
fi

echo ""

# [3/3] Cleanup
echo "[3/3] Cleanup..."

# Cleanup go run processes
pkill -f "go run.*game" 2>/dev/null
pkill -f "go run.*user" 2>/dev/null

echo "  [OK] Done"

echo ""
echo "============================================"
echo "  Service Status"
echo "============================================"
echo ""

if [ "$GAME_STOPPED" = true ]; then
    echo "  [STOPPED] game-service"
else
    echo "  [NOT RUN] game-service"
fi

if [ "$USER_STOPPED" = true ]; then
    echo "  [STOPPED] user-api"
else
    echo "  [NOT RUN] user-api"
fi

echo ""
echo "All services stopped."
echo ""

exit 0
