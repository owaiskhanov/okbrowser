package edge

// OK Browser: the ICoreWebView2_2 and ICoreWebView2_3 vtable layouts.
//
// Only the vtable SHAPES are needed: iCoreWebView2_4Vtbl (which carries the
// download events we use) embeds _3, which embeds _2, which embeds the base
// interface. Getting these layouts right is what makes the AddDownloadStarting
// slot land at the correct offset.
//
// The public ICoreWebView2_2 / ICoreWebView2_3 wrapper types and their
// methods (suspend/resume, virtual host mapping) are not used by the browser
// and were removed.

type iCoreWebView2_2Vtbl struct {
	iCoreWebView2Vtbl
	AddWebResourceResponseReceived    ComProc
	RemoveWebResourceResponseReceived ComProc
	NavigateWithWebResourceRequest    ComProc
	AddDomContentLoaded               ComProc
	RemoveDomContentLoaded            ComProc
	GetCookieManager                  ComProc
	GetEnvironment                    ComProc
}

type iCoreWebView2_3Vtbl struct {
	iCoreWebView2_2Vtbl
	TrySuspend                          ComProc
	Resume                              ComProc
	GetIsSuspended                      ComProc
	SetVirtualHostNameToFolderMapping   ComProc
	ClearVirtualHostNameToFolderMapping ComProc
}
