package health

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWaitForHealthySuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	parts := strings.Split(srv.URL, ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])

	// Should return quickly — first attempt succeeds.
	waitForHealthyWithConfig("test", "test-container", port, 3, 10*time.Millisecond, 1*time.Second)
}

func TestWaitForHealthyTimeout(t *testing.T) {
	// Use a port that nothing is listening on with minimal retries.
	waitForHealthyWithConfig("test", "test-container", 19999, 2, 10*time.Millisecond, 50*time.Millisecond)
}

func TestWaitForHealthyNon200(t *testing.T) {
	// A server returning 500 should NOT pass the health check.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	parts := strings.Split(srv.URL, ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])

	// Should time out since 500 != 200.
	waitForHealthyWithConfig("test", "test-container", port, 2, 10*time.Millisecond, 1*time.Second)
}

func TestWaitForHealthyPublicAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	parts := strings.Split(srv.URL, ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])

	// Test the public API.
	WaitForHealthy("test", "test-container", port)
}
