// Package web serves the browser console. It is one static page that talks to the API over
// plain HTTP, so it imports nothing from the rest of the project.
package web

import (
	_ "embed"
	"net/http"
)

//go:embed index.html
var page []byte

func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})
}
