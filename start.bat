@echo off
cls
echo ======================================
echo     DDZ Game Service Start Script
echo ======================================
echo.

cd /d "%~dp0"

echo [1/2] Starting game-service (port: 8080)...
start "game-service" /MIN go run ./app/game/cmd/server/ -f app/game/etc/game-local.yaml

timeout /t 3 /nobreak >nul

echo [2/2] Starting user-api (port: 8888)...
start "user-api" /MIN go run ./app/user/ -f app/user/etc/user-api.yaml

timeout /t 2 /nobreak >nul

echo.
echo ======================================
echo     Services started successfully!
echo ======================================
echo.
echo Service addresses:
echo   Web Frontend:   http://localhost:8080/
echo   User API:       http://localhost:8888/
echo   WebSocket:      ws://localhost:8080/ws
echo.

exit /b 0