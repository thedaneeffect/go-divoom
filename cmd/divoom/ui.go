package main

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:webdist
var webFS embed.FS

// uiHandler serves the built Svelte panel.
func uiHandler() http.Handler {
	sub, err := fs.Sub(webFS, "webdist")
	if err != nil {
		panic(err) // impossible: embedded path is fixed at compile time
	}
	return http.FileServerFS(sub)
}
