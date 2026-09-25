//go:build windows

package archive

import (
	"fmt"
	"syscall"
	"unsafe"
)

func availableBytes(path string) (uint64, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	getDiskFreeSpaceEx := syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")
	var freeBytes uint64
	result, _, callErr := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytes)),
		0,
		0,
	)
	if result == 0 {
		return 0, fmt.Errorf("GetDiskFreeSpaceEx %q: %w", path, callErr)
	}
	return freeBytes, nil
}
