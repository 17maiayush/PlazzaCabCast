package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ridehailing/internal/web"
)

func TestHandlerServesTheConsole(t *testing.T) {
	rec := httptest.NewRecorder()
	web.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("status = %d, content type = %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "<title>PlazzaCabCast</title>") {
		t.Fatal("the embedded page is missing or not the console")
	}
}
