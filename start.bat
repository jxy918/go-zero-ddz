@echo off
setlocal enabledelayedexpansion
cd /d "%~dp0"

echo ============================================
echo   go-zero-ddz Start Script
echo ============================================
echo.

:: [1/4] Check ports
echo [1/4] Checking ports...

set "PORT_8080_FREE=true"
set "PORT_8888_FREE=true"

for /f "tokens=5" %%a in ('netstat -aon ^| findstr ":8080 " ^| findstr "LISTENING"') do (
    set "PORT_8080_FREE=false"
    set "PID_8080=%%a"
)

for /f "tokens=5" %%a in ('netstat -aon ^| findstr ":8888 " ^| findstr "LISTENING"') do (
    set "PORT_8888_FREE=false"
    set "PID_8888=%%a"
)

if "!PORT_8080_FREE!"=="false" (
    echo   [ERROR] Port 8080 in use by PID: !PID_8080!
    echo   Please run stop.bat first.
    goto :error_exit
)
if "!PORT_8888_FREE!"=="false" (
    echo   [ERROR] Port 8888 in use by PID: !PID_8888!
    echo   Please run stop.bat first.
    goto :error_exit
)

echo   [OK] Port 8080 available
echo   [OK] Port 8888 available
echo.

:: [2/4] Build binaries
echo [2/4] Building binaries...

echo   Building game-service ...
go build -o server.exe ./app/game/cmd/server/
if !errorlevel! neq 0 (
    echo   [ERROR] game-service build failed
    goto :error_exit
)
echo   [OK] server.exe built

echo   Building user-api ...
go build -o user.exe ./app/user/
if !errorlevel! neq 0 (
    echo   [ERROR] user-api build failed
    goto :error_exit
)
echo   [OK] user.exe built
echo.

:: [3/4] Start services
echo [3/4] Starting services...

echo   Starting game-service (port 8080)...
start "game-service" /MIN server.exe -f app/game/etc/game-local.yaml
timeout /t 2 /nobreak >nul
echo   [OK] game-service started

echo   Starting user-api (port 8888)...
start "user-api" /MIN user.exe -f app/user/etc/user-api.yaml
timeout /t 2 /nobreak >nul
echo   [OK] user-api started
echo.

:: [4/4] Wait for services
echo [4/4] Waiting for services ready...

set "GAME_READY=false"
set "USER_READY=false"

for /l %%i in (1,1,10) do (
    timeout /t 1 /nobreak >nul
    if "!GAME_READY!"=="false" (
        for /f "tokens=5" %%a in ('netstat -aon ^| findstr ":8080 " ^| findstr "LISTENING"') do (
            set "GAME_READY=true"
        )
    )
    if "!USER_READY!"=="false" (
        for /f "tokens=5" %%a in ('netstat -aon ^| findstr ":8888 " ^| findstr "LISTENING"') do (
            set "USER_READY=true"
        )
    )
    if "!GAME_READY!"=="true" if "!USER_READY!"=="true" goto :services_ready
)

:services_ready

echo.
echo ============================================
echo   Service Status
echo ============================================
echo.

if "!GAME_READY!"=="true" (
    echo   [RUNNING] game-service
    echo     Web:       http://localhost:8080/
    echo     WebSocket: ws://localhost:8080/ws
) else (
    echo   [STARTING] game-service (port 8080 not ready yet)
)

if "!USER_READY!"=="true" (
    echo   [RUNNING] user-api
    echo     API:       http://localhost:8888/
) else (
    echo   [STARTING] user-api (port 8888 not ready yet)
)

echo.
echo Tip: run stop.bat to stop all services
echo.
exit /b 0

:error_exit
echo.
echo [FAILED] Please check error messages above
echo.
pause
exit /b 1
