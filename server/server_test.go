package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerEndpoints(t *testing.T) {
	// 1. Test GetListeners initially empty
	req := httptest.NewRequest("GET", "/getListeners", nil)
	w := httptest.NewRecorder()
	handleGetListeners(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", w.Code)
	}

	var listenersResp map[string][]int
	if err := json.Unmarshal(w.Body.Bytes(), &listenersResp); err != nil {
		t.Fatalf("Failed to parse getListeners response: %v", err)
	}

	// 2. Test GetZombies initially empty
	req = httptest.NewRequest("GET", "/getZombies", nil)
	w = httptest.NewRecorder()
	handleGetZombies(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", w.Code)
	}

	var zombiesResp []*Zombie
	if err := json.Unmarshal(w.Body.Bytes(), &zombiesResp); err != nil {
		t.Fatalf("Failed to parse getZombies response: %v", err)
	}
	if len(zombiesResp) != 0 {
		t.Errorf("Expected 0 zombies, got %d", len(zombiesResp))
	}

	// 3. Test NewListener with valid port
	body, _ := json.Marshal(map[string]int{"port": 15432})
	req = httptest.NewRequest("POST", "/newListener", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	handleNewListener(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	// 4. Test StopListener
	body, _ = json.Marshal(map[string]int{"port": 15432})
	req = httptest.NewRequest("POST", "/stopListener", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	handleStopListener(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK on stopListener, got %d: %s", w.Code, w.Body.String())
	}
}
