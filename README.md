# ExtBrightness

A tray icon with a brightness slider for external monitors on Windows 10/11.
The laptop's built-in panel is never listed.

- Left-click the sun icon: one slider per external monitor.
  Drag, scroll the wheel (5% per notch), or use the keyboard:
  Left/Right 5%, Shift+Left/Right 1%, PgUp/PgDn 20%, Home/End, Up/Down to switch monitor, Esc to close.
- Right-click: Start with Windows, Rescan monitors, Exit.

## How it works

External monitors take brightness commands over DDC/CI (VCP feature 0x10), sent
with `SetVCPFeature` from `dxva2.dll`. The built-in panel is filtered out by its
connector type from `QueryDisplayConfig` (internal / embedded DisplayPort / LVDS).
Anything that does not answer DDC/CI is skipped too.

If the flyout says no monitor was found, switch DDC/CI on in the monitor's
on-screen menu. Some USB-C docks and KVM switches do not pass DDC/CI through.

## Build

Needs Go 1.21 or newer, nothing else (no cgo, no third-party modules).

    build.bat        on Windows
    ./build.sh       cross-compile from Linux or macOS

`go test` checks the Win32 struct layouts and the slider maths.
`ExtBrightness.exe --demo` runs with two pretend monitors.

## Files

- `main.go`    tray icon, flyout window, input
- `ddc.go`     monitor detection and DDC/CI worker
- `render.go`  drawing of the flyout and tray glyph
- `win.go`     Win32 bindings, registry, theme
- `types.go`   Win32 structures
- `res/`       manifest and icon (compiled into `rsrc_windows_amd64.syso`)
