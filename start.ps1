Write-Host "======================================"
Write-Host "    DDZ Game Service Start Script"
Write-Host "======================================"
Write-Host ""

Set-Location $PSScriptRoot

Write-Host "[1/2] Starting game-service (port: 8080)..."
Start-Process -FilePath "go" -ArgumentList "run", "./app/game/cmd/server/", "-f", "app/game/etc/game-local.yaml" -WindowStyle Hidden

Start-Sleep -Seconds 3

Write-Host "[2/2] Starting user-api (port: 8888)..."
Start-Process -FilePath "go" -ArgumentList "run", "./app/user/", "-f", "app/user/etc/user-api.yaml" -WindowStyle Hidden

Start-Sleep -Seconds 2

Write-Host ""
Write-Host "======================================"
Write-Host "    Services started successfully!"
Write-Host "======================================"
Write-Host ""
Write-Host "Service addresses:"
Write-Host "  Web Frontend:   http://localhost:8080/"
Write-Host "  User API:       http://localhost:8888/"
Write-Host "  WebSocket:      ws://localhost:8080/ws"
Write-Host ""