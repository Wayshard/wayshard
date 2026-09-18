package webembed

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// Dist is populated at release time by copying the Web build into this directory.
//
//go:embed all:dist
var Dist embed.FS

func Handler() http.Handler {
	sub, err := fs.Sub(Dist, "dist")
	if err != nil {
		return fallback()
	}
	file := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || !strings.Contains(r.URL.Path, ".") {
			if b, err := fs.ReadFile(sub, "index.html"); err == nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(b)
				return
			}
		}
		file.ServeHTTP(w, r)
	})
}

func fallback() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>Wayshard</title>
<style>body{font-family:ui-sans-serif,system-ui;background:#111;color:#eee;margin:2rem}</style>
<h1>Wayshard</h1><p>Web client is not embedded in this build. Use the CLI or rebuild with <code>make web</code>.</p>`))
	})
}
