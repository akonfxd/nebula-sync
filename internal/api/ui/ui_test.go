package ui

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const indexBody = "<!doctype html><title>nebula-sync</title>"

func testConsole() fs.FS {
	return fstest.MapFS{
		indexFile:                 {Data: []byte(indexBody)},
		"assets/index-abc123.js":  {Data: []byte("export default 1;")},
		"assets/index-abc123.css": {Data: []byte("body{}")},
		"favicon.svg":             {Data: []byte("<svg/>")},
	}
}

func get(t *testing.T, console fs.FS, path string) *http.Response {
	t.Helper()

	recorder := httptest.NewRecorder()
	newHandler(console).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))

	response := recorder.Result()
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })

	return response
}

func TestHandler_servesTheShellForUnknownPaths(t *testing.T) {
	console := testConsole()

	for _, path := range []string{"/", "/instances", "/profiles/7", "/does-not-exist"} {
		response := get(t, console, path)

		assert.Equal(t, http.StatusOK, response.StatusCode, path)
		assert.Equal(t, "text/html; charset=utf-8", response.Header.Get("Content-Type"), path)
		assert.Equal(t, "no-store", response.Header.Get("Cache-Control"), path)
	}
}

func TestHandler_servesFilesThatExist(t *testing.T) {
	response := get(t, testConsole(), "/favicon.svg")

	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Empty(t, response.Header.Get("Cache-Control"))
}

// A hashed asset name changes with its content, so caching it forever is safe.
// The shell must never get that treatment or an upgrade never reaches a browser.
func TestHandler_cachesHashedAssetsButNotTheShell(t *testing.T) {
	console := testConsole()

	asset := get(t, console, "/assets/index-abc123.js")
	assert.Equal(t, immutableYear, asset.Header.Get("Cache-Control"))

	shell := get(t, console, "/")
	assert.Equal(t, "no-store", shell.Header.Get("Cache-Control"))
}

func TestHandler_sendsAContentSecurityPolicy(t *testing.T) {
	policy := get(t, testConsole(), "/").Header.Get("Content-Security-Policy")

	require.NotEmpty(t, policy)
	assert.Contains(t, policy, "default-src 'self'")
	assert.Contains(t, policy, "frame-ancestors 'none'")
	assert.NotContains(t, policy, "unsafe-inline")
	assert.NotContains(t, policy, "unsafe-eval")
}

// A binary built without running the frontend build must still answer, and say
// why, rather than looking like a broken deployment.
func TestHandler_withoutAConsoleExplainsItself(t *testing.T) {
	empty := fstest.MapFS{".gitkeep": {Data: []byte{}}}

	response := get(t, empty, "/")

	assert.Equal(t, http.StatusNotFound, response.StatusCode)
	assert.False(t, hasConsole(empty))
}

// The embedded filesystem always exists, built or not.
func TestEmbeddedDistIsUsable(t *testing.T) {
	root, err := root()
	require.NoError(t, err)

	entries, err := fs.ReadDir(root, ".")
	require.NoError(t, err)
	assert.NotEmpty(t, entries)

	if !Built() {
		t.Skip("console not built, run `make web`")
	}

	response := get(t, root, "/")
	assert.Equal(t, http.StatusOK, response.StatusCode)
}
