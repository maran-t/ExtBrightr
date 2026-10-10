#!/bin/sh
# Builds ExtBrightness.exe (64-bit Windows). Needs Go 1.21+; cross-compiles from any OS.
# rsrc_windows_amd64.syso carries the icon and manifest; to regenerate it:
#   go install github.com/akavel/rsrc@latest
#   rsrc -manifest res/app.manifest -ico res/icon.ico -arch amd64 -o rsrc_windows_amd64.syso
set -e
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H=windowsgui -s -w" -o ExtBrightness.exe .
