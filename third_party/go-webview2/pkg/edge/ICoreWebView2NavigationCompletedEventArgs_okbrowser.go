package edge

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// OK Browser addition: typed getters for the navigation-completed args.

// GetIsSuccess reports whether the navigation completed successfully.
func (i *ICoreWebView2NavigationCompletedEventArgs) GetIsSuccess() (bool, error) {
	var v int32 // COM BOOL is 32-bit
	_, _, err := i.vtbl.GetIsSuccess.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&v)),
	)
	if err != windows.ERROR_SUCCESS {
		return false, err
	}
	return v != 0, nil
}

// GetWebErrorStatus returns the COREWEBVIEW2_WEB_ERROR_STATUS of a failed
// navigation (0 = Unknown).
func (i *ICoreWebView2NavigationCompletedEventArgs) GetWebErrorStatus() (uint32, error) {
	var v uint32
	_, _, err := i.vtbl.GetWebErrorStatus.Call(
		uintptr(unsafe.Pointer(i)),
		uintptr(unsafe.Pointer(&v)),
	)
	if err != windows.ERROR_SUCCESS {
		return 0, err
	}
	return v, nil
}
