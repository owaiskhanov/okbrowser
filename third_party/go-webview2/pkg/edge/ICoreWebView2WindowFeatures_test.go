//go:build windows

package edge

import (
	"reflect"
	"testing"
	"unsafe"
)

// Vtable slot order is the declaration order in WebView2.idl, NOT the
// alphabetical order of the published "Summary" table. Getting this wrong is
// how every download used to crash the browser.
//
// The trap specific to this interface: get_Height comes BEFORE get_Width,
// which is the opposite of the usual (width, height) convention. Sorting
// these "sensibly" would swap a popup's two axes.
func TestWindowFeaturesVtblOrder(t *testing.T) {
	want := []string{
		// IUnknown
		"QueryInterface", "AddRef", "Release",
		// ICoreWebView2WindowFeatures
		"GetHasPosition",
		"GetHasSize",
		"GetLeft",
		"GetTop",
		"GetHeight",
		"GetWidth",
		"GetShouldDisplayMenuBar",
		"GetShouldDisplayStatus",
		"GetShouldDisplayToolbar",
		"GetShouldDisplayScrollBars",
	}

	typ := reflect.TypeOf(_ICoreWebView2WindowFeaturesVtbl{})
	var got []string
	var walk func(reflect.Type)
	walk = func(rt reflect.Type) {
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			if f.Anonymous && f.Type.Kind() == reflect.Struct {
				walk(f.Type)
				continue
			}
			got = append(got, f.Name)
		}
	}
	walk(typ)

	if len(got) != len(want) {
		t.Fatalf("vtable has %d slots, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("slot %d is %q, want %q", i, got[i], want[i])
		}
	}

	// Every slot must be pointer-sized; a wrong-sized field shifts everything
	// after it.
	if s := unsafe.Sizeof(_ICoreWebView2WindowFeaturesVtbl{}); s != uintptr(len(want))*unsafe.Sizeof(uintptr(0)) {
		t.Errorf("vtable struct is %d bytes, want %d", s, uintptr(len(want))*unsafe.Sizeof(uintptr(0)))
	}
}

// GetWindowFeatures was appended to the NewWindowRequested args vtable; it
// must land in the slot after GetDeferral or it would call the wrong method.
func TestNewWindowRequestedArgsVtblOrder(t *testing.T) {
	want := []string{
		"QueryInterface", "AddRef", "Release",
		"GetUri",
		"PutNewWindow",
		"GetNewWindow",
		"PutHandled",
		"GetHandled",
		"GetIsUserInitiated",
		"GetDeferral",
		"GetWindowFeatures",
	}

	typ := reflect.TypeOf(_ICoreWebView2NewWindowRequestedEventArgsVtbl{})
	var got []string
	var walk func(reflect.Type)
	walk = func(rt reflect.Type) {
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			if f.Anonymous && f.Type.Kind() == reflect.Struct {
				walk(f.Type)
				continue
			}
			got = append(got, f.Name)
		}
	}
	walk(typ)

	if len(got) != len(want) {
		t.Fatalf("vtable has %d slots, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("slot %d is %q, want %q", i, got[i], want[i])
		}
	}
}

// ICoreWebView2Deferral adds exactly one method after IUnknown. If Complete
// were not in slot 3, completing a deferral would call an arbitrary function
// pointer - and the deferral is what keeps an OAuth opener's script blocked
// until the popup's engine exists.
func TestDeferralVtblOrder(t *testing.T) {
	want := []string{"QueryInterface", "AddRef", "Release", "Complete"}

	var got []string
	var walk func(reflect.Type)
	walk = func(rt reflect.Type) {
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			if f.Anonymous && f.Type.Kind() == reflect.Struct {
				walk(f.Type)
				continue
			}
			got = append(got, f.Name)
		}
	}
	walk(reflect.TypeOf(_ICoreWebView2DeferralVtbl{}))

	if len(got) != len(want) {
		t.Fatalf("vtable has %d slots, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("slot %d is %q, want %q", i, got[i], want[i])
		}
	}
}
