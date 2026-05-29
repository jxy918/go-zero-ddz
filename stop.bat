@echo off
cls
echo ======================================
echo     DDZ Game Service Stop Script
echo ======================================
echo.

echo [1/2] Stopping game-service...
taskkill /F /IM server.exe >nul 2>&1
if %errorlevel% equ 0 (
    echo      OK - game-service stopped
) else (
    echo      INFO - game-service not running
)

echo [2/2] Stopping user-api...
taskkill /F /IM user.exe >nul 2>&1
if %errorlevel% equ 0 (
    echo      OK - user-api stopped
) else (
    echo      INFO - user-api not running
)

echo.
echo ======================================
echo     Services stopped successfully!
echo ======================================
echo.
echo Press any key to exit...
timeout /t 1 /nobreak >nul