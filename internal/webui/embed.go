package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed dist
var content embed.FS

func Handler() http.Handler {
	sub, err := fs.Sub(content, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.FileServer(http.FS(sub))
}
