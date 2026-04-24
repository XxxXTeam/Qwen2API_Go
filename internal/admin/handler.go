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

	batches *batchManager
}

func NewHandler(cfg config.Config, runtime *config.Runtime, keyring *auth.Keyring, accounts *account.Service, openaiHandler *openai.Handler, stats *metrics.DashboardStats, logger *logging.Logger) *Handler {
	return &Handler{
		cfg:      cfg,
		runtime:  runtime,
		keyring:  keyring,
		accounts: accounts,
		openai:   openaiHandler,
		metrics:  stats,
		logger:   logger,
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
	})
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
	h.runtime.SetAutoRefresh(payload.AutoRefresh, payload.AutoRefreshInterval)
	writeJSON(w, http.StatusOK, map[string]any{"status": true, "message": "自动刷新设置更新成功"})
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
	h.runtime.SetBatchLoginConcurrency(payload.BatchLoginConcurrency)
	writeJSON(w, http.StatusOK, map[string]any{"status": true, "message": "批量登录并发数更新成功"})
}

func (h *Handler) HandleSetOutThink(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		OutThink bool `json:"outThink"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体格式错误"})
		return
	}
	h.runtime.SetOutThink(payload.OutThink)
	writeJSON(w, http.StatusOK, map[string]any{"status": true, "message": "思考输出设置更新成功"})
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
	h.runtime.SetSearchInfoMode(payload.SearchInfoMode)
	writeJSON(w, http.StatusOK, map[string]any{"status": true, "message": "搜索信息模式更新成功"})
}

func (h *Handler) HandleSimpleModelMap(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		SimpleModelMap bool `json:"simpleModelMap"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "请求体格式错误"})
		return
	}
	h.runtime.SetSimpleModelMap(payload.SimpleModelMap)
	writeJSON(w, http.StatusOK, map[string]any{"status": true, "message": "简化模型映射设置更新成功"})
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

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
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
