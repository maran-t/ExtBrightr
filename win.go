//go:build windows

package main

import (
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")
	dxva2    = syscall.NewLazyDLL("dxva2.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")
	shcore   = syscall.NewLazyDLL("shcore.dll")
	ntdll    = syscall.NewLazyDLL("ntdll.dll")
	uxtheme  = syscall.NewLazyDLL("uxtheme.dll")

	pSetDefaultDllDirectories = kernel32.NewProc("SetDefaultDllDirectories")
	pGetModuleHandleW         = kernel32.NewProc("GetModuleHandleW")
	pCreateMutexW             = kernel32.NewProc("CreateMutexW")
	pGetTickCount64           = kernel32.NewProc("GetTickCount64")
	pGetProcAddress           = kernel32.NewProc("GetProcAddress")
	pQueryPerfCounter         = kernel32.NewProc("QueryPerformanceCounter")
	pQueryPerfFrequency       = kernel32.NewProc("QueryPerformanceFrequency")

	pRegisterClassExW              = user32.NewProc("RegisterClassExW")
	pCreateWindowExW               = user32.NewProc("CreateWindowExW")
	pDefWindowProcW                = user32.NewProc("DefWindowProcW")
	pDestroyWindow                 = user32.NewProc("DestroyWindow")
	pGetMessageW                   = user32.NewProc("GetMessageW")
	pTranslateMessage              = user32.NewProc("TranslateMessage")
	pDispatchMessageW              = user32.NewProc("DispatchMessageW")
	pPostMessageW                  = user32.NewProc("PostMessageW")
	pPostQuitMessage               = user32.NewProc("PostQuitMessage")
	pRegisterWindowMessageW        = user32.NewProc("RegisterWindowMessageW")
	pLoadCursorW                   = user32.NewProc("LoadCursorW")
	pShowWindow                    = user32.NewProc("ShowWindow")
	pSetWindowPos                  = user32.NewProc("SetWindowPos")
	pSetForegroundWindow           = user32.NewProc("SetForegroundWindow")
	pSetFocus                      = user32.NewProc("SetFocus")
	pInvalidateRect                = user32.NewProc("InvalidateRect")
	pUpdateWindow                  = user32.NewProc("UpdateWindow")
	pBeginPaint                    = user32.NewProc("BeginPaint")
	pEndPaint                      = user32.NewProc("EndPaint")
	pGetClientRect                 = user32.NewProc("GetClientRect")
	pGetCursorPos                  = user32.NewProc("GetCursorPos")
	pScreenToClient                = user32.NewProc("ScreenToClient")
	pSetCapture                    = user32.NewProc("SetCapture")
	pReleaseCapture                = user32.NewProc("ReleaseCapture")
	pTrackMouseEvent               = user32.NewProc("TrackMouseEvent")
	pGetKeyState                   = user32.NewProc("GetKeyState")
	pSetTimer                      = user32.NewProc("SetTimer")
	pKillTimer                     = user32.NewProc("KillTimer")
	pCreatePopupMenu               = user32.NewProc("CreatePopupMenu")
	pAppendMenuW                   = user32.NewProc("AppendMenuW")
	pTrackPopupMenu                = user32.NewProc("TrackPopupMenu")
	pDestroyMenu                   = user32.NewProc("DestroyMenu")
	pCreateIconIndirect            = user32.NewProc("CreateIconIndirect")
	pDestroyIcon                   = user32.NewProc("DestroyIcon")
	pGetSystemMetrics              = user32.NewProc("GetSystemMetrics")
	pMonitorFromPoint              = user32.NewProc("MonitorFromPoint")
	pGetMonitorInfoW               = user32.NewProc("GetMonitorInfoW")
	pEnumDisplayMonitors           = user32.NewProc("EnumDisplayMonitors")
	pGetDC                         = user32.NewProc("GetDC")
	pReleaseDC                     = user32.NewProc("ReleaseDC")
	pDrawTextW                     = user32.NewProc("DrawTextW")
	pSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	pSetProcessDPIAware            = user32.NewProc("SetProcessDPIAware")
	pGetDisplayConfigBufferSizes   = user32.NewProc("GetDisplayConfigBufferSizes")
	pQueryDisplayConfig            = user32.NewProc("QueryDisplayConfig")
	pDisplayConfigGetDeviceInfo    = user32.NewProc("DisplayConfigGetDeviceInfo")
	pWindowFromPoint               = user32.NewProc("WindowFromPoint")
	pGetAncestor                   = user32.NewProc("GetAncestor")
	pGetClassNameW                 = user32.NewProc("GetClassNameW")
	pSystemParametersInfoW         = user32.NewProc("SystemParametersInfoW")

	pCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	pCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	pCreateBitmap       = gdi32.NewProc("CreateBitmap")
	pSelectObject       = gdi32.NewProc("SelectObject")
	pDeleteObject       = gdi32.NewProc("DeleteObject")
	pDeleteDC           = gdi32.NewProc("DeleteDC")
	pBitBlt             = gdi32.NewProc("BitBlt")
	pCreateFontW        = gdi32.NewProc("CreateFontW")
	pSetBkMode          = gdi32.NewProc("SetBkMode")
	pSetTextColor       = gdi32.NewProc("SetTextColor")
	pGetTextFaceW       = gdi32.NewProc("GetTextFaceW")
	pGdiFlush           = gdi32.NewProc("GdiFlush")

	pShellNotifyIconW       = shell32.NewProc("Shell_NotifyIconW")
	pShellNotifyIconGetRect = shell32.NewProc("Shell_NotifyIconGetRect")
	pRegGetValueW           = advapi32.NewProc("RegGetValueW")
	pRegOpenKeyExW          = advapi32.NewProc("RegOpenKeyExW")
	pRegSetValueExW         = advapi32.NewProc("RegSetValueExW")
	pRegDeleteValueW        = advapi32.NewProc("RegDeleteValueW")
	pRegCloseKey            = advapi32.NewProc("RegCloseKey")
	pDwmSetWindowAttribute  = dwmapi.NewProc("DwmSetWindowAttribute")
	pDwmExtendFrame         = dwmapi.NewProc("DwmExtendFrameIntoClientArea")
	pDwmFlush               = dwmapi.NewProc("DwmFlush")
	pGetDpiForMonitor       = shcore.NewProc("GetDpiForMonitor")
	pRtlGetVersion          = ntdll.NewProc("RtlGetVersion")
	pGetNumPhysicalMonitors = dxva2.NewProc("GetNumberOfPhysicalMonitorsFromHMONITOR")
	pGetPhysicalMonitors    = dxva2.NewProc("GetPhysicalMonitorsFromHMONITOR")
	pDestroyPhysicalMonitor = dxva2.NewProc("DestroyPhysicalMonitor")
	pGetVCPFeatureAndReply  = dxva2.NewProc("GetVCPFeatureAndVCPFeatureReply")
	pSetVCPFeature          = dxva2.NewProc("SetVCPFeature")
)

const (
	wmDestroy        = 0x0002
	wmActivate       = 0x0006
	wmPaint          = 0x000F
	wmEraseBkgnd     = 0x0014
	wmSettingChange  = 0x001A
	wmNcCalcSize     = 0x0083
	wmNcHitTest      = 0x0084
	wmNcActivate     = 0x0086
	wmContextMenu    = 0x007B
	wmDisplayChange  = 0x007E
	wmKeyDown        = 0x0100
	wmTimer          = 0x0113
	wmMouseMove      = 0x0200
	wmLButtonDown    = 0x0201
	wmLButtonUp      = 0x0202
	wmRButtonUp      = 0x0205
	wmMouseWheel     = 0x020A
	wmCaptureChanged = 0x0215
	wmPowerBroadcast = 0x0218
	wmMouseLeave     = 0x02A3
	wmApp            = 0x8000

	wsPopup         = 0x80000000
	wsCaption       = 0x00C00000
	htClient        = 1
	wsExTopmost     = 0x00000008
	wsExToolWindow  = 0x00000080
	csDropShadow    = 0x00020000
	swHide          = 0
	swpShowWindow   = 0x0040
	swpNoActivate   = 0x0010
	swpNoSize       = 0x0001
	swpNoMove       = 0x0002
	swpNoZOrder     = 0x0004
	swpFrameChanged = 0x0020
	gaRoot          = 2
	idcArrow        = 32512
	smCxSmIcon      = 49
	tmeLeave        = 0x00000002
	monitorNearest  = 2
	pbtResumeAuto   = 0x0012
	errAlreadyExist = 183

	nimAdd     = 0
	nimModify  = 1
	nimDelete  = 2
	nifMessage = 0x01
	nifIcon    = 0x02
	nifTip     = 0x04

	mfString      = 0x0000
	mfChecked     = 0x0008
	mfSeparator   = 0x0800
	tpmRightBtn   = 0x0002
	tpmBottomAlgn = 0x0020
	tpmNoNotify   = 0x0080
	tpmReturnCmd  = 0x0100

	vkTab    = 0x09
	vkShift  = 0x10
	vkEscape = 0x1B
	vkPrior  = 0x21
	vkNext   = 0x22
	vkEnd    = 0x23
	vkHome   = 0x24
	vkLeft   = 0x25
	vkUp     = 0x26
	vkRight  = 0x27
	vkDown   = 0x28

	dtCenter       = 0x0001
	dtRight        = 0x0002
	dtVCenter      = 0x0004
	dtWordBreak    = 0x0010
	dtSingleLine   = 0x0020
	dtNoPrefix     = 0x0800
	dtEndEllipsis  = 0x8000
	bkTransparent  = 1
	srcCopy        = 0x00CC0020
	cleartypeQual  = 5
	antialiasQual  = 4 // greyscale: ClearType needs an opaque background
	defaultCharset = 1

	hkeyCurrentUser = 0x80000001
	keyQueryValue   = 0x0001
	keySetValue     = 0x0002
	regSZ           = 1
	rrfRtRegBinary  = 0x00000008
	rrfRtRegDword   = 0x00000010

	dwmwaTransitionsOff   = 3 // DWMWA_TRANSITIONS_FORCEDISABLED
	dwmwaUseDarkMode      = 20
	dwmwaCornerPreference = 33
	dwmwaBorderColor      = 34
	dwmwaSystemBackdrop   = 38
	dwmwcpRound           = 2
	dwmColorNone          = 0xFFFFFFFE
	dwmsbtNone            = 1
	dwmsbtTransient       = 3 // Desktop Acrylic, as used by Windows 11 flyouts

	spiGetClientAreaAnimation = 0x1042
)

var hwndTopmost = ^uintptr(0) // (HWND)-1

func utf16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(strings.ReplaceAll(s, "\x00", ""))
	if err != nil {
		p, _ = syscall.UTF16PtrFromString("")
	}
	return p
}

func loword(v uintptr) uint16 { return uint16(v & 0xFFFF) }
func hiword(v uintptr) uint16 { return uint16((v >> 16) & 0xFFFF) }

// Signed client coordinates packed into an LPARAM.
func lparamXY(l uintptr) (int32, int32) {
	return int32(int16(loword(l))), int32(int16(hiword(l)))
}

func colorref(c rgb) uintptr {
	return uintptr(c.r) | uintptr(c.g)<<8 | uintptr(c.b)<<16
}

func tickCount() uint64 {
	t, _, _ := pGetTickCount64.Call()
	return uint64(t)
}

var perfFreq int64

// perfSeconds is a high-resolution clock for animation; GetTickCount moves
// in steps of about 16 ms, which is a whole frame.
func perfSeconds() float64 {
	if perfFreq == 0 {
		pQueryPerfFrequency.Call(uintptr(unsafe.Pointer(&perfFreq)))
		if perfFreq == 0 {
			return float64(tickCount()) / 1000
		}
	}
	var c int64
	pQueryPerfCounter.Call(uintptr(unsafe.Pointer(&c)))
	return float64(c) / float64(perfFreq)
}

// animationsOn follows Settings > Accessibility > Visual effects >
// Animation effects.
func animationsOn() bool {
	on := int32(1)
	if r, _, _ := pSystemParametersInfoW.Call(spiGetClientAreaAnimation, 0, uintptr(unsafe.Pointer(&on)), 0); r == 0 {
		return true
	}
	return on != 0
}

// available reports whether an optional export exists on this Windows build.
func available(p *syscall.LazyProc) bool { return p.Find() == nil }

func windowsBuild() uint32 {
	var v osVersionInfo
	v.dwOSVersionInfoSize = uint32(unsafe.Sizeof(v))
	if !available(pRtlGetVersion) {
		return 0
	}
	pRtlGetVersion.Call(uintptr(unsafe.Pointer(&v)))
	return v.dwBuildNumber
}

// --- registry -------------------------------------------------------------

func regDword(subkey, name string) (uint32, bool) {
	var data, size uint32 = 0, 4
	r, _, _ := pRegGetValueW.Call(hkeyCurrentUser,
		uintptr(unsafe.Pointer(utf16(subkey))), uintptr(unsafe.Pointer(utf16(name))),
		rrfRtRegDword, 0, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&size)))
	return data, r == 0
}

func regBinary(subkey, name string, buf []byte) (int, bool) {
	size := uint32(len(buf))
	r, _, _ := pRegGetValueW.Call(hkeyCurrentUser,
		uintptr(unsafe.Pointer(utf16(subkey))), uintptr(unsafe.Pointer(utf16(name))),
		rrfRtRegBinary, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	return int(size), r == 0
}

func regString(subkey, name string) (string, bool) {
	var buf [1024]uint16
	size := uint32(len(buf) * 2)
	r, _, _ := pRegGetValueW.Call(hkeyCurrentUser,
		uintptr(unsafe.Pointer(utf16(subkey))), uintptr(unsafe.Pointer(utf16(name))),
		0x00000002 /* RRF_RT_REG_SZ */, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r != 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf[:]), true
}

func regSetString(subkey, name, value string) bool {
	var key uintptr
	r, _, _ := pRegOpenKeyExW.Call(hkeyCurrentUser, uintptr(unsafe.Pointer(utf16(subkey))), 0,
		keySetValue, uintptr(unsafe.Pointer(&key)))
	if r != 0 {
		return false
	}
	defer pRegCloseKey.Call(key)
	data, err := syscall.UTF16FromString(value)
	if err != nil {
		return false
	}
	r, _, _ = pRegSetValueExW.Call(key, uintptr(unsafe.Pointer(utf16(name))), 0, regSZ,
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)*2))
	return r == 0
}

func regDelete(subkey, name string) bool {
	var key uintptr
	r, _, _ := pRegOpenKeyExW.Call(hkeyCurrentUser, uintptr(unsafe.Pointer(utf16(subkey))), 0,
		keySetValue, uintptr(unsafe.Pointer(&key)))
	if r != 0 {
		return false
	}
	defer pRegCloseKey.Call(key)
	r, _, _ = pRegDeleteValueW.Call(key, uintptr(unsafe.Pointer(utf16(name))))
	return r == 0
}

// --- theme ----------------------------------------------------------------

const (
	keyPersonalize = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`
	keyAccent      = `Software\Microsoft\Windows\CurrentVersion\Explorer\Accent`
)

// taskbarIsDark follows the "Windows mode" colour, which is what the taskbar
// and its flyouts use (the "app mode" colour is a separate setting).
func taskbarIsDark() bool {
	if v, ok := regDword(keyPersonalize, "SystemUsesLightTheme"); ok {
		return v == 0
	}
	return true
}

// accentFor picks the shade of the user's accent colour that Windows itself
// uses for controls on a dark or light surface.
func accentFor(dark bool) rgb {
	var pal [32]byte // eight RGBA entries: Light3..Light1, Accent, Dark1..Dark3, complement
	if n, ok := regBinary(keyAccent, "AccentPalette", pal[:]); ok && n == len(pal) {
		i := 4 * 4 // Dark1
		if dark {
			i = 1 * 4 // Light2
		}
		return rgb{pal[i], pal[i+1], pal[i+2]}
	}
	return defaultAccent(dark)
}

// allowDarkMenus opts the process into dark context menus when the user's
// app theme is dark. The exports are unnamed (uxtheme ordinals 135 and 136,
// present since Windows 10 1903); if they are missing the menu stays light.
func allowDarkMenus(build uint32) {
	if build < 18362 || uxtheme.Load() != nil {
		return
	}
	setMode, _, _ := pGetProcAddress.Call(uxtheme.Handle(), 135)
	flush, _, _ := pGetProcAddress.Call(uxtheme.Handle(), 136)
	if setMode != 0 && flush != 0 {
		syscall.SyscallN(setMode, 1) // SetPreferredAppMode(AllowDark)
		syscall.SyscallN(flush)      // FlushMenuThemes
	}
}

// --- autostart ------------------------------------------------------------

const (
	keyRun      = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValue    = "ExtBrightness"
	appTitle    = "External Monitor Brightness"
	mutexName   = `Local\ExtBrightness.SingleInstance`
	classMain   = "ExtBrightness.Main"
	classFlyout = "ExtBrightness.Flyout"
)

func autostartCommand(exe string) string { return `"` + exe + `"` }

func autostartEnabled() (string, bool) { return regString(keyRun, runValue) }

func setAutostart(exe string, on bool) {
	if on {
		regSetString(keyRun, runValue, autostartCommand(exe))
	} else {
		regDelete(keyRun, runValue)
	}
}
