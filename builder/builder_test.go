package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"testing"
)

func TestPayloadConfigInjection(t *testing.T) {
	dummyStub := []byte("MZ_DUMMY_EXECUTABLE_CONTENT_1234567890")
	testPort := 8080
	testIP := "192.168.1.100"

	parsedIP := net.ParseIP(testIP)
	if parsedIP == nil || parsedIP.To4() == nil {
		t.Fatalf("Failed to parse test IP")
	}
	ipv4 := parsedIP.To4()

	// Inject 6-byte config
	config := make([]byte, 6)
	binary.LittleEndian.PutUint16(config[0:2], uint16(testPort))
	copy(config[2:6], ipv4)

	payload := append(dummyStub, config...)

	// Simulate client reading from payload
	if len(payload) < 6 {
		t.Fatalf("Payload too short")
	}

	startIndex := len(payload) - 6
	extractedPort := int(binary.LittleEndian.Uint16(payload[startIndex : startIndex+2]))
	ipBytes := payload[startIndex+2 : startIndex+6]
	extractedIP := fmt.Sprintf("%d.%d.%d.%d", ipBytes[0], ipBytes[1], ipBytes[2], ipBytes[3])

	if extractedPort != testPort {
		t.Errorf("Expected port %d, got %d", testPort, extractedPort)
	}

	if extractedIP != testIP {
		t.Errorf("Expected IP %s, got %s", testIP, extractedIP)
	}

	// Verify stub integrity is preserved
	if !bytes.Equal(payload[:startIndex], dummyStub) {
		t.Errorf("Stub prefix was modified during injection")
	}
}
