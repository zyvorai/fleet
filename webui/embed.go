package webui

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"sync"
)

//go:embed static/*
var assets embed.FS

// assetVersion is a short hash of app.css+app.js, computed once from the
// embedded bytes. It's injected as a ?v= query param on index.html's asset
// links so every new binary automatically busts browsers' cached copies of
// app.css/app.js instead of relying on users to hard-refresh.
var assetVersion = sync.OnceValue(func() string {
	sub, _ := fs.Sub(assets, "static")
	css, _ := fs.ReadFile(sub, "app.css")
	js, _ := fs.ReadFile(sub, "app.js")
	sum := sha256.Sum256(append(css, js...))
	return hex.EncodeToString(sum[:])[:10]
})

func Handler() http.Handler {
	sub, _ := fs.Sub(assets, "static")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "." || p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(sub, p); err != nil {
			p = "index.html"
		}
		b, err := fs.ReadFile(sub, p)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if p == "index.html" {
			w.Header().Set("Cache-Control", "no-store")
			v := assetVersion()
			html := strings.NewReplacer(
				`href="/app.css"`, `href="/app.css?v=`+v+`"`,
				`src="/app.js"`, `src="/app.js?v=`+v+`"`,
			).Replace(string(b))
			b = []byte(html)
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		if ct := mime.TypeByExtension(path.Ext(p)); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = w.Write(b)
	})
}
