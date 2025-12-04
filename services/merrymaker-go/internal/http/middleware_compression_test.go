package httpx

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	contentEncodingGzip = "gzip"
	acceptEncodingGzip  = "gzip"
)

func TestCompression(t *testing.T) {
	testContent := strings.Repeat("Hello, World! ", 1000) // Repeatable content compresses well

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(testContent))
	})

	tests := []struct {
		name           string
		acceptEncoding string
		expectGzip     bool
		level          int
	}{
		{
			name:           "client accepts gzip",
			acceptEncoding: "gzip, deflate",
			expectGzip:     true,
			level:          6,
		},
		{
			name:           "client does not accept gzip",
			acceptEncoding: "deflate",
			expectGzip:     false,
			level:          6,
		},
		{
			name:           "no accept-encoding header",
			acceptEncoding: "",
			expectGzip:     false,
			level:          6,
		},
		{
			name:           "compression level 1 (fastest)",
			acceptEncoding: acceptEncodingGzip,
			expectGzip:     true,
			level:          1,
		},
		{
			name:           "compression level 9 (best)",
			acceptEncoding: acceptEncodingGzip,
			expectGzip:     true,
			level:          9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := runCompressionTest(t, compressionTestConfig{
				Handler:        handler,
				Level:          tt.level,
				AcceptEncoding: tt.acceptEncoding,
			})
			defer resp.Body.Close()

			if tt.expectGzip {
				verifyGzipResponse(t, resp, testContent)
			} else {
				verifyUncompressedResponse(t, resp, testContent)
			}
		})
	}
}

type compressionTestConfig struct {
	Handler        http.Handler
	Level          int
	AcceptEncoding string
}

func runCompressionTest(t *testing.T, cfg compressionTestConfig) *http.Response {
	t.Helper()

	middleware := Compression(CompressionConfig{Level: cfg.Level})
	wrappedHandler := middleware(cfg.Handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if cfg.AcceptEncoding != "" {
		req.Header.Set("Accept-Encoding", cfg.AcceptEncoding)
	}

	rec := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rec, req)

	return rec.Result()
}

func verifyGzipResponse(t *testing.T, resp *http.Response, expectedContent string) {
	t.Helper()

	if resp.Header.Get("Content-Encoding") != contentEncodingGzip {
		t.Errorf("expected Content-Encoding: %s, got: %s", contentEncodingGzip, resp.Header.Get("Content-Encoding"))
	}

	if resp.Header.Get("Content-Length") != "" {
		t.Errorf("expected no Content-Length header, got: %s", resp.Header.Get("Content-Length"))
	}

	// Verify Vary header is set for cache compatibility
	if resp.Header.Get("Vary") != "Accept-Encoding" {
		t.Errorf("expected Vary: Accept-Encoding, got: %s", resp.Header.Get("Vary"))
	}

	body := decompressGzipBody(t, resp.Body)
	if string(body) != expectedContent {
		t.Errorf("decompressed content mismatch")
	}
}

func verifyUncompressedResponse(t *testing.T, resp *http.Response, expectedContent string) {
	t.Helper()

	if resp.Header.Get("Content-Encoding") == contentEncodingGzip {
		t.Errorf("expected no gzip encoding, got Content-Encoding: %s", contentEncodingGzip)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	if string(body) != expectedContent {
		t.Errorf("content mismatch")
	}
}

func decompressGzipBody(t *testing.T, r io.Reader) []byte {
	t.Helper()

	gr, err := gzip.NewReader(r)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer gr.Close()

	body, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("failed to read decompressed body: %v", err)
	}

	return body
}

func TestCompressionWithStatusCodes(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		expectGzip  bool
		contentType string
		writeBody   bool
	}{
		{"200 OK with HTML", http.StatusOK, true, "text/html", true},
		{"404 Not Found with HTML", http.StatusNotFound, true, "text/html", true},
		{"500 Internal Server Error with HTML", http.StatusInternalServerError, true, "text/html", true},
		{"204 No Content", http.StatusNoContent, false, "", false},
		{"304 Not Modified", http.StatusNotModified, false, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.contentType != "" {
					w.Header().Set("Content-Type", tt.contentType)
				}
				w.WriteHeader(tt.statusCode)
				if tt.writeBody {
					_, _ = w.Write([]byte("test content"))
				}
			})

			middleware := Compression(CompressionConfig{Level: 6})
			wrappedHandler := middleware(handler)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept-Encoding", acceptEncodingGzip)

			rec := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rec, req)

			resp := rec.Result()
			defer resp.Body.Close()

			if resp.StatusCode != tt.statusCode {
				t.Errorf("expected status code %d, got %d", tt.statusCode, resp.StatusCode)
			}

			gotEncoding := resp.Header.Get("Content-Encoding")
			if tt.expectGzip && gotEncoding != contentEncodingGzip {
				t.Errorf(
					"expected Content-Encoding: %s, got: %s",
					contentEncodingGzip,
					gotEncoding,
				)
			}
			if !tt.expectGzip && gotEncoding == contentEncodingGzip {
				t.Errorf("expected no gzip encoding for status %d", tt.statusCode)
			}
		})
	}
}

func TestCompressionContentTypeFiltering(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		expectGzip  bool
	}{
		{"text/html", "text/html", true},
		{"text/css", "text/css", true},
		{"application/json", "application/json", true},
		{"application/javascript", "application/javascript", true},
		{"image/svg+xml", "image/svg+xml", true},
		{"image/jpeg", "image/jpeg", false},
		{"image/png", "image/png", false},
		{"application/pdf", "application/pdf", false},
		{"application/zip", "application/zip", false},
		{"video/mp4", "video/mp4", false},
		{"text/html; charset=utf-8", "text/html; charset=utf-8", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("test content"))
			})

			middleware := Compression(CompressionConfig{Level: 6})
			wrappedHandler := middleware(handler)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept-Encoding", acceptEncodingGzip)

			rec := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rec, req)

			resp := rec.Result()
			defer resp.Body.Close()

			gotEncoding := resp.Header.Get("Content-Encoding")
			if tt.expectGzip && gotEncoding != contentEncodingGzip {
				t.Errorf(
					"expected Content-Encoding: %s for %s, got: %s",
					contentEncodingGzip,
					tt.contentType,
					gotEncoding,
				)
			}
			if !tt.expectGzip && gotEncoding == contentEncodingGzip {
				t.Errorf("expected no gzip encoding for %s", tt.contentType)
			}
		})
	}
}

func TestCompressionHEADRequest(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		// HEAD requests should not write body
	})

	middleware := Compression(CompressionConfig{Level: 6})
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest(http.MethodHead, "/", nil)
	req.Header.Set("Accept-Encoding", acceptEncodingGzip)

	rec := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	// HEAD requests should not be compressed
	if resp.Header.Get("Content-Encoding") == contentEncodingGzip {
		t.Errorf("expected no gzip encoding for HEAD request")
	}
}

func TestCompressionAcceptEncodingQValue(t *testing.T) {
	tests := []struct {
		name           string
		acceptEncoding string
		expectGzip     bool
	}{
		{"gzip with q=1", "gzip;q=1", true},
		{"gzip with q=0.5", "gzip;q=0.5", true},
		{"gzip with q=0", "gzip;q=0", false},
		{"gzip, deflate", "gzip, deflate", true},
		{"deflate, gzip", "deflate, gzip", true},
		{"deflate only", "deflate", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("test content"))
			})

			middleware := Compression(CompressionConfig{Level: 6})
			wrappedHandler := middleware(handler)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.acceptEncoding != "" {
				req.Header.Set("Accept-Encoding", tt.acceptEncoding)
			}

			rec := httptest.NewRecorder()
			wrappedHandler.ServeHTTP(rec, req)

			resp := rec.Result()
			defer resp.Body.Close()

			gotEncoding := resp.Header.Get("Content-Encoding")
			if tt.expectGzip && gotEncoding != contentEncodingGzip {
				t.Errorf(
					"expected Content-Encoding: %s for %s, got: %s",
					contentEncodingGzip,
					tt.acceptEncoding,
					gotEncoding,
				)
			}
			if !tt.expectGzip && gotEncoding == contentEncodingGzip {
				t.Errorf("expected no gzip encoding for %s", tt.acceptEncoding)
			}
		})
	}
}

func TestCompressionPreExistingContentEncoding(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Encoding", "br") // Already compressed with Brotli
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test content"))
	})

	middleware := Compression(CompressionConfig{Level: 6})
	wrappedHandler := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", acceptEncodingGzip)

	rec := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	// Should not override existing Content-Encoding
	if resp.Header.Get("Content-Encoding") != "br" {
		t.Errorf("expected Content-Encoding: br, got: %s", resp.Header.Get("Content-Encoding"))
	}
}
