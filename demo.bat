@echo off
REM Demo script for GoClaw Control UI (Windows)

echo 🎬 GoClaw Control UI Demo
echo ==========================
echo.

REM Check if binary exists
if not exist "goclaw.exe" (
    echo ❌ goclaw.exe not found. Building...
    call build-ui.bat
    if errorlevel 1 (
        echo ❌ Build failed!
        exit /b 1
    )
)

echo ✅ Binary ready
echo.

REM Start the gateway
echo 🚀 Starting GoClaw Gateway...
echo.
start /B goclaw.exe gateway run --port 28789

REM Wait for server to start
echo ⏳ Waiting for server to start...
timeout /t 3 /nobreak >nul

REM Check if server is running
curl -s http://localhost:28789/health >nul 2>&1
if %errorlevel% equ 0 (
    echo ✅ Gateway is running!
    echo.
    echo 📍 Access points:
    echo    • Control UI:    http://localhost:28789/
    echo    • WebSocket:     ws://localhost:28789/ws
    echo    • Health Check:  http://localhost:28789/health
    echo    • Channels API:  http://localhost:28789/api/channels
    echo.
    echo 🎯 Features:
    echo    ✅ Real-time WebSocket communication
    echo    ✅ Chat interface
    echo    ✅ Multi-view navigation
    echo    ✅ Auto-reconnect
    echo    ✅ Light/Dark theme
    echo.
    echo 🌐 Opening browser...
    start http://localhost:28789/
    echo.
    echo Press Ctrl+C to stop the gateway
    echo.
    pause
    taskkill /F /IM goclaw.exe >nul 2>&1
) else (
    echo ❌ Failed to start gateway!
    taskkill /F /IM goclaw.exe >nul 2>&1
    exit /b 1
)
