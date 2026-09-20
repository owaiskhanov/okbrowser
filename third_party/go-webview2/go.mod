module github.com/jchv/go-webview2

go 1.26.0

require (
	github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1
	golang.org/x/sys v0.0.0-20210218145245-beda7e5e158e
)

// Vendored alongside the parent module so this module also builds and tests
// fully offline (see the parent go.mod).
replace github.com/jchv/go-winloader => ../go-winloader

replace golang.org/x/sys => ../x-sys
