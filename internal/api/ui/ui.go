// Package ui serves the console: a single page application built by vite into
// dist and compiled into the binary, so a release is still one file.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// dist is written by `npm --prefix web run build`. Only .gitkeep is committed,
// so a checkout without a frontend build still compiles; Built reports which
// of the two this binary is.
//
//go:embed all:dist
var dist embed.FS

const (
	indexFile = "index.html"
	// assetPrefix is the directory vite fills with content hashed files. A hash
	// in the name is what makes caching them for a year safe.
	assetPrefix   = "assets/"
	immutableYear = "public, max-age=31536000, immutable"
)

// contentSecurityPolicy is what the built console actually needs: its own
// scripts and styles, its own api, and nothing else. The build is configured
// to emit no inline script, so 'unsafe-inline' is not in the list.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self'; " +
	"img-src 'self' data:; " +
	"font-src 'self'; " +
	"connect-src 'self'; " +
	"form-action 'none'; " +
	"base-uri 'none'; " +
	"frame-ancestors 'none'"

// Built reports whether a console was compiled into this binary.
func Built() bool {
	root, err := root()

	return err == nil && hasConsole(root)
}

// Handler serves the console, falling back to index.html so a reload of a
// client side route lands on the application instead of a 404.
func Handler() http.Handler {
	root, err := root()
	if err != nil {
		return http.HandlerFunc(missing)
	}

	return newHandler(root)
}

func root() (fs.FS, error) {
	return fs.Sub(dist, "dist")
}

func newHandler(root fs.FS) http.Handler {
	if !hasConsole(root) {
		return http.HandlerFunc(missing)
	}

	files := http.FileServerFS(root)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")

		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)

		if path != "" && exists(root, path) {
			if strings.HasPrefix(path, assetPrefix) {
				w.Header().Set("Cache-Control", immutableYear)
			}

			files.ServeHTTP(w, r)

			return
		}

		serveIndex(w, r, root)
	})
}

// serveIndex answers every unknown path with the application shell. The shell
// must not be cached, or a browser keeps loading the asset names of an older
// release after an upgrade.
func serveIndex(w http.ResponseWriter, r *http.Request, root fs.FS) {
	index, err := fs.ReadFile(root, indexFile)
	if err != nil {
		http.Error(w, "console is unavailable", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)

		return
	}

	_, _ = w.Write(index)
}

func hasConsole(root fs.FS) bool {
	return exists(root, indexFile)
}

func exists(root fs.FS, path string) bool {
	info, err := fs.Stat(root, path)

	return err == nil && !info.IsDir()
}

// missing explains the empty dist rather than returning a bare 404, because a
// binary built without the frontend looks broken otherwise.
func missing(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("the console was not built into this binary, the api is at /api/v1\n"))
}
