package main

import (
	"embed"
	_ "embed"
	"io/fs"
	"net/http"
)

// webdist holds the built Svelte panel, copied in by `mise run build`. Only
// .gitkeep is tracked, so a plain `go build` (or `go test`, or a checkout with no
// web toolchain) still compiles — it just embeds no index.html. Nothing tracked
// lives here, so a build can never clobber a checked-in file.
//
//go:embed all:webdist
var webFS embed.FS

// placeholderPage is served when the binary was built without the panel. It lives
// outside webdist precisely so `mise run build` (which wipes webdist) cannot eat it.
//
//go:embed placeholder.html
var placeholderPage []byte

// uiHandler serves the built Svelte panel. When the binary lacks it, every route
// serves the placeholder explaining how to build it — silently serving nothing
// looks like a broken server, which is how this bug was found.
func uiHandler() http.Handler {
	sub, err := fs.Sub(webFS, "webdist")
	if err != nil {
		panic(err) // impossible: embedded path is fixed at compile time
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write(placeholderPage)
		})
	}
	return http.FileServerFS(sub)
}
