#!/bin/bash
# Demo script for GoClaw Control UI

echo "🎬 GoClaw Control UI Demo"
echo "=========================="
echo ""

# Check if binary exists
if [ ! -f "goclaw.exe" ]; then
    echo "❌ goclaw.exe not found. Building..."
    ./build-ui.sh
    if [ $? -ne 0 ]; then
        echo "❌ Build failed!"
        exit 1
    fi
fi

echo "✅ Binary ready"
echo ""

# Start the gateway
echo "🚀 Starting GoClaw Gateway..."
echo ""
./goclaw.exe gateway run --port 28789 &
GATEWAY_PID=$!

# Wait for server to start
echo "⏳ Waiting for server to start..."
sleep 3

# Check if server is running
if curl -s http://localhost:28789/health > /dev/null; then
    echo "✅ Gateway is running!"
    echo ""
    echo "📍 Access points:"
    echo "   • Control UI:    http://localhost:28789/"
    echo "   • WebSocket:     ws://localhost:28789/ws"
    echo "   • Health Check:  http://localhost:28789/health"
    echo "   • Channels API:  http://localhost:28789/api/channels"
    echo ""
    echo "🎯 Features:"
    echo "   ✅ Real-time WebSocket communication"
    echo "   ✅ Chat interface"
    echo "   ✅ Multi-view navigation"
    echo "   ✅ Auto-reconnect"
    echo "   ✅ Light/Dark theme"
    echo ""
    echo "🌐 Opening browser..."

    # Open browser based on OS
    if [[ "$OSTYPE" == "darwin"* ]]; then
        open http://localhost:28789/
    elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
        xdg-open http://localhost:28789/
    elif [[ "$OSTYPE" == "msys" ]] || [[ "$OSTYPE" == "cygwin" ]]; then
        start http://localhost:28789/
    fi

    echo ""
    echo "Press Ctrl+C to stop the gateway"

    # Wait for user interrupt
    trap "echo ''; echo '🛑 Stopping gateway...'; kill $GATEWAY_PID; exit 0" INT
    wait $GATEWAY_PID
else
    echo "❌ Failed to start gateway!"
    kill $GATEWAY_PID 2>/dev/null
    exit 1
fi
