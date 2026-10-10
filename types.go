package main

// Win32 structures used by the app. They live in an untagged file so their
// layouts can be checked by `go test` on any 64-bit platform.

type point struct{ x, y int32 }

type rect struct{ left, top, right, bottom int32 }

type msgT struct {
	hwnd     uintptr
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	pt       point
	lPrivate uint32
}

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type paintStruct struct {
	hdc         uintptr
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  uintptr
	hbmColor uintptr
}

type guid struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

type notifyIconData struct {
	cbSize           uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         guid
	hBalloonIcon     uintptr
}

type notifyIconIdentifier struct {
	cbSize   uint32
	hWnd     uintptr
	uID      uint32
	guidItem guid
}

type monitorInfoEx struct {
	cbSize    uint32
	rcMonitor rect
	rcWork    rect
	dwFlags   uint32
	szDevice  [32]uint16
}

type trackMouseEventT struct {
	cbSize      uint32
	dwFlags     uint32
	hwndTrack   uintptr
	dwHoverTime uint32
}

type osVersionInfo struct {
	dwOSVersionInfoSize uint32
	dwMajorVersion      uint32
	dwMinorVersion      uint32
	dwBuildNumber       uint32
	dwPlatformId        uint32
	szCSDVersion        [128]uint16
}

// PHYSICAL_MONITOR (physicalmonitorenumerationapi.h, packed).
type physicalMonitor struct {
	handle uintptr
	desc   [128]uint16
}

// --- QueryDisplayConfig family -------------------------------------------

type luid struct {
	lowPart  uint32
	highPart int32
}

type dcPathSource struct {
	adapterID   luid
	id          uint32
	modeInfoIdx uint32
	statusFlags uint32
}

type dcRational struct{ numerator, denominator uint32 }

type dcPathTarget struct {
	adapterID        luid
	id               uint32
	modeInfoIdx      uint32
	outputTechnology uint32
	rotation         uint32
	scaling          uint32
	refreshRate      dcRational
	scanLineOrdering uint32
	targetAvailable  int32
	statusFlags      uint32
}

type dcPathInfo struct {
	source dcPathSource
	target dcPathTarget
	flags  uint32
}

// DISPLAYCONFIG_MODE_INFO is only ever passed through as an opaque buffer.
type dcModeInfo [64]byte

type dcHeader struct {
	typ       uint32
	size      uint32
	adapterID luid
	id        uint32
}

type dcSourceName struct {
	header            dcHeader
	viewGdiDeviceName [32]uint16
}

type dcTargetName struct {
	header            dcHeader
	flags             uint32
	outputTechnology  uint32
	edidManufactureID uint16
	edidProductCodeID uint16
	connectorInstance uint32
	friendlyName      [64]uint16
	devicePath        [128]uint16
}

// Output technologies that mean "this is the built-in panel".
const (
	outTechLVDS           = 6
	outTechDPEmbedded     = 11
	outTechUDIEmbedded    = 13
	outTechInternal       = 0x80000000
	dcInfoGetSourceName   = 1
	dcInfoGetTargetName   = 2
	qdcOnlyActivePaths    = 2
	errInsufficientBuffer = 122
)

func isInternalTech(t uint32) bool {
	switch t {
	case outTechInternal, outTechLVDS, outTechDPEmbedded, outTechUDIEmbedded:
		return true
	}
	return false
}
