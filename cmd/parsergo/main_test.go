package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sagaragas/parser-go/internal/api"
)

func TestMainStub(t *testing.T) {
	// Main package tests are handled via integration tests in api package
	// This is a placeholder to ensure the package has test coverage
	t.Log("Main package - see api package for handler tests")
}

func TestServeWithListenerExposesStartupReadinessWindow(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveWithListener(ctx, logger, listener, 250*time.Millisecond)
	}()

	baseURL := "http://" + listener.Addr().String()

	waitForStatus(t, baseURL+"/healthz", http.StatusOK, 2*time.Second)

	readyResp, readyBody := doRequest(t, http.MethodGet, baseURL+"/readyz", "", nil)
	if readyResp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected initial /readyz status %d, got %d with body %s", http.StatusServiceUnavailable, readyResp.StatusCode, readyBody)
	}
	if !strings.Contains(readyBody, `"ready":false`) {
		t.Fatalf("expected initial /readyz body to contain ready:false, got %s", readyBody)
	}

	var submitBuf bytes.Buffer
	writer := multipart.NewWriter(&submitBuf)
	part, err := writer.CreateFormFile("file", "access.log")
	if err != nil {
		t.Fatalf("create multipart file failed: %v", err)
	}
	if _, err := part.Write([]byte(`127.0.0.1 - - [10/Oct/2000:13:55:36 -0700] "GET /test HTTP/1.0" 200 100`)); err != nil {
		t.Fatalf("write multipart file failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}

	submitResp, submitBody := doRequest(t, http.MethodPost, baseURL+"/v1/analyses", writer.FormDataContentType(), &submitBuf)
	if submitResp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected startup submission status %d, got %d with body %s", http.StatusServiceUnavailable, submitResp.StatusCode, submitBody)
	}

	var apiErr api.APIError
	if err := json.Unmarshal([]byte(submitBody), &apiErr); err != nil {
		t.Fatalf("failed to decode startup submission body: %v", err)
	}
	if apiErr.Code != api.ErrCodeServiceUnavailable {
		t.Fatalf("expected startup submission error code %s, got %s", api.ErrCodeServiceUnavailable, apiErr.Code)
	}
	if strings.Contains(submitBody, `"id"`) {
		t.Fatalf("expected startup submission body to omit job id, got %s", submitBody)
	}

	waitForStatus(t, baseURL+"/readyz", http.StatusOK, 2*time.Second)

	cancel()

	if err := <-errCh; err != nil && err != http.ErrServerClosed {
		t.Fatalf("serveWithListener returned error: %v", err)
	}
}

func waitForStatus(t *testing.T, url string, want int, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, _ := doRequest(t, http.MethodGet, url, "", nil)
		if resp.StatusCode == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	resp, body := doRequest(t, http.MethodGet, url, "", nil)
	t.Fatalf("timed out waiting for %s to return %d, got %d with body %s", url, want, resp.StatusCode, body)
}

func doRequest(t *testing.T, method, url, contentType string, body io.Reader) (*http.Response, string) {
	t.Helper()

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	client := &http.Client{Timeout: 200 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		return &http.Response{StatusCode: 0}, err.Error()
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body failed: %v", err)
	}

	return resp, string(data)
}
