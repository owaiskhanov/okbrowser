module github.com/owaiskhanov/okbrowser

go 1.27

require (
	github.com/jchv/go-webview2 v0.0.0-00010101000000-000000000000
	github.com/lxn/win v0.0.0-00010101000000-000000000000
	golang.org/x/sys v0.0.0-20210218145245-beda7e5e158e
)

require github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect

// All dependencies are vendored into third_party/ so the project builds
// fully offline and is immune to upstream changes.
replace github.com/jchv/go-webview2 => ./third_party/go-webview2

replace github.com/jchv/go-winloader => ./third_party/go-winloader

replace github.com/lxn/win => ./third_party/lxn-win

replace golang.org/x/sys => ./third_party/x-sys
