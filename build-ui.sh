#!/bin/bash
# Build script for GoClaw Control UI

set -e

echo "🎨 Building GoClaw Control UI..."

# Build UI
echo "📦 Building frontend..."
cd ui
npm run build
cd ..

# Copy to gateway
echo "📋 Copying to gateway..."
rm -rf gateway/dist/control-ui
mkdir -p gateway/dist
cp -r dist/control-ui gateway/dist/

# Build Go binary
echo "🔨 Building Go binary..."
go build -o goclaw.exe .

echo "✅ Build complete!"
echo ""
echo "To run the gateway with UI:"
echo "  ./goclaw.exe gateway run --port 28789"
echo ""
echo "Then open: http://localhost:28789/"
