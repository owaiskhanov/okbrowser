//go:build windows

package edge

import (
	"reflect"
	"testing"
	"unsafe"
)

// The COM vtable slot order below is the declaration order in WebView2.idl.
// It is NOT the alphabetical order shown by the "Summary" table of the
// published documentation -- confusing the two is what previously made every
// download crash the browser: GetURI() was wired to slot 17, which is really
// get_InterruptReason, so a 4-byte enum was written into an LPWSTR out-param
// and the resulting garbage pointer was dereferenced.
//
// A vtable is an ABI contract. If this test fails, the fix is to match the
// IDL, never to "sort" the fields.
func TestDownloadOperationVtblOrder(t *testing.T) {
	want := []string{
		// IUnknown
		"QueryInterface", "AddRef", "Release",
		// ICoreWebView2DownloadOperation
		"AddBytesReceivedChanged", "RemoveBytesReceivedChanged",
		"AddEstimatedEndTimeChanged", "RemoveEstimatedEndTimeChanged",
		"AddStateChanged", "RemoveStateChanged",
		"GetUri",
		"GetContentDisposition",
		"GetMimeType",
		"GetTotalBytesToReceive",
		"GetBytesReceived",
		"GetEstimatedEndTime",
		"GetResultFilePath",
		"GetState",
		"GetInterruptReason",
		"Cancel",
		"Pause",
		"Resume",
		"GetCanResume",
	}
	assertVtblOrder(t, reflect.TypeOf(iDownloadOperationVtbl{}), want)
}

// TestDownloadStartingArgsVtblOrder locks ICoreWebView2DownloadStartingEventArgs.
func TestDownloadStartingArgsVtblOrder(t *testing.T) {
	want := []string{
		"QueryInterface", "AddRef", "Release",
		"GetDownloadOperation",
		"GetCancel", "PutCancel",
		"GetResultFilePath", "PutResultFilePath",
		"GetHandled", "PutHandled",
		"GetDeferral",
	}
	assertVtblOrder(t, reflect.TypeOf(iDownloadStartingArgsVtbl{}), want)
}

// TestCoreWebView2_4VtblOrder locks ICoreWebView2_4, which is where
// add_DownloadStarting lives. It derives from ICoreWebView2_3, so the
// inherited slots must come first.
func TestCoreWebView2_4VtblOrder(t *testing.T) {
	v := reflect.TypeOf(iCoreWebView2_4Vtbl{})
	base := reflect.TypeOf(iCoreWebView2_3Vtbl{})
	if got := v.Field(0).Type; got != base {
		t.Fatalf("ICoreWebView2_4 must embed iCoreWebView2_3Vtbl first, got %v", got)
	}
	want := []string{
		"AddFrameCreated", "RemoveFrameCreated",
		"AddDownloadStarting", "RemoveDownloadStarting",
	}
	baseSlots := int(base.Size() / unsafe.Sizeof(ComProc(0)))
	for i, name := range want {
		f, ok := v.FieldByName(name)
		if !ok {
			t.Fatalf("ICoreWebView2_4 vtable missing %s", name)
		}
		wantSlot := baseSlots + i
		if gotSlot := int(f.Offset / unsafe.Sizeof(ComProc(0))); gotSlot != wantSlot {
			t.Errorf("%s: slot %d, want %d", name, gotSlot, wantSlot)
		}
	}
}

// assertVtblOrder flattens an embedded-struct vtable and checks that every
// method sits at the exact slot index the COM ABI requires.
func assertVtblOrder(t *testing.T, typ reflect.Type, want []string) {
	t.Helper()

	slot := unsafe.Sizeof(ComProc(0))
	if got := typ.Size() / slot; int(got) != len(want) {
		t.Fatalf("%s has %d slots, want %d", typ.Name(), got, len(want))
	}

	var flatten func(reflect.Type, uintptr)
	got := map[uintptr]string{}
	flatten = func(rt reflect.Type, base uintptr) {
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			if f.Type.Kind() == reflect.Struct {
				flatten(f.Type, base+f.Offset)
				continue
			}
			got[base+f.Offset] = f.Name
		}
	}
	flatten(typ, 0)

	for i, name := range want {
		off := uintptr(i) * slot
		if got[off] != name {
			t.Errorf("%s slot %d = %q, want %q", typ.Name(), i, got[off], name)
		}
	}
}
