package main

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:ui_dist
var uiDistFS embed.FS

// embeddedUI returns an http.FileSystem serving the embedded frontend.
func embeddedUI() http.FileSystem {
	sub, err := fs.Sub(uiDistFS, "ui_dist")
	if err != nil {
		panic("embedded ui_dist not found: " + err.Error())
	}
	return http.FS(sub)
}
