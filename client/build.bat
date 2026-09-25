@echo off

REM Compile the Go program as a Windows GUI application (stripped symbols)
set GOOS=windows
set GOARCH=amd64
go build -ldflags "-H=windowsgui -s -w" -o client.stub .
echo Client stub compiled: client.stub