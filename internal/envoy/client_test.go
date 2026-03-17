package envoy_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DuncanDoyle/krp/internal/envoy"
)

func TestFetchConfigDump_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/config_dump" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"configs": []}`))
	}))
	defer srv.Close()

	data, err := envoy.FetchConfigDump(srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty config dump")
	}
}

func TestFetchConfigDump_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := envoy.FetchConfigDump(srv.URL)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}
