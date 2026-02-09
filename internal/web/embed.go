package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

const uiPrefix = "/ui/logs/"

// Handler serves the embedded frontend SPA with cache headers and
// index.html fallback for React Router client-side routing.
// Missing assets/ files return 404 (not index.html) to surface broken chunks.
func Handler() http.Handler {
	subFS, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic("embedded frontend sub filesystem: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(subFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlPath := strings.TrimPrefix(r.URL.Path, uiPrefix)
		if urlPath == "" {
			urlPath = "index.html"
		}

		if f, err := subFS.Open(urlPath); err == nil {
			info, statErr := f.Stat()
			f.Close()
			if statErr == nil && info.IsDir() {
				http.NotFound(w, r)
				return
			}
			if strings.HasPrefix(urlPath, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else if urlPath == "index.html" {
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			}
			http.StripPrefix(uiPrefix, fileServer).ServeHTTP(w, r)
			return
		}

		// Missing asset files → real 404 (don't mask broken JS/CSS chunks)
		if strings.HasPrefix(urlPath, "assets/") {
			http.NotFound(w, r)
			return
		}

		// SPA fallback: non-asset paths → index.html for client-side routing
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		r.URL.Path = uiPrefix + "index.html"
		http.StripPrefix(uiPrefix, fileServer).ServeHTTP(w, r)
	})
}

func Prefix() string {
	return uiPrefix
}

func HasAssets() bool {
	entries, err := fs.ReadDir(distFS, "dist")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Name() != ".gitkeep" {
			return true
		}
	}
	return false
}
