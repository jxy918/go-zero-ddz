@echo off
setlocal enabledelayedexpansion
cd /d "%~dp0"

echo ============================================
echo   go-zero-ddz Stop Script
echo ============================================
echo.

:: [1/3] Stop game-service
echo [1/3] Stopping game-service...

set "GAME_STOPPED=false"

taskkill /F /IM server.exe >nul 2>&1
if !errorlevel! equ 0 (
    echo   [OK] server.exe stopped
    set "GAME_STOPPED=true"
)

taskkill /F /FI "WINDOWTITLE eq game-service*" >nul 2>&1

for /f "tokens=5" %%a in ('netstat -aon ^| findstr ":8080 " ^| findstr "LISTENING"') do (
    taskkill /F /PID %%a >nul 2>&1
    if !errorlevel! equ 0 (
        echo   [OK] Killed process on port 8080 (PID: %%a)
        set "GAME_STOPPED=true"
    )
)

if "!GAME_STOPPED!"=="false" (
    echo   [INFO] game-service was not running
)

echo.

:: [2/3] Stop user-api
echo [2/3] Stopping user-api...

set "USER_STOPPED=false"

taskkill /F /IM user.exe >nul 2>&1
if !errorlevel! equ 0 (
    echo   [OK] user.exe stopped
    set "USER_STOPPED=true"
)

taskkill /F /FI "WINDOWTITLE eq user-api*" >nul 2>&1

for /f "tokens=5" %%a in ('netstat -aon ^| findstr ":8888 " ^| findstr "LISTENING"') do (
    taskkill /F /PID %%a >nul 2>&1
    if !errorlevel! equ 0 (
        echo   [OK] Killed process on port 8888 (PID: %%a)
        set "USER_STOPPED=true"
    )
)

if "!USER_STOPPED!"=="false" (
    echo   [INFO] user-api was not running
)

echo.

:: [3/3] Cleanup
echo [3/3] Cleanup...
echo   [OK] Done

echo.
echo ============================================
echo   Service Status
echo ============================================
echo.

if "!GAME_STOPPED!"=="true" (
    echo   [STOPPED] game-service
) else (
    echo   [NOT RUN] game-service
)

if "!USER_STOPPED!"=="true" (
    echo   [STOPPED] user-api
) else (
    echo   [NOT RUN] user-api
)

echo.
echo All services stopped.
echo.
timeout /t 2 /nobreak >nul
exit /b 0
