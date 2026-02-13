# goclaw restart script with Control UI
Write-Host "🔄 Restarting goclaw..." -ForegroundColor Cyan

# Stop existing process
Write-Host "Stopping goclaw..."
$proc = Get-Process -Name "goclaw" -ErrorAction SilentlyContinue
if ($proc) {
    Stop-Process -Name "goclaw" -Force
    Start-Sleep 2
    Write-Host "✓ Stopped" -ForegroundColor Green
}

# Pull latest code
Write-Host "Pulling latest code..."
$scriptPath = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $scriptPath
git pull
if ($LASTEXITCODE -eq 0) {
    Write-Host "✓ Code updated" -ForegroundColor Green
} else {
    Write-Host "⚠ Git pull failed, continuing with existing code..." -ForegroundColor Yellow
}

# Build UI
Write-Host "Building Control UI..."
if (Test-Path ".\build-ui.bat") {
    & .\build-ui.bat
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✓ UI built" -ForegroundColor Green
    } else {
        Write-Host "⚠ UI build failed, using existing build..." -ForegroundColor Yellow
    }
} else {
    Write-Host "⚠ build-ui.bat not found, skipping UI build" -ForegroundColor Yellow
}

# Build Go binary
Write-Host "Building Go binary..."
$env:Path = "C:\Go\bin;$env:Path"
go build -o goclaw.exe .
if ($LASTEXITCODE -eq 0) {
    Write-Host "✓ Binary built" -ForegroundColor Green
} else {
    Write-Host "✗ Build failed!" -ForegroundColor Red
    exit 1
}

# Start gateway with Control UI
Write-Host "Starting gateway with Control UI..."
Start-Process -FilePath ".\goclaw.exe" -ArgumentList "gateway", "run",  -WindowStyle Hidden
Start-Sleep 3

# Check if started
$proc = Get-Process -Name "goclaw" -ErrorAction SilentlyContinue
if ($proc) {
    Write-Host "✓ Started (PID: $($proc.Id))" -ForegroundColor Green
    Write-Host ""
    Write-Host "📍 Access points:" -ForegroundColor Cyan
    Write-Host "   • Control UI:    http://localhost:28789/" -ForegroundColor White
    Write-Host "   • WebSocket:     ws://localhost:28789/ws" -ForegroundColor White
    Write-Host "   • Health Check:  http://localhost:28789/health" -ForegroundColor White
    Write-Host "   • Channels API:  http://localhost:28789/api/channels" -ForegroundColor White
} else {
    Write-Host "✗ Failed to start!" -ForegroundColor Red
    exit 1
}
