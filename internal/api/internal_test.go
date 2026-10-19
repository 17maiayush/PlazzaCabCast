package api

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ridehailing/internal/store"
)

func TestWriteJSONReportsMarshalFailures(t *testing.T) {
	var logs bytes.Buffer
	h := &handler{logger: slog.New(slog.NewTextHandler(&logs, nil))}
	rec := httptest.NewRecorder()
	h.writeJSON(rec, http.StatusOK, make(chan int))
	if rec.Code != http.StatusInternalServerError || !strings.Contains(logs.String(), "marshal") {
		t.Fatalf("status = %d, logs = %q", rec.Code, logs.String())
	}
}

func TestUnknownErrorsAre500AndLogged(t *testing.T) {
	var logs bytes.Buffer
	h := &handler{logger: slog.New(slog.NewTextHandler(&logs, nil))}
	rec := httptest.NewRecorder()
	h.writeError(rec, errors.New("disk on fire"))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "disk on fire") {
		t.Fatalf("status = %d, body = %s; the cause must not leak", rec.Code, rec.Body)
	}
	if !strings.Contains(logs.String(), "disk on fire") {
		t.Fatalf("cause not logged: %q", logs.String())
	}
}

func TestIdempotentReleasesTheKeyWhenTheHandlerPanics(t *testing.T) {
	idem := store.NewIdempotency()
	h := &handler{logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))}
	wrapped := h.idempotent(idem, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))

	req := httptest.NewRequest(http.MethodPost, "/rides", strings.NewReader(`{"user_id":"usr_1"}`))
	req.Header.Set("Idempotency-Key", "k1")
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected the panic to propagate")
			}
		}()
		wrapped.ServeHTTP(httptest.NewRecorder(), req)
	}()

	if replay, err := idem.Begin("usr_1:k1", "anything"); replay != nil || err != nil {
		t.Fatalf("the key should be free again, got %v, %v", replay, err)
	}
}
