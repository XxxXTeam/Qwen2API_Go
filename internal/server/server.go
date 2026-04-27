package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"qwen2api/internal/admin"
	"qwen2api/internal/auth"
	"qwen2api/internal/config"
	"qwen2api/internal/logging"
	"qwen2api/internal/metrics"
	"qwen2api/internal/openai"
)

func New(cfg config.Config, keyring *auth.Keyring, openAIHandler *openai.Handler, adminHandler *admin.Handler, stats *metrics.DashboardStats, logger *logging.Logger) *http.Server {
	mux := http.NewServeMux()

	withAnyKey := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			apiKey := auth.ExtractAPIKey(r)
			result := keyring.Validate(apiKey)
			if !result.IsValid {
				logger.WarnModule("AUTH", "auth rejected request_id=%s path=%s method=%s remote=%s reason=invalid_api_key api_key=%s", requestIDFromContext(r), r.URL.Path, r.Method, clientIP(r), logger.Mask(apiKey))
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "Unauthorized"})
				return
			}
			logger.DebugModule("AUTH", "auth accepted request_id=%s path=%s method=%s admin=%t api_key=%s", requestIDFromContext(r), r.URL.Path, r.Method, result.IsAdmin, logger.Mask(apiKey))
			next(w, r)
		}
	}

	withAnthropicKey := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			apiKey := auth.ExtractAPIKey(r)
			result := keyring.Validate(apiKey)
			if !result.IsValid {
				logger.WarnModule("AUTH", "anthropic auth rejected request_id=%s path=%s method=%s remote=%s reason=invalid_api_key api_key=%s", requestIDFromContext(r), r.URL.Path, r.Method, clientIP(r), logger.Mask(apiKey))
				writeJSON(w, http.StatusUnauthorized, map[string]any{
					"type": "error",
					"error": map[string]any{
						"type":    "authentication_error",
						"message": "Unauthorized",
					},
				})
				return
			}
			logger.DebugModule("AUTH", "anthropic auth accepted request_id=%s path=%s method=%s admin=%t api_key=%s", requestIDFromContext(r), r.URL.Path, r.Method, result.IsAdmin, logger.Mask(apiKey))
			next(w, r)
		}
	}

	withAdminKey := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			apiKey := auth.ExtractAPIKey(r)
			result := keyring.Validate(apiKey)
			if !result.IsValid || !result.IsAdmin {
				logger.WarnModule("AUTH", "admin auth rejected request_id=%s path=%s method=%s remote=%s valid=%t admin=%t api_key=%s", requestIDFromContext(r), r.URL.Path, r.Method, clientIP(r), result.IsValid, result.IsAdmin, logger.Mask(apiKey))
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "Admin access required"})
				return
			}
			logger.DebugModule("AUTH", "admin auth accepted request_id=%s path=%s method=%s api_key=%s", requestIDFromContext(r), r.URL.Path, r.Method, logger.Mask(apiKey))
			next(w, r)
		}
	}

	handle := func(pattern string, kind string, handler http.HandlerFunc) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			requestID := newRequestID()
			start := time.Now()
			statusWriter := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
			statusWriter.Header().Set("X-Request-ID", requestID)
			r = r.WithContext(withRequestID(r.Context(), requestID))
			logger.InfoModule("HTTP", "request started request_id=%s kind=%s method=%s path=%s query=%s remote=%s ua=%q content_length=%d", requestID, kind, r.Method, r.URL.Path, r.URL.RawQuery, clientIP(r), r.UserAgent(), r.ContentLength)
			handler(statusWriter, r)
			stats.RecordRequest(kind, statusWriter.statusCode)
			logger.InfoModule("HTTP", "request completed request_id=%s kind=%s method=%s path=%s status=%d duration=%s bytes=%d", requestID, kind, r.Method, r.URL.Path, statusWriter.statusCode, time.Since(start), statusWriter.bytesWritten)
		})
	}

	handle("/verify", "admin", ensureMethod(http.MethodPost, adminHandler.HandleVerify))
	handle("/models", "models", ensureMethod(http.MethodGet, openAIHandler.HandleModels))
	handle("/v1/models", "models", ensureMethod(http.MethodGet, withAnyKey(openAIHandler.HandleModels)))
	handle("/v1/chat/completions", "chat", ensureMethod(http.MethodPost, withAnyKey(openAIHandler.HandleChatCompletion)))
	handle("/v1/messages", "chat", ensureMethod(http.MethodPost, withAnthropicKey(openAIHandler.HandleAnthropicMessages)))
	handle("/v1/messages/count_tokens", "chat", ensureMethod(http.MethodPost, withAnthropicKey(openAIHandler.HandleAnthropicCountTokens)))
	handle("/v1/images/generations", "image", ensureMethod(http.MethodPost, withAnyKey(openAIHandler.HandleImagesGeneration)))
	handle("/v1/images/edits", "image", ensureMethod(http.MethodPost, withAnyKey(openAIHandler.HandleImagesEdit)))
	handle("/v1/videos", "video", ensureMethod(http.MethodPost, withAnyKey(openAIHandler.HandleVideos)))
	handle("/v1/uploads", "upload", ensureMethod(http.MethodPost, withAnyKey(openAIHandler.HandleUploads)))
	handle("/v1/files/upload", "upload", ensureMethod(http.MethodPost, withAnyKey(openAIHandler.HandleUploads)))

	handle("/api/dashboard/overview", "admin", ensureMethod(http.MethodGet, withAdminKey(adminHandler.HandleOverview)))
	handle("/api/models", "admin", ensureMethod(http.MethodGet, withAdminKey(adminHandler.HandleModels)))
	handle("/api/settings", "admin", ensureMethod(http.MethodGet, withAdminKey(adminHandler.HandleSettings)))
	handle("/api/addRegularKey", "admin", ensureMethod(http.MethodPost, withAdminKey(adminHandler.HandleAddRegularKey)))
	handle("/api/deleteRegularKey", "admin", ensureMethod(http.MethodPost, withAdminKey(adminHandler.HandleDeleteRegularKey)))
	handle("/api/setAutoRefresh", "admin", ensureMethod(http.MethodPost, withAdminKey(adminHandler.HandleSetAutoRefresh)))
	handle("/api/setBatchLoginConcurrency", "admin", ensureMethod(http.MethodPost, withAdminKey(adminHandler.HandleSetBatchLoginConcurrency)))
	handle("/api/setOutThink", "admin", ensureMethod(http.MethodPost, withAdminKey(adminHandler.HandleSetOutThink)))
	handle("/api/search-info-mode", "admin", ensureMethod(http.MethodPost, withAdminKey(adminHandler.HandleSearchInfoMode)))
	handle("/api/simple-model-map", "admin", ensureMethod(http.MethodPost, withAdminKey(adminHandler.HandleSimpleModelMap)))
	handle("/api/reload-runtime-config", "admin", ensureMethod(http.MethodPost, withAdminKey(adminHandler.HandleReloadRuntimeConfig)))
	handle("/api/getAllAccounts", "admin", ensureMethod(http.MethodGet, withAdminKey(adminHandler.HandleGetAccounts)))
	handle("/api/setAccount", "admin", ensureMethod(http.MethodPost, withAdminKey(adminHandler.HandleSetAccount)))
	handle("/api/deleteAccount", "admin", ensureMethod(http.MethodDelete, withAdminKey(adminHandler.HandleDeleteAccount)))
	handle("/api/setAccounts", "admin", ensureMethod(http.MethodPost, withAdminKey(adminHandler.HandleSetAccounts)))
	handle("/api/refreshAccount", "admin", ensureMethod(http.MethodPost, withAdminKey(adminHandler.HandleRefreshAccount)))
	handle("/api/refreshAllAccounts", "admin", ensureMethod(http.MethodPost, withAdminKey(adminHandler.HandleRefreshAllAccounts)))
	handle("/api/forceRefreshAllAccounts", "admin", ensureMethod(http.MethodPost, withAdminKey(adminHandler.HandleForceRefreshAllAccounts)))
	handle("/api/batchTasks/", "admin", ensureMethod(http.MethodGet, withAdminKey(adminHandler.HandleBatchTask)))

	publicDir := filepath.Join("public", "out")
	staticFS := http.FileServer(http.Dir(publicDir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Clean(r.URL.Path)
		if path == "/" {
			serveIndex(w, r, publicDir)
			return
		}
		target := filepath.Join(publicDir, strings.TrimPrefix(path, "/"))
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			staticFS.ServeHTTP(w, r)
			return
		}
		serveIndex(w, r, publicDir)
	})

	return &http.Server{
		Addr:    cfg.ListenAddressOrDefault() + ":" + strconv(cfg.ListenPort),
		Handler: cors(mux),
	}
}

func serveIndex(w http.ResponseWriter, r *http.Request, publicDir string) {
	http.ServeFile(w, r, filepath.Join(publicDir, "index.html"))
}

func ensureMethod(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "Method Not Allowed"})
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (s *statusRecorder) WriteHeader(statusCode int) {
	s.statusCode = statusCode
	s.ResponseWriter.WriteHeader(statusCode)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	written, err := s.ResponseWriter.Write(b)
	s.bytesWritten += written
	return written, err
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, x-api-key")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func strconv(port int) string {
	return fmt.Sprintf("%d", port)
}

type requestIDKey struct{}

func withRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

func requestIDFromContext(r *http.Request) string {
	if r == nil {
		return ""
	}
	if value, ok := r.Context().Value(requestIDKey{}).(string); ok {
		return value
	}
	return ""
}

func newRequestID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}
