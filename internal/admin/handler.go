package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"qwen2api/internal/account"
	"qwen2api/internal/auth"
	"qwen2api/internal/config"
	"qwen2api/internal/logging"
	"qwen2api/internal/metrics"
	"qwen2api/internal/openai"
	"qwen2api/internal/prompts"
	"qwen2api/internal/storage"
)

type Handler struct {
	cfg      config.Config
	runtime  *config.Runtime
	keyring  *auth.Keyring
	accounts *account.Service
	openai   *openai.Handler
	metrics  *metrics.DashboardStats
	logger   *logging.Logger
	sessions *openai.ConversationSessionService

	batches *batchManager
}

func NewHandler(cfg config.Config, runtime *config.Runtime, keyring *auth.Keyring, accounts *account.Service, openaiHandler *openai.Handler, stats *metrics.DashboardStats, logger *logging.Logger, sessions *openai.ConversationSessionService) *Handler {
	return &Handler{
		cfg:      cfg,
		runtime:  runtime,
		keyring:  keyring,
		accounts: accounts,
		openai:   openaiHandler,
		metrics:  stats,
		logger:   logger,
		sessions: sessions,
		batches:  newBatchManager(),
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func decodeJSON(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

func (h *Handler) HandleVerify(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		APIKey string `json:"apiKey"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"status": 400, "message": "Bad Request"})
		return
	}
	result := h.keyring.Validate(payload.APIKey)
	if !result.IsValid {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"status": 401, "message": "Unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  200,
		"message": "success",
		"isAdmin": result.IsAdmin,
	})
}

func (h *Handler) HandleSettings(w http.ResponseWriter, r *http.Request) {
	keys := h.keyring.Snapshot()
	runtime := h.runtime.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"adminKey":              keys.AdminKey,
		"regularKeys":           keys.RegularKeys,
		"autoRefresh":           runtime.AutoRefresh,
		"autoRefreshInterval":   runtime.AutoRefreshInterval,
		"batchLoginConcurrency": runtime.BatchLoginConcurrency,
		"outThink":              runtime.OutThink,
		"searchInfoMode":        runtime.SearchInfoMode,
		"simpleModelMap":        runtime.SimpleModelMap,
		"chatCleanupMode":       runtime.ChatCleanupMode,
		"qwenWeb2ControlPrompt": runtime.QwenWeb2ControlPrompt,
	})
}

func (h *Handler) HandlePrompts(w http.ResponseWriter, r *http.Request) {
	runtime := h.runtime.Snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"data":       prompts.List(runtime.PromptOverrides),
		"categories": prompts.Categories(),
	})
}

func (h *Handler) HandlePromptsAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.HandlePrompts(w, r)
	case http.MethodPost:
		h.HandleSetPrompts(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "Method Not Allowed"})
	}
}

func (h *Handler) HandleSetPrompts(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Prompts map[string]string `json:"prompts"`
	}
	if err := decodeJSON(r, &payload); err != nil || payload.Prompts == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体格式错误"})
		return
	}

	overrides := prompts.CloneOverrides(h.runtime.Snapshot().PromptOverrides)
	for id, value := range payload.Prompts {
		if !prompts.KnownID(id) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "未知提示词 ID: " + id})
			return
		}
		overrides[id] = value
	}
	overrides = prompts.NormalizeOverrides(overrides)

	if err := h.persistRuntimeSettings(func(snapshot *config.RuntimeSnapshot) {
		snapshot.PromptOverrides = prompts.CloneOverrides(overrides)
		snapshot.QwenWeb2ControlPrompt = prompts.Resolve(overrides, prompts.IDQwenWeb2Control)
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	h.runtime.SetPromptOverrides(overrides)
	writeJSON(w, http.StatusOK, map[string]any{"status": true, "message": "提示词配置已热更新并写入 .env"})
}

func (h *Handler) HandleResetPrompts(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		IDs []string `json:"ids"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体格式错误"})
		return
	}

	overrides := prompts.CloneOverrides(h.runtime.Snapshot().PromptOverrides)
	if len(payload.IDs) == 0 {
		overrides = map[string]string{}
	} else {
		for _, id := range payload.IDs {
			if !prompts.KnownID(id) {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "未知提示词 ID: " + id})
				return
			}
			delete(overrides, id)
		}
	}
	overrides = prompts.NormalizeOverrides(overrides)

	if err := h.persistRuntimeSettings(func(snapshot *config.RuntimeSnapshot) {
		snapshot.PromptOverrides = prompts.CloneOverrides(overrides)
		snapshot.QwenWeb2ControlPrompt = prompts.Resolve(overrides, prompts.IDQwenWeb2Control)
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	h.runtime.SetPromptOverrides(overrides)
	writeJSON(w, http.StatusOK, map[string]any{"status": true, "message": "提示词已恢复默认并写入 .env"})
}

func (h *Handler) HandleAddRegularKey(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		APIKey string `json:"apiKey"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体格式错误"})
		return
	}
	if err := h.keyring.AddRegularKey(payload.APIKey); err != nil {
		status := http.StatusBadRequest
		if err == auth.ErrAPIKeyExists {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "API Key添加成功"})
}

func (h *Handler) HandleDeleteRegularKey(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		APIKey string `json:"apiKey"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体格式错误"})
		return
	}
	if err := h.keyring.DeleteRegularKey(payload.APIKey); err != nil {
		status := http.StatusBadRequest
		switch err {
		case auth.ErrDeleteAdminKey:
			status = http.StatusForbidden
		case auth.ErrAPIKeyNotFound:
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "API Key删除成功"})
}

func (h *Handler) HandleSetAutoRefresh(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		AutoRefresh         bool `json:"autoRefresh"`
		AutoRefreshInterval int  `json:"autoRefreshInterval"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体格式错误"})
		return
	}
	if payload.AutoRefreshInterval < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的自动刷新间隔"})
		return
	}
	if payload.AutoRefreshInterval == 0 {
		payload.AutoRefreshInterval = 6 * 60 * 60
	}
	if err := h.persistRuntimeSettings(func(snapshot *config.RuntimeSnapshot) {
		snapshot.AutoRefresh = payload.AutoRefresh
		snapshot.AutoRefreshInterval = payload.AutoRefreshInterval
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	h.runtime.SetAutoRefresh(payload.AutoRefresh, payload.AutoRefreshInterval)
	writeJSON(w, http.StatusOK, map[string]any{"status": true, "message": "自动刷新设置更新成功，已热更新并写入 .env"})
}

func (h *Handler) HandleSetBatchLoginConcurrency(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		BatchLoginConcurrency int `json:"batchLoginConcurrency"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体格式错误"})
		return
	}
	if payload.BatchLoginConcurrency < 1 || payload.BatchLoginConcurrency > 20 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的批量登录并发数，允许范围为 1-20"})
		return
	}
	if err := h.persistRuntimeSettings(func(snapshot *config.RuntimeSnapshot) {
		snapshot.BatchLoginConcurrency = payload.BatchLoginConcurrency
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	h.runtime.SetBatchLoginConcurrency(payload.BatchLoginConcurrency)
	writeJSON(w, http.StatusOK, map[string]any{"status": true, "message": "批量登录并发数更新成功，已热更新并写入 .env"})
}

func (h *Handler) HandleSetOutThink(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		OutThink bool `json:"outThink"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体格式错误"})
		return
	}
	if err := h.persistRuntimeSettings(func(snapshot *config.RuntimeSnapshot) {
		snapshot.OutThink = payload.OutThink
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	h.runtime.SetOutThink(payload.OutThink)
	writeJSON(w, http.StatusOK, map[string]any{"status": true, "message": "思考输出设置更新成功，已热更新并写入 .env"})
}

func (h *Handler) HandleSearchInfoMode(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		SearchInfoMode string `json:"searchInfoMode"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体格式错误"})
		return
	}
	if payload.SearchInfoMode != "table" && payload.SearchInfoMode != "text" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的搜索信息模式"})
		return
	}
	if err := h.persistRuntimeSettings(func(snapshot *config.RuntimeSnapshot) {
		snapshot.SearchInfoMode = payload.SearchInfoMode
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	h.runtime.SetSearchInfoMode(payload.SearchInfoMode)
	writeJSON(w, http.StatusOK, map[string]any{"status": true, "message": "搜索信息模式更新成功，已热更新并写入 .env"})
}

func (h *Handler) HandleSimpleModelMap(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		SimpleModelMap bool `json:"simpleModelMap"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体格式错误"})
		return
	}
	if err := h.persistRuntimeSettings(func(snapshot *config.RuntimeSnapshot) {
		snapshot.SimpleModelMap = payload.SimpleModelMap
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	h.runtime.SetSimpleModelMap(payload.SimpleModelMap)
	writeJSON(w, http.StatusOK, map[string]any{"status": true, "message": "简化模型映射设置更新成功，已热更新并写入 .env"})
}

func (h *Handler) HandleSetChatCleanupMode(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ChatCleanupMode int `json:"chatCleanupMode"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体格式错误"})
		return
	}
	if payload.ChatCleanupMode < 0 || payload.ChatCleanupMode > 2 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "无效的对话清理模式，允许值为 0-2"})
		return
	}
	if err := h.persistRuntimeSettings(func(snapshot *config.RuntimeSnapshot) {
		snapshot.ChatCleanupMode = payload.ChatCleanupMode
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	h.runtime.SetChatCleanupMode(payload.ChatCleanupMode)
	writeJSON(w, http.StatusOK, map[string]any{"status": true, "message": "对话清理模式更新成功，已热更新并写入 .env"})
}

func (h *Handler) HandleSetQwenWeb2ControlPrompt(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		QwenWeb2ControlPrompt string `json:"qwenWeb2ControlPrompt"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "璇锋眰浣撴牸寮忛敊璇?"})
		return
	}
	prompt := strings.TrimSpace(payload.QwenWeb2ControlPrompt)
	if err := h.persistRuntimeSettings(func(snapshot *config.RuntimeSnapshot) {
		snapshot.QwenWeb2ControlPrompt = prompt
		overrides := prompts.CloneOverrides(snapshot.PromptOverrides)
		overrides[prompts.IDQwenWeb2Control] = prompt
		snapshot.PromptOverrides = prompts.NormalizeOverrides(overrides)
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	h.runtime.SetQwenWeb2ControlPrompt(prompt)
	writeJSON(w, http.StatusOK, map[string]any{"status": true, "message": "Web2 Qwen control prompt updated, hot reloaded and saved to .env"})
}

func (h *Handler) HandleReloadRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	if err := config.ReloadDotEnv(config.DefaultEnvPath); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "重新加载 .env 失败: " + err.Error()})
		return
	}
	loaded := config.Load()
	snapshot := config.RuntimeSnapshotFromConfig(loaded)
	h.runtime.ApplySnapshot(snapshot)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  true,
		"message": "运行配置已从 .env 重新加载并热更新",
		"runtime": snapshot,
	})
}

func (h *Handler) HandleOverview(w http.ResponseWriter, r *http.Request) {
	keys := h.keyring.Snapshot()
	runtime := h.runtime.Snapshot()
	health := h.accounts.BuildHealthStats()

	writeJSON(w, http.StatusOK, map[string]any{
		"server": map[string]any{
			"listenAddress":         h.cfg.ListenAddressOrDefault(),
			"listenPort":            h.cfg.ListenPort,
			"dataSaveMode":          h.cfg.DataSaveMode,
			"cacheMode":             h.cfg.CacheMode,
			"searchInfoMode":        runtime.SearchInfoMode,
			"outThink":              runtime.OutThink,
			"autoRefresh":           runtime.AutoRefresh,
			"autoRefreshInterval":   runtime.AutoRefreshInterval,
			"batchLoginConcurrency": runtime.BatchLoginConcurrency,
			"chatCleanupMode":       runtime.ChatCleanupMode,
			"logLevel":              h.cfg.LogLevel,
			"enableFileLog":         h.cfg.EnableFileLog,
		},
		"apiKeys": map[string]any{
			"total":   len(keys.APIKeys),
			"admin":   boolToInt(keys.AdminKey != ""),
			"regular": len(keys.RegularKeys),
		},
		"accounts": map[string]any{
			"initialized":  health.Initialized,
			"total":        health.Accounts["total"],
			"valid":        health.Accounts["valid"],
			"expiringSoon": health.Accounts["expiringSoon"],
			"expired":      health.Accounts["expired"],
			"invalid":      health.Accounts["invalid"],
		},
		"analytics":   h.metrics.Snapshot(),
		"rotation":    health.Rotation,
		"generatedAt": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *Handler) HandleModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.openai.ListModelVariants(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, h.modelsResponse(models))
}

func (h *Handler) HandleRefreshModels(w http.ResponseWriter, r *http.Request) {
	models, err := h.openai.RefreshModelVariants(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, h.modelsResponse(models))
}

func (h *Handler) modelsResponse(models []map[string]any) map[string]any {
	usageByModel := h.metrics.ModelUsageSnapshot()
	result := make([]map[string]any, 0, len(models))
	for _, item := range models {
		modelID := strings.TrimSpace(fmt.Sprint(item["id"]))
		modelName := strings.TrimSpace(fmt.Sprint(item["name"]))
		upstreamID := strings.TrimSpace(fmt.Sprint(item["upstream_id"]))
		usage := mergeModelUsage(usageByModel, modelID, modelName, upstreamID)

		enriched := make(map[string]any, len(item)+1)
		for key, value := range item {
			enriched[key] = value
		}
		enriched["usage"] = map[string]int{
			"promptTokens":     usage.PromptTokens,
			"completionTokens": usage.CompletionTokens,
			"totalTokens":      usage.TotalTokens,
		}
		result = append(result, enriched)
	}

	return map[string]any{
		"object": "list",
		"data":   result,
	}
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (h *Handler) persistRuntimeSettings(mutator func(snapshot *config.RuntimeSnapshot)) error {
	snapshot := h.runtime.Snapshot()
	mutator(&snapshot)
	if snapshot.AutoRefreshInterval <= 0 {
		snapshot.AutoRefreshInterval = 6 * 60 * 60
	}
	if snapshot.BatchLoginConcurrency <= 0 {
		snapshot.BatchLoginConcurrency = 1
	}
	overrides := prompts.CloneOverrides(snapshot.PromptOverrides)
	overrides[prompts.IDQwenWeb2Control] = snapshot.QwenWeb2ControlPrompt
	snapshot.PromptOverrides = prompts.NormalizeOverrides(overrides)
	if err := config.SaveDotEnvValues(config.DefaultEnvPath, config.RuntimeSnapshotToEnv(snapshot)); err != nil {
		return fmt.Errorf("写入 .env 失败: %w", err)
	}
	return nil
}

func mergeModelUsage(snapshot map[string]metrics.ModelUsage, aliases ...string) metrics.ModelUsage {
	seen := make(map[string]struct{}, len(aliases))
	usage := metrics.ModelUsage{}
	for _, alias := range aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		if _, ok := seen[alias]; ok {
			continue
		}
		seen[alias] = struct{}{}
		item := snapshot[alias]
		usage.PromptTokens += item.PromptTokens
		usage.CompletionTokens += item.CompletionTokens
		usage.TotalTokens += item.TotalTokens
	}
	return usage
}

func (h *Handler) HandleGetAccounts(w http.ResponseWriter, r *http.Request) {
	page := maxInt(1, parseIntDefault(r.URL.Query().Get("page"), 1))
	pageSize := minInt(200, maxInt(10, parseIntDefault(r.URL.Query().Get("pageSize"), 50)))
	keyword := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("keyword")))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "" {
		status = "all"
	}
	sortBy := r.URL.Query().Get("sortBy")
	if sortBy != "email" && sortBy != "status" && sortBy != "expires" {
		sortBy = "expires"
	}
	sortOrder := "desc"
	if r.URL.Query().Get("sortOrder") == "asc" {
		sortOrder = "asc"
	}
	maskSensitive := r.URL.Query().Get("maskSensitive") != "false"

	accounts := h.accounts.ListAccounts()
	overallStats := summarizeAccounts(accounts)
	filtered := make([]storage.Account, 0, len(accounts))
	for _, item := range accounts {
		runtime := account.RuntimeForAccount(item)
		if keyword != "" && !strings.Contains(strings.ToLower(item.Email), keyword) {
			continue
		}
		if status != "all" && runtime.Status != status {
			continue
		}
		filtered = append(filtered, item)
	}

	sort.Slice(filtered, func(i, j int) bool {
		left := filtered[i]
		right := filtered[j]
		var less bool
		switch sortBy {
		case "email":
			less = strings.ToLower(left.Email) < strings.ToLower(right.Email)
		case "status":
			less = account.RuntimeForAccount(left).Status < account.RuntimeForAccount(right).Status
		default:
			less = left.Expires < right.Expires
		}
		if sortOrder == "asc" {
			return less
		}
		return !less
	})

	total := len(filtered)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	data := make([]map[string]any, 0, end-start)
	for _, item := range filtered[start:end] {
		runtime := account.RuntimeForAccount(item)
		password := item.Password
		token := item.Token
		if maskSensitive {
			password = account.MaskSecret(item.Password, 2, 0)
			token = account.MaskSecret(item.Token, 8, 6)
		}
		var expiresAt any = nil
		if runtime.ExpiresAt != "" {
			expiresAt = runtime.ExpiresAt
		}
		data = append(data, map[string]any{
			"email":          item.Email,
			"password":       password,
			"token":          token,
			"expires":        item.Expires,
			"expiresAt":      expiresAt,
			"status":         runtime.Status,
			"remainingHours": runtime.RemainingHours,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"overallStats":  overallStats,
		"filteredStats": summarizeAccounts(filtered),
		"total":         total,
		"page":          page,
		"pageSize":      pageSize,
		"totalPages":    maxInt(1, (total+pageSize-1)/pageSize),
		"keyword":       keyword,
		"status":        status,
		"sortBy":        sortBy,
		"sortOrder":     sortOrder,
		"maskSensitive": maskSensitive,
		"data":          data,
	})
}

func summarizeAccounts(accounts []storage.Account) map[string]int {
	stats := map[string]int{
		"total":        len(accounts),
		"valid":        0,
		"expiringSoon": 0,
		"expired":      0,
		"invalid":      0,
	}
	for _, item := range accounts {
		stats[account.RuntimeForAccount(item).Status]++
	}
	return stats
}

func parseIntDefault(raw string, fallback int) int {
	if value, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
		return value
	}
	return fallback
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (h *Handler) HandleSetAccount(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体格式错误"})
		return
	}
	if err := h.accounts.AddAccount(r.Context(), payload.Email, payload.Password); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "已存在") {
			status = http.StatusConflict
		} else if strings.Contains(err.Error(), "登录") {
			status = http.StatusUnauthorized
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"email": payload.Email, "message": "账号创建成功"})
}

func (h *Handler) HandleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体格式错误"})
		return
	}
	if err := h.accounts.DeleteAccount(payload.Email); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "不存在") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "账号删除成功"})
}

func (h *Handler) HandleRefreshAccount(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体格式错误"})
		return
	}
	if err := h.accounts.RefreshAccount(r.Context(), payload.Email); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "不存在") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "账号令牌刷新成功", "email": payload.Email})
}

func (h *Handler) HandleRefreshAllAccounts(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ThresholdHours int `json:"thresholdHours"`
	}
	_ = decodeJSON(r, &payload)
	if payload.ThresholdHours == 0 {
		payload.ThresholdHours = 24
	}
	refreshed, err := h.accounts.RefreshAllAccounts(r.Context(), payload.ThresholdHours)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message":        "批量刷新完成",
		"refreshedCount": refreshed,
		"thresholdHours": payload.ThresholdHours,
	})
}

func (h *Handler) HandleForceRefreshAllAccounts(w http.ResponseWriter, r *http.Request) {
	refreshed, err := h.accounts.RefreshAllAccounts(r.Context(), 24*365)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message":        "强制刷新完成",
		"refreshedCount": refreshed,
		"totalAccounts":  len(h.accounts.ListAccounts()),
	})
}

type batchTask struct {
	mu           sync.RWMutex
	ID           string           `json:"taskId"`
	Status       string           `json:"status"`
	Message      string           `json:"message"`
	Total        int              `json:"total"`
	Valid        int              `json:"valid"`
	Skipped      int              `json:"skipped"`
	Invalid      int              `json:"invalid"`
	Processed    int              `json:"processed"`
	Completed    int              `json:"completed"`
	Success      int              `json:"success"`
	Failed       int              `json:"failed"`
	Concurrency  int              `json:"concurrency"`
	ActiveEmails []string         `json:"activeEmails"`
	FailedEmails []string         `json:"failedEmails"`
	RecentResult []map[string]any `json:"recentResults"`
	CreatedAt    int64            `json:"createdAt"`
	StartedAt    *int64           `json:"startedAt"`
	FinishedAt   *int64           `json:"finishedAt"`
}

func (t *batchTask) snapshot() map[string]any {
	t.mu.RLock()
	defer t.mu.RUnlock()

	total := maxInt(1, t.Total)
	progress := float64(t.Completed) * 100 / float64(total)

	activeEmails := append([]string(nil), t.ActiveEmails...)
	failedEmails := append([]string(nil), t.FailedEmails...)
	recentResults := append([]map[string]any(nil), t.RecentResult...)
	return map[string]any{
		"taskId":        t.ID,
		"status":        t.Status,
		"message":       t.Message,
		"total":         t.Total,
		"valid":         t.Valid,
		"skipped":       t.Skipped,
		"invalid":       t.Invalid,
		"processed":     t.Processed,
		"completed":     t.Completed,
		"pending":       maxInt(0, t.Total-t.Completed),
		"success":       t.Success,
		"failed":        t.Failed,
		"progress":      progress,
		"concurrency":   t.Concurrency,
		"activeEmails":  activeEmails,
		"failedEmails":  failedEmails,
		"recentResults": recentResults,
		"createdAt":     t.CreatedAt,
		"startedAt":     t.StartedAt,
		"finishedAt":    t.FinishedAt,
	}
}

func (t *batchTask) update(fn func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	fn()
}

type batchManager struct {
	mu    sync.RWMutex
	tasks map[string]*batchTask
}

func newBatchManager() *batchManager {
	return &batchManager{tasks: map[string]*batchTask{}}
}

func (m *batchManager) set(task *batchTask) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[task.ID] = task
}

func (m *batchManager) get(id string) (*batchTask, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, ok := m.tasks[id]
	return task, ok
}

func (h *Handler) HandleSetAccounts(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Accounts string `json:"accounts"`
		Async    bool   `json:"async"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体格式错误"})
		return
	}
	lines := make([]string, 0)
	parsed := make([]storage.Account, 0)
	invalid := 0
	for _, line := range strings.Split(strings.ReplaceAll(payload.Accounts, "\r", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
		email, password, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(email) == "" || strings.TrimSpace(password) == "" {
			invalid++
			continue
		}
		parsed = append(parsed, storage.Account{Email: strings.TrimSpace(email), Password: strings.TrimSpace(password)})
	}
	if len(lines) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "账号列表不能为空"})
		return
	}
	if len(parsed) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "没有符合格式的账号，请使用 email:password"})
		return
	}

	existing := map[string]bool{}
	for _, item := range h.accounts.ListAccounts() {
		existing[strings.ToLower(item.Email)] = true
	}
	newAccounts := make([]storage.Account, 0)
	skipped := 0
	seen := map[string]bool{}
	for _, item := range parsed {
		key := strings.ToLower(item.Email)
		if existing[key] || seen[key] {
			skipped++
			continue
		}
		seen[key] = true
		newAccounts = append(newAccounts, item)
	}

	task := &batchTask{
		ID:           fmt.Sprintf("batch_%d", time.Now().UnixNano()),
		Status:       "pending",
		Message:      "任务已创建，等待执行",
		Total:        len(lines),
		Valid:        len(parsed),
		Skipped:      skipped,
		Invalid:      invalid,
		Completed:    skipped + invalid,
		Concurrency:  h.runtime.Snapshot().BatchLoginConcurrency,
		ActiveEmails: []string{},
		FailedEmails: []string{},
		RecentResult: []map[string]any{},
		CreatedAt:    time.Now().UnixMilli(),
	}
	h.batches.set(task)

	run := func() {
		startedAt := time.Now().UnixMilli()
		task.update(func() {
			task.StartedAt = &startedAt
			task.Status = "running"
			task.Message = fmt.Sprintf("正在处理 %d/%d", task.Completed, task.Total)
		})

		concurrency := maxInt(1, task.Concurrency)
		for i := 0; i < len(newAccounts); i += concurrency {
			end := i + concurrency
			if end > len(newAccounts) {
				end = len(newAccounts)
			}
			var wg sync.WaitGroup
			for _, item := range newAccounts[i:end] {
				wg.Add(1)
				accountItem := item
				go func() {
					defer wg.Done()
					task.update(func() {
						task.ActiveEmails = append(task.ActiveEmails, accountItem.Email)
					})
					err := h.accounts.AddAccount(context.Background(), accountItem.Email, accountItem.Password)
					task.update(func() {
						task.Processed++
						task.Completed++
						task.ActiveEmails = removeString(task.ActiveEmails, accountItem.Email)
						if err != nil {
							task.Failed++
							task.FailedEmails = appendIfMissing(task.FailedEmails, accountItem.Email)
							task.RecentResult = append([]map[string]any{{
								"email":   accountItem.Email,
								"status":  "failed",
								"message": err.Error(),
							}}, task.RecentResult...)
						} else {
							task.Success++
							task.RecentResult = append([]map[string]any{{
								"email":   accountItem.Email,
								"status":  "success",
								"message": "登录成功",
							}}, task.RecentResult...)
						}
						if len(task.RecentResult) > 12 {
							task.RecentResult = task.RecentResult[:12]
						}
						task.Message = fmt.Sprintf("正在处理 %d/%d", task.Completed, task.Total)
					})
				}()
			}
			wg.Wait()
		}

		finishedAt := time.Now().UnixMilli()
		task.update(func() {
			task.Status = "completed"
			task.Message = fmt.Sprintf("批量添加完成，成功 %d 个，失败 %d 个", task.Success, task.Failed)
			task.FinishedAt = &finishedAt
		})
	}

	if payload.Async {
		go run()
		writeJSON(w, http.StatusAccepted, task.snapshot())
		return
	}

	run()
	writeJSON(w, http.StatusOK, task.snapshot())
}

func removeString(items []string, target string) []string {
	filtered := items[:0]
	for _, item := range items {
		if item != target {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func appendIfMissing(items []string, target string) []string {
	for _, item := range items {
		if item == target {
			return items
		}
	}
	return append(items, target)
}

func (h *Handler) HandleBatchTask(w http.ResponseWriter, r *http.Request) {
	taskID := strings.TrimPrefix(r.URL.Path, "/api/batchTasks/")
	task, ok := h.batches.get(taskID)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "任务不存在或已过期"})
		return
	}
	writeJSON(w, http.StatusOK, task.snapshot())
}

func (h *Handler) HandleDashboardStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "Streaming unsupported"})
		return
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			keys := h.keyring.Snapshot()
			health := h.accounts.BuildHealthStats()
			payload, _ := json.Marshal(map[string]any{
				"event": "snapshot",
				"data": map[string]any{
					"accounts": map[string]any{
						"initialized":  health.Initialized,
						"total":        health.Accounts["total"],
						"valid":        health.Accounts["valid"],
						"expiringSoon": health.Accounts["expiringSoon"],
						"expired":      health.Accounts["expired"],
						"invalid":      health.Accounts["invalid"],
					},
					"apiKeys": map[string]any{
						"total":   len(keys.APIKeys),
						"admin":   boolToInt(keys.AdminKey != ""),
						"regular": len(keys.RegularKeys),
					},
					"analytics":   h.metrics.Snapshot(),
					"generatedAt": time.Now().UTC().Format(time.RFC3339),
				},
			})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}
}

// HandleSessions returns a list of all active conversation sessions
func (h *Handler) HandleSessions(w http.ResponseWriter, r *http.Request) {
	if h.sessions == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "会话服务未初始化"})
		return
	}

	sessions, err := h.sessions.ListAll()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	// Enrich sessions with additional info
	result := make([]map[string]any, 0, len(sessions))
	for _, s := range sessions {
		result = append(result, map[string]any{
			"context_hash":  s.ContextHash,
			"account_email": s.AccountEmail,
			"chat_id":       s.ChatID,
			"model":         s.Model,
			"chat_type":     s.ChatType,
			"updated_at":    s.UpdatedAt,
			"updated_time":  time.UnixMilli(s.UpdatedAt).Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":    len(result),
		"sessions": result,
	})
}

// HandleSessionChat retrieves chat history for a specific session by chat_id
func (h *Handler) HandleSessionChat(w http.ResponseWriter, r *http.Request) {
	if h.sessions == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "会话服务未初始化"})
		return
	}

	chatID := r.URL.Query().Get("chat_id")
	if chatID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "缺少 chat_id 参数"})
		return
	}

	// Find session by chat_id
	sessions, err := h.sessions.ListAll()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	var found *storage.ConversationSession
	for i := range sessions {
		if sessions[i].ChatID == chatID {
			found = &sessions[i]
			break
		}
	}

	if found == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "会话未找到"})
		return
	}

	// Get cached messages from chat tracker if available
	messages := []map[string]any{}
	// Note: Full chat history retrieval would require calling Qwen API
	// For now, we return session metadata and indicate that full history requires API call

	writeJSON(w, http.StatusOK, map[string]any{
		"session": map[string]any{
			"context_hash":  found.ContextHash,
			"account_email": found.AccountEmail,
			"chat_id":       found.ChatID,
			"model":         found.Model,
			"chat_type":     found.ChatType,
			"updated_at":    found.UpdatedAt,
			"updated_time":  time.UnixMilli(found.UpdatedAt).Format(time.RFC3339),
		},
		"messages":          messages,
		"note":              "完整聊天记录需要从 Qwen API 获取，当前仅返回会话元数据",
		"web_interface_url": "https://chat.qwen.ai/c/" + found.ChatID,
	})
}

// HandleDeleteSession deletes a conversation session
func (h *Handler) HandleDeleteSession(w http.ResponseWriter, r *http.Request) {
	if h.sessions == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "会话服务未初始化"})
		return
	}

	var payload struct {
		ContextHash string `json:"context_hash"`
	}
	if err := decodeJSON(r, &payload); err != nil || payload.ContextHash == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体格式错误或缺少 context_hash"})
		return
	}

	h.sessions.Delete(payload.ContextHash)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  true,
		"message": "会话已删除",
	})
}
