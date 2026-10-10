package main

import (
	"testing"
	"unsafe"
)

// Sizes and key offsets as laid out by the Windows SDK for x64.
func TestStructLayouts(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("layout check is for 64-bit targets")
	}
	check := func(name string, got, want uintptr) {
		if got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
	}
	check("MSG", unsafe.Sizeof(msgT{}), 48)
	check("WNDCLASSEXW", unsafe.Sizeof(wndClassEx{}), 80)
	check("PAINTSTRUCT", unsafe.Sizeof(paintStruct{}), 72)
	check("BITMAPINFOHEADER", unsafe.Sizeof(bitmapInfoHeader{}), 40)
	check("ICONINFO", unsafe.Sizeof(iconInfo{}), 32)
	check("NOTIFYICONDATAW", unsafe.Sizeof(notifyIconData{}), 976)
	check("NOTIFYICONDATAW.szTip", unsafe.Offsetof(notifyIconData{}.szTip), 40)
	check("NOTIFYICONDATAW.guidItem", unsafe.Offsetof(notifyIconData{}.guidItem), 952)
	check("NOTIFYICONIDENTIFIER", unsafe.Sizeof(notifyIconIdentifier{}), 40)
	check("MONITORINFOEXW", unsafe.Sizeof(monitorInfoEx{}), 104)
	check("TRACKMOUSEEVENT", unsafe.Sizeof(trackMouseEventT{}), 24)
	check("OSVERSIONINFOW", unsafe.Sizeof(osVersionInfo{}), 276)
	check("PHYSICAL_MONITOR", unsafe.Sizeof(physicalMonitor{}), 264)
	check("DISPLAYCONFIG_PATH_SOURCE_INFO", unsafe.Sizeof(dcPathSource{}), 20)
	check("DISPLAYCONFIG_PATH_TARGET_INFO", unsafe.Sizeof(dcPathTarget{}), 48)
	check("DISPLAYCONFIG_PATH_INFO", unsafe.Sizeof(dcPathInfo{}), 72)
	check("DISPLAYCONFIG_PATH_INFO.target", unsafe.Offsetof(dcPathInfo{}.target), 20)
	check("DISPLAYCONFIG_MODE_INFO", unsafe.Sizeof(dcModeInfo{}), 64)
	check("DISPLAYCONFIG_DEVICE_INFO_HEADER", unsafe.Sizeof(dcHeader{}), 20)
	check("DISPLAYCONFIG_SOURCE_DEVICE_NAME", unsafe.Sizeof(dcSourceName{}), 84)
	check("DISPLAYCONFIG_TARGET_DEVICE_NAME", unsafe.Sizeof(dcTargetName{}), 420)
	check("DISPLAYCONFIG_TARGET_DEVICE_NAME.friendlyName", unsafe.Offsetof(dcTargetName{}.friendlyName), 36)
}

func TestSliderMapping(t *testing.T) {
	for _, s := range []float64{1, 1.25, 1.5, 2} {
		g := rowGeometry(0, s)
		for pct := 0; pct <= 100; pct++ {
			if got := pctFromX(g, thumbX(g, pct)); got != pct {
				t.Fatalf("scale %.2f: pct %d round-trips to %d", s, pct, got)
			}
		}
		if pctFromX(g, -50) != 0 || pctFromX(g, 1e6) != 100 {
			t.Fatalf("scale %.2f: out-of-range x not clamped", s)
		}
	}
}
