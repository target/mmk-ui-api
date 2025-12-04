package httpx

import (
	"bufio"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
)

// Logging returns a middleware that logs HTTP requests and responses.
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			const defaultHTTPStatus = 200
			ww := &respWriter{ResponseWriter: w, status: defaultHTTPStatus}
			next.ServeHTTP(ww, r)
			logger.Info("http",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.status),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}

type respWriter struct {
	http.ResponseWriter
	status int
}

func (w *respWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Recover returns a middleware that recovers from panics and logs them.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("panic",
						slog.Any("error", err),
						slog.String("path", r.URL.Path),
						slog.String("method", r.Method),
						slog.String("stack", string(debug.Stack())))
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAuth returns a middleware that requires authentication.
// If the user is not authenticated, it returns a 401 Unauthorized response.
func RequireAuth(authSvc AuthServiceInterface) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session := getSessionFromRequest(r, authSvc)
			if session == nil {
				WriteError(w, ErrorParams{
					Code:    http.StatusUnauthorized,
					ErrCode: "authentication_required",
					Err:     errors.New("authentication required"),
				})
				return
			}

			// Add session to request context
			ctx := SetSessionInContext(r.Context(), session)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns a middleware that requires a specific role.
// If the user doesn't have the required role, it returns a 403 Forbidden response.
func RequireRole(authSvc AuthServiceInterface, requiredRole domainauth.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session := getSessionFromRequest(r, authSvc)
			if session == nil {
				WriteError(w, ErrorParams{
					Code:    http.StatusUnauthorized,
					ErrCode: "authentication_required",
					Err:     errors.New("authentication required"),
				})
				return
			}

			// Check if user has required role
			if !hasRequiredRole(session.Role, requiredRole) {
				WriteError(w, ErrorParams{
					Code:    http.StatusForbidden,
					ErrCode: "insufficient_permissions",
					Err:     errors.New("insufficient permissions"),
				})
				return
			}

			// Add session to request context
			ctx := SetSessionInContext(r.Context(), session)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuth returns a middleware that optionally adds authentication information.
// If the user is authenticated, the session is added to the request context.
// If not authenticated, the request continues without session information.
func OptionalAuth(authSvc AuthServiceInterface) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session := getSessionFromRequest(r, authSvc)
			if session != nil {
				// Add session to request context
				ctx := SetSessionInContext(r.Context(), session)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// getSessionFromRequest retrieves and validates a session from the request.
func getSessionFromRequest(r *http.Request, authSvc AuthServiceInterface) *domainauth.Session {
	// Get session ID from cookie
	sessionCookie, err := r.Cookie("session_id")
	if err != nil {
		return nil
	}

	// Get session from auth service
	session, err := authSvc.GetSession(r.Context(), sessionCookie.Value)
	if err != nil {
		return nil
	}

	return session
}

// hasRequiredRole checks if the user's role meets the required role.
// Role hierarchy: Guest < User < Admin.
func hasRequiredRole(userRole, requiredRole domainauth.Role) bool {
	roleHierarchy := map[domainauth.Role]int{
		domainauth.RoleGuest: 0,
		domainauth.RoleUser:  1,
		domainauth.RoleAdmin: 2,
	}

	userLevel, userExists := roleHierarchy[userRole]
	requiredLevel, requiredExists := roleHierarchy[requiredRole]

	if !userExists || !requiredExists {
		return false
	}

	return userLevel >= requiredLevel
}

// browserRequestKey is an unexported context key type for browser request detection.
type browserRequestKey struct{}

// BrowserDetection returns a middleware that detects browser requests vs API requests.
// It sets a context value that can be used by downstream handlers to determine
// whether to return HTML or JSON responses.
func BrowserDetection() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			isBrowser := isBrowserRequest(r)
			ctx := context.WithValue(r.Context(), browserRequestKey{}, isBrowser)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// IsBrowserRequest returns true if the current request is from a browser.
func IsBrowserRequest(r *http.Request) bool {
	if val := r.Context().Value(browserRequestKey{}); val != nil {
		if isBrowser, ok := val.(bool); ok {
			return isBrowser
		}
	}
	// Fallback to direct detection if middleware wasn't used
	return isBrowserRequest(r)
}

// isBrowserRequest determines if a request is from a browser based on:
// 1. Path prefix - API routes start with /api/
// 2. Accept header - browsers typically accept text/html
// 3. HTMX requests are considered browser requests.
func isBrowserRequest(r *http.Request) bool {
	// API routes are explicitly not browser requests
	if strings.HasPrefix(r.URL.Path, "/api/") {
		return false
	}

	// Static assets are not browser requests
	if strings.HasPrefix(r.URL.Path, "/static/") {
		return false
	}

	// HTMX requests are browser requests
	if IsHTMX(r) {
		return true
	}

	// Check Accept header for HTML preference
	accept := r.Header.Get("Accept")
	if accept == "" {
		// No Accept header, assume browser for non-API routes
		return true
	}

	// Browser requests typically accept text/html
	return strings.Contains(accept, "text/html")
}

// RequireAuthBrowser returns a middleware that requires authentication with browser-aware behavior.
// For API requests: returns 401 JSON response if not authenticated.
// For browser requests: redirects to login page if not authenticated.
func RequireAuthBrowser(authSvc AuthServiceInterface) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session := getSessionFromRequest(r, authSvc)
			if session == nil {
				if IsBrowserRequest(r) {
					// Redirect browser requests to login
					redirectToLogin(w, r)
					return
				}
				// Return JSON error for API requests
				WriteError(w, ErrorParams{
					Code:    http.StatusUnauthorized,
					ErrCode: "authentication_required",
					Err:     errors.New("authentication required"),
				})
				return
			}

			// Add session to request context
			ctx := SetSessionInContext(r.Context(), session)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRoleBrowser returns a middleware that requires a specific role with browser-aware behavior.
// For API requests: returns 401/403 JSON responses.
// For browser requests: redirects to login or shows access denied page.
func RequireRoleBrowser(authSvc AuthServiceInterface, requiredRole domainauth.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session := getSessionFromRequest(r, authSvc)
			if session == nil {
				if IsBrowserRequest(r) {
					// Redirect browser requests to login
					redirectToLogin(w, r)
					return
				}
				// Return JSON error for API requests
				WriteError(w, ErrorParams{
					Code:    http.StatusUnauthorized,
					ErrCode: "authentication_required",
					Err:     errors.New("authentication required"),
				})
				return
			}

			// Check if user has required role
			if !hasRequiredRole(session.Role, requiredRole) {
				if IsBrowserRequest(r) {
					// For browser requests, show access denied page
					showAccessDenied(w, r)
					return
				}
				// Return JSON error for API requests
				WriteError(w, ErrorParams{
					Code:    http.StatusForbidden,
					ErrCode: "insufficient_permissions",
					Err:     errors.New("insufficient permissions"),
				})
				return
			}

			// Add session to request context
			ctx := SetSessionInContext(r.Context(), session)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// redirectToLogin redirects browser requests to the login page with the current URL as redirect_uri.
func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	redirectPath := redirectPathForRequest(r)
	if redirectPath == "" {
		redirectPath = "/"
	}
	redirectParam := url.QueryEscape(redirectPath)

	if IsHTMX(r) {
		// For HTMX requests, instruct the browser to navigate to the signed-out page
		// so we can show consistent messaging and a sign-in button instead of an error swap.
		signedOutURL := "/auth/signed-out?redirect_uri=" + redirectParam
		SetHXRedirect(w, signedOutURL)
		w.WriteHeader(http.StatusOK)
		return
	}

	loginURL := "/auth/login?redirect_uri=" + redirectParam
	http.Redirect(w, r, loginURL, http.StatusSeeOther)
}

func redirectPathForRequest(r *http.Request) string {
	if IsHTMX(r) {
		if current := safeRedirectFromURL(r.Header.Get("Hx-Current-Url")); current != "" {
			return current
		}
		if referer := safeRedirectFromURL(r.Header.Get("Referer")); referer != "" {
			return referer
		}
	}

	return safeRedirectPath(r.URL.RequestURI())
}

func safeRedirectFromURL(raw string) string {
	if raw == "" {
		return ""
	}

	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	// Reject scheme-relative or host-only references.
	if u.Host != "" && !u.IsAbs() {
		return ""
	}

	// For absolute URLs, use just the path/query portion to keep redirects within the app.
	if u.IsAbs() {
		return safeRedirectPath(u.RequestURI())
	}

	return safeRedirectPath(raw)
}

// showAccessDenied shows an access denied page for browser requests.
func showAccessDenied(w http.ResponseWriter, _ *http.Request) {
	// For now, just return a simple HTTP error
	// This could be enhanced to render a proper error template
	http.Error(w, "Access Denied: You don't have permission to access this resource", http.StatusForbidden)
}

// CompressionConfig holds configuration for the compression middleware.
type CompressionConfig struct {
	Level         int // Compression level (1-9, where 6 is default)
	MinSize       int // Minimum response size to compress (bytes, 0 = always compress)
	writerPool    *gzipWriterPool
	compressTypes map[string]bool
	Logger        *slog.Logger
}

// gzipWriterPool manages a pool of gzip writers for reuse.
type gzipWriterPool struct {
	pools map[int]*gzipLevelPool
}

type gzipLevelPool struct {
	level int
	pool  *sync.Pool
}

func newGzipWriterPool() *gzipWriterPool {
	return &gzipWriterPool{
		pools: make(map[int]*gzipLevelPool),
	}
}

func (p *gzipWriterPool) get(level int) *gzip.Writer {
	pool := p.ensureLevelPool(level)
	if writer := p.tryGetWriter(pool); writer != nil {
		return writer
	}
	return newGzipWriter(level)
}

func (p *gzipWriterPool) put(w *gzip.Writer, level int) {
	if pool, ok := p.pools[level]; ok {
		w.Reset(io.Discard)
		pool.pool.Put(w)
	}
}

func getDefaultCompressibleTypes() map[string]bool {
	return map[string]bool{
		"text/html":                true,
		"text/css":                 true,
		"text/plain":               true,
		"text/xml":                 true,
		"text/javascript":          true,
		"application/javascript":   true,
		"application/x-javascript": true,
		"application/json":         true,
		"application/xml":          true,
		"application/rss+xml":      true,
		"application/atom+xml":     true,
		"image/svg+xml":            true,
	}
}

// Compression returns a middleware that compresses HTTP responses using gzip.
// It compresses responses only when:
// - Client accepts gzip encoding (via Accept-Encoding header).
// - Content-Type is compressible (text/html, text/css, application/json, etc.).
// - Response status is not 1xx, 204, or 304.
// - Request method is not HEAD.
// - Response size exceeds MinSize threshold (if configured).
func Compression(cfg CompressionConfig) func(http.Handler) http.Handler {
	if cfg.writerPool == nil {
		cfg.writerPool = newGzipWriterPool()
	}
	if cfg.compressTypes == nil {
		cfg.compressTypes = getDefaultCompressibleTypes()
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if client accepts gzip encoding (with basic q-value handling)
			if !acceptsGzip(r.Header.Get("Accept-Encoding")) {
				next.ServeHTTP(w, r)
				return
			}

			// Skip compression for HEAD requests
			if r.Method == http.MethodHead {
				next.ServeHTTP(w, r)
				return
			}

			// Wrap response writer to intercept writes and decide compression at WriteHeader time
			gzw := &gzipResponseWriter{
				ResponseWriter: w,
				request:        r,
				config:         &cfg,
				minSize:        cfg.MinSize,
			}

			// Add Vary header for cache compatibility
			w.Header().Add("Vary", "Accept-Encoding")

			next.ServeHTTP(gzw, r)

			// Ensure gzip writer is closed if it was used
			if gzw.gzipWriter != nil {
				if err := gzw.gzipWriter.Close(); err != nil {
					cfg.Logger.ErrorContext(r.Context(), "closing gzip writer failed", "error", err)
				}
				cfg.writerPool.put(gzw.gzipWriter, cfg.Level)
			}
		})
	}
}

// acceptsGzip checks if the client accepts gzip encoding, respecting q-values.
func acceptsGzip(acceptEncoding string) bool {
	if acceptEncoding == "" {
		return false
	}

	// Simple parsing: check for "gzip" and ensure it's not explicitly disabled with q=0
	parts := strings.Split(acceptEncoding, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)

		// Check if this part contains "gzip"
		if !strings.Contains(strings.ToLower(part), "gzip") {
			continue
		}

		// Extract encoding name (before any semicolon)
		encoding := part
		if idx := strings.Index(part, ";"); idx != -1 {
			encoding = strings.TrimSpace(part[:idx])
		}

		if strings.ToLower(encoding) != "gzip" {
			continue
		}

		// Check for explicit q=0 or q=0.0 (disabled)
		// This is a simple check - a full RFC implementation would parse q-values properly
		if strings.Contains(part, "q=0.0") || strings.Contains(part, "q=0;") || strings.HasSuffix(part, "q=0") {
			return false
		}
		return true
	}
	return false
}

// isCompressibleContentType checks if the content type should be compressed.
func isCompressibleContentType(contentType string, compressTypes map[string]bool) bool {
	// Extract media type without parameters (e.g., "text/html; charset=utf-8" -> "text/html")
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = contentType[:idx]
	}
	contentType = strings.TrimSpace(strings.ToLower(contentType))
	return compressTypes[contentType]
}

// gzipResponseWriter wraps http.ResponseWriter to compress response body.
type gzipResponseWriter struct {
	http.ResponseWriter
	request         *http.Request
	config          *CompressionConfig
	gzipWriter      *gzip.Writer
	headerWritten   bool
	shouldCompress  bool
	minSize         int
	bufferedContent []byte
}

func (p *gzipWriterPool) ensureLevelPool(level int) *gzipLevelPool {
	if pool, ok := p.pools[level]; ok {
		return pool
	}

	newPool := &gzipLevelPool{
		level: level,
		pool: &sync.Pool{
			New: func() interface{} {
				return newGzipWriter(level)
			},
		},
	}
	p.pools[level] = newPool
	return newPool
}

func (p *gzipWriterPool) tryGetWriter(pool *gzipLevelPool) *gzip.Writer {
	w := pool.pool.Get()
	if w == nil {
		return nil
	}

	writer, ok := w.(*gzip.Writer)
	if !ok {
		return nil
	}

	return writer
}

func newGzipWriter(level int) *gzip.Writer {
	w, err := gzip.NewWriterLevel(io.Discard, level)
	if err != nil {
		return gzip.NewWriter(io.Discard)
	}

	return w
}

// WriteHeader decides whether to compress based on status code, content-type, and existing encoding.
func (w *gzipResponseWriter) WriteHeader(statusCode int) {
	if w.headerWritten {
		return
	}
	w.headerWritten = true

	// Don't compress for certain status codes
	if statusCode < 200 || statusCode == http.StatusNoContent || statusCode == http.StatusNotModified {
		w.ResponseWriter.WriteHeader(statusCode)
		return
	}

	// Don't compress if Content-Encoding is already set
	if w.Header().Get("Content-Encoding") != "" {
		w.ResponseWriter.WriteHeader(statusCode)
		return
	}

	// Check if content type is compressible
	contentType := w.Header().Get("Content-Type")
	switch {
	case contentType == "":
		// If no content-type set yet, we'll need to buffer and decide later
		// For now, assume compressible and let Write handle it
		w.shouldCompress = true
	case !isCompressibleContentType(contentType, w.config.compressTypes):
		w.ResponseWriter.WriteHeader(statusCode)
		return
	default:
		w.shouldCompress = true
	}

	// If we should compress, initialize the gzip writer
	if w.shouldCompress {
		w.gzipWriter = w.config.writerPool.get(w.config.Level)
		w.gzipWriter.Reset(w.ResponseWriter)

		// Set compression headers
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length") // Length will change after compression
	}

	w.ResponseWriter.WriteHeader(statusCode)
}

// Write compresses data if compression is enabled.
func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if !w.headerWritten {
		// If content-type not set, try to detect it
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", http.DetectContentType(b))
		}
		w.WriteHeader(http.StatusOK)
	}

	// Handle minimum size threshold
	if w.minSize > 0 && w.gzipWriter != nil && len(w.bufferedContent) < w.minSize {
		w.bufferedContent = append(w.bufferedContent, b...)
		if len(w.bufferedContent) < w.minSize {
			return len(b), nil
		}
		// Threshold reached, write buffered content
		_, err := w.gzipWriter.Write(w.bufferedContent)
		w.bufferedContent = nil
		return len(b), err
	}

	if w.gzipWriter != nil {
		return w.gzipWriter.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

// Flush implements http.Flusher for streaming support.
func (w *gzipResponseWriter) Flush() {
	if w.gzipWriter != nil {
		if err := w.gzipWriter.Flush(); err != nil {
			w.config.Logger.ErrorContext(w.request.Context(), "flushing gzip writer failed", "error", err)
		}
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack implements http.Hijacker for WebSocket support.
func (w *gzipResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, errors.New("http.Hijacker not supported")
}

// Push implements http.Pusher for HTTP/2 server push support.
func (w *gzipResponseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return errors.New("http.Pusher not supported")
}
