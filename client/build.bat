@echo off

REM Compile the Go program as a Windows GUI application
set GOOS=windows
set GOARCH=amd64
go build -ldflags "-H=windowsgui -s" -o client.stub