package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"ridehailing/internal/domain"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := loadConfig(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "8080" || cfg.Radius != 5000 || cfg.Matching != "nearest" || cfg.Surge != "demand" ||
		cfg.CancelGrace != 3*time.Minute || cfg.CancelFee != 2000 || len(cfg.Tariffs) != len(domain.AllCarTypes()) {
		t.Fatalf("defaults = %+v", cfg)
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	cfg, err := loadConfig(env(map[string]string{
		"PORT": "9090", "MATCH_RADIUS_KM": "2.5", "MATCHING": "rated", "SURGE": "off", "CANCEL_GRACE_MIN": "0", "CANCEL_FEE_INR": "50",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "9090" || cfg.Radius != 2500 || cfg.Matching != "rated" || cfg.Surge != "off" || cfg.CancelGrace != 0 || cfg.CancelFee != 5000 {
		t.Fatalf("overrides = %+v", cfg)
	}
}

func TestLoadConfigRejectsBadValues(t *testing.T) {
	tests := map[string]string{
		"PORT": "abc", "MATCH_RADIUS_KM": "abc", "CANCEL_GRACE_MIN": "abc", "CANCEL_FEE_INR": "abc",
	}
	for key, value := range tests {
		t.Run(key+"="+value, func(t *testing.T) {
			_, err := loadConfig(env(map[string]string{key: value}))
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("error = %v, want one naming %s", err, key)
			}
		})
	}
	for _, tc := range []struct{ key, value string }{
		{"PORT", "70000"}, {"MATCH_RADIUS_KM", "0"}, {"CANCEL_GRACE_MIN", "-1"}, {"CANCEL_FEE_INR", "-1"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			if _, err := loadConfig(env(map[string]string{tc.key: tc.value})); err == nil || !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("error = %v, want one naming %s", err, tc.key)
			}
		})
	}
}

func TestNewHandlerWiresEveryStrategy(t *testing.T) {
	for _, matching := range []string{"nearest", "rated"} {
		for _, surge := range []string{"demand", "off"} {
			cfg, err := loadConfig(env(map[string]string{"MATCHING": matching, "SURGE": surge}))
			if err != nil {
				t.Fatal(err)
			}
			h, err := newHandler(cfg, io.Discard)
			if err != nil {
				t.Fatalf("%s/%s: %v", matching, surge, err)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("%s/%s: healthz = %d", matching, surge, rec.Code)
			}
			page := httptest.NewRecorder()
			h.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
			if page.Code != http.StatusOK || !strings.HasPrefix(page.Header().Get("Content-Type"), "text/html") {
				t.Fatalf("%s/%s: console = %d %q", matching, surge, page.Code, page.Header().Get("Content-Type"))
			}
		}
	}
}

func TestNewHandlerRejectsBadWiring(t *testing.T) {
	base, err := loadConfig(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		tweak func(*config)
	}{
		{"unknown matching", func(c *config) { c.Matching = "random" }},
		{"unknown surge", func(c *config) { c.Surge = "maybe" }},
		{"broken tariffs", func(c *config) { delete(c.Tariffs, domain.Sedan) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			cfg.Tariffs = maps.Clone(base.Tariffs)
			tc.tweak(&cfg)
			if _, err := newHandler(cfg, io.Discard); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestRunServesUntilCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := &syncBuffer{}
	done := make(chan error, 1)
	go func() { done <- run(ctx, env(map[string]string{"PORT": "0"}), out) }()

	listening := regexp.MustCompile(`listening on (http://\S+)`)
	var base string
	for deadline := time.Now().Add(5 * time.Second); base == "" && time.Now().Before(deadline); {
		if m := listening.FindStringSubmatch(out.String()); m != nil {
			base = m[1]
		} else {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if base == "" {
		t.Fatalf("server never reported its address; output: %q", out.String())
	}
	resp, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz = %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned %v after cancel", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run did not stop after cancel")
	}
}

func TestRunFailures(t *testing.T) {
	taken, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	port := fmt.Sprint(taken.Addr().(*net.TCPAddr).Port)

	tests := []struct {
		name string
		env  map[string]string
	}{
		{"bad config", map[string]string{"PORT": "abc"}},
		{"bad wiring", map[string]string{"MATCHING": "random"}},
		{"port in use", map[string]string{"PORT": port}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := run(context.Background(), env(tc.env), io.Discard); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestMainExitsOnBadConfig(t *testing.T) {
	t.Setenv("MATCHING", "random")
	var code int
	var errOut bytes.Buffer
	exit, stderr = func(c int) { code = c }, &errOut
	defer func() { exit, stderr = os.Exit, os.Stderr }()

	main()
	if code != 1 || !strings.Contains(errOut.String(), "MATCHING") {
		t.Fatalf("exit code = %d, stderr = %q", code, errOut.String())
	}
}
