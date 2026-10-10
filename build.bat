@echo off
rem Builds ExtBrightness.exe (64-bit Windows). Needs Go 1.21 or newer.
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
go build -trimpath -ldflags "-H=windowsgui -s -w" -o ExtBrightness.exe .
