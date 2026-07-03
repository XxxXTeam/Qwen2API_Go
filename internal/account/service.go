package account

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"qwen2api/internal/config"
	"qwen2api/internal/logging"
	"qwen2api/internal/qwen"
	"qwen2api/internal/storage"
)

type RuntimeState struct {
	Status         string  `json:"status"`
	RemainingHours float64 `json:"remainingHours"`
	ExpiresAt      string  `json:"expiresAt"`
}

type RotationStats struct {
	Total        int                    `json:"total"`
	Available    int                    `json:"available"`
	InCooldown   int                    `json:"inCooldown"`
	CurrentIndex int                    `json:"currentIndex"`
	UsageStats   map[string]UsageStatus `json:"usageStats"`
}

type UsageStatus struct {
	Failures  int   `json:"failures"`
	LastUsed  int64 `json:"lastUsed"`
	Available bool  `json:"available"`
}

type HealthStats struct {
	Initialized bool           `json:"initialized"`
	Accounts    map[string]int `json:"accounts"`
	Rotation    RotationStats  `json:"rotation"`
}

const guestAccountEmail = "guest"

const accountBrowserWarmupInterval = 2 * time.Second

type Service struct {
	cfg      config.Config
	runtime  *config.Runtime
	store    storage.AccountStore
	client   *qwen.Client
	logger   *logging.Logger
	tokenMgr *tokenManager

	mu           sync.RWMutex
	accounts     []storage.Account
	initialized  bool
	currentIndex int
	lastUsed     map[string]time.Time
	failures     map[string]int

	refreshStop chan struct{}
}

func NewService(cfg config.Config, runtime *config.Runtime, store storage.AccountStore, client *qwen.Client, logger *logging.Logger) *Service {
	return &Service{
		cfg:         cfg,
		runtime:     runtime,
		store:       store,
		client:      client,
		logger:      logger,
		tokenMgr:    &tokenManager{client: client, logger: logger},
		lastUsed:    map[string]time.Time{},
		failures:    map[string]int{},
		refreshStop: make(chan struct{}),
	}
}

func (s *Service) Initialize(ctx context.Context) error {
	s.logger.InfoModule("ACCOUNT", "account initialize started")
	loadedAccounts, err := s.store.LoadAccounts()
	if err != nil {
		s.logger.ErrorModule("ACCOUNT", "account initialize load failed: %v", err)
		return err
	}
	if sessions, err := s.store.LoadBrowserSessions(); err == nil {
		s.client.RestoreBrowserSessions(browserSessionsFromStorage(sessions))
	} else if !isReadonlyStoreError(err) {
		s.logger.WarnModule("ACCOUNT", "加载浏览器会话失败: %v", err)
	}
	if s.logger.IsDebug() {
		s.logger.DebugModule("ACCOUNT", "account initialize loaded total=%d", len(loadedAccounts))
	}

	validated := make([]storage.Account, 0, len(loadedAccounts))
	for _, account := range loadedAccounts {
		normalized, ok := s.ensureAccountReady(ctx, account)
		if ok {
			validated = append(validated, normalized)
		}
	}
	if len(validated) == 0 {
		if guest, ok := s.ensureGuestReady(ctx); ok {
			validated = append(validated, guest)
		}
	}

	s.mu.Lock()
	s.accounts = validated
	s.initialized = true
	s.mu.Unlock()

	if err := s.store.SaveAllAccounts(mergePersistentAccounts(loadedAccounts, validated)); err != nil && !isReadonlyStoreError(err) {
		s.logger.WarnModule("ACCOUNT", "保存初始化后的账号状态失败: %v", err)
	}

	go s.runAutoRefresh()
	go s.warmBrowserSessions(filterPersistentAccounts(validated))

	s.logger.InfoModule("ACCOUNT", "account initialize completed available=%d", len(validated))
	return nil
}

func isReadonlyStoreError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "不支持")
}

func (s *Service) Close() {
	close(s.refreshStop)
}

func (s *Service) ensureAccountReady(ctx context.Context, account storage.Account) (storage.Account, bool) {
	if account.IsGuest() {
		return s.ensureGuestReady(ctx)
	}
	if valid, _, ok := decodeJWTExpiry(account.Token); ok && valid {
		account.Expires, _ = decodeExpiry(account.Token)
		return account, true
	}
	if strings.TrimSpace(account.Email) == "" || strings.TrimSpace(account.Password) == "" {
		return storage.Account{}, false
	}
	token, err := s.tokenMgr.Login(ctx, account.Email, account.Password)
	if err != nil {
		s.logger.WarnModule("ACCOUNT", "账号 %s 登录失败: %v", account.Email, err)
		return storage.Account{}, false
	}
	exp, err := decodeExpiry(token)
	if err != nil {
		s.logger.WarnModule("ACCOUNT", "账号 %s token 解析失败: %v", account.Email, err)
		return storage.Account{}, false
	}
	account.Token = token
	account.Expires = exp
	return account, true
}

func (s *Service) ensureGuestReady(ctx context.Context) (storage.Account, bool) {
	if _, err := s.client.EnsureGuestCookieHeader(ctx); err != nil {
		s.logger.WarnModule("ACCOUNT", "游客 cookie 预热失败，将在首次请求时继续重试: %v", err)
	}
	return storage.Account{
		Email:  guestAccountEmail,
		Source: storage.AccountSourceGuest,
	}, true
}

func filterPersistentAccounts(accounts []storage.Account) []storage.Account {
	filtered := make([]storage.Account, 0, len(accounts))
	for _, account := range accounts {
		if account.IsGuest() {
			continue
		}
		filtered = append(filtered, account)
	}
	return filtered
}

func mergePersistentAccounts(loaded []storage.Account, validated []storage.Account) []storage.Account {
	persisted := make([]storage.Account, 0, len(loaded)+len(validated))
	indexByEmail := map[string]int{}

	for _, account := range loaded {
		if account.IsGuest() {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(account.Email))
		if key == "" {
			continue
		}
		indexByEmail[key] = len(persisted)
		persisted = append(persisted, account)
	}

	for _, account := range validated {
		if account.IsGuest() {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(account.Email))
		if key == "" {
			continue
		}
		if index, ok := indexByEmail[key]; ok {
			persisted[index] = account
			continue
		}
		indexByEmail[key] = len(persisted)
		persisted = append(persisted, account)
	}

	return persisted
}

func decodeExpiry(token string) (int64, error) {
	_, exp, ok := decodeJWTExpiry(token)
	if !ok {
		return 0, errors.New("token 无效")
	}
	return exp, nil
}

func decodeJWTExpiry(token string) (bool, int64, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return false, 0, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false, 0, false
	}
	var body struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return false, 0, false
	}
	if body.Exp <= 0 {
		return false, 0, false
	}
	return body.Exp > time.Now().Unix(), body.Exp, true
}

type tokenManager struct {
	client *qwen.Client
	logger *logging.Logger
}

func (m *tokenManager) Login(ctx context.Context, email string, password string) (string, error) {
	hash := sha256.Sum256([]byte(password))
	return m.client.SignIn(ctx, email, hex.EncodeToString(hash[:]))
}

func (s *Service) runAutoRefresh() {
	for {
		snapshot := s.currentRuntime()
		waitSeconds := snapshot.AutoRefreshInterval
		if waitSeconds <= 0 {
			waitSeconds = 6 * 60 * 60
		}
		if !snapshot.AutoRefresh {
			waitSeconds = 30
		}

		select {
		case <-time.After(time.Duration(waitSeconds) * time.Second):
			if !snapshot.AutoRefresh {
				continue
			}
			_, _ = s.RefreshAllAccounts(context.Background(), 24)
		case <-s.refreshStop:
			return
		}
	}
}

func (s *Service) warmBrowserSessions(accounts []storage.Account) {
	if !s.client.BrowserAuthEnabled() || len(accounts) == 0 {
		return
	}

	s.logger.InfoModule("ACCOUNT", "账号浏览器会话后台预热开始 total=%d", len(accounts))
	for index, account := range accounts {
		if index > 0 {
			select {
			case <-time.After(accountBrowserWarmupInterval):
			case <-s.refreshStop:
				return
			}
		}
		s.refreshBrowserSessionForAccount(context.Background(), account)
		if s.logger.IsDebug() {
			s.logger.DebugModule("ACCOUNT", "账号浏览器会话后台预热进度 %d/%d email=%s", index+1, len(accounts), account.Email)
		}
	}
	s.logger.InfoModule("ACCOUNT", "账号浏览器会话后台预热完成 total=%d", len(accounts))
}

func (s *Service) currentRuntime() config.RuntimeSnapshot {
	if s.runtime == nil {
		return config.RuntimeSnapshot{
			BatchLoginConcurrency: s.cfg.BatchLoginConcurrency,
			AutoRefresh:           s.cfg.AutoRefresh,
			AutoRefreshInterval:   s.cfg.AutoRefreshInterval,
			OutThink:              s.cfg.OutThink,
			SearchInfoMode:        s.cfg.SearchInfoMode,
			SimpleModelMap:        s.cfg.SimpleModelMap,
		}
	}
	return s.runtime.Snapshot()
}

func (s *Service) Accounts() []storage.Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]storage.Account(nil), s.accounts...)
}

func (s *Service) ListAccounts() []storage.Account {
	return s.Accounts()
}

func (s *Service) AddAccount(ctx context.Context, email, password string, cookieHeader ...string) error {
	if strings.TrimSpace(email) == "" || strings.TrimSpace(password) == "" {
		return errors.New("邮箱和密码不能为空")
	}
	cookie := optionalCookie(cookieHeader)

	s.mu.RLock()
	for _, account := range s.accounts {
		if strings.EqualFold(account.Email, email) {
			s.mu.RUnlock()
			return errors.New("账号已存在")
		}
	}
	s.mu.RUnlock()

	token, err := s.tokenMgr.Login(ctx, email, password)
	if err != nil {
		return err
	}
	exp, err := decodeExpiry(token)
	if err != nil {
		return err
	}
	if err := s.AddAccountWithToken(email, password, token, exp, cookie); err != nil {
		return err
	}
	s.refreshBrowserSessionForAccount(ctx, storage.Account{
		Email:    email,
		Password: password,
		Token:    token,
		Cookie:   cookie,
		Expires:  exp,
	})
	return nil
}

func (s *Service) AddAccountWithToken(email, password, token string, expires int64, cookieHeader ...string) error {
	account := storage.Account{
		Email:    email,
		Password: password,
		Token:    token,
		Cookie:   optionalCookie(cookieHeader),
		Expires:  expires,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	previous := append([]storage.Account(nil), s.accounts...)
	for _, existing := range s.accounts {
		if !strings.EqualFold(existing.Email, email) {
			continue
		}
		return errors.New("账号已存在")
	}
	filtered := s.accounts[:0]
	for _, existing := range s.accounts {
		if existing.IsGuest() {
			continue
		}
		filtered = append(filtered, existing)
	}
	s.accounts = filtered
	s.accounts = append(s.accounts, account)
	if err := s.store.SaveAccount(account); err != nil && !isReadonlyStoreError(err) {
		s.accounts = previous
		return err
	}
	return nil
}

func (s *Service) DeleteAccount(email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	index := -1
	for i, account := range s.accounts {
		if strings.EqualFold(account.Email, email) {
			if account.IsGuest() {
				return errors.New("游客账号不可删除")
			}
			index = i
			break
		}
	}
	if index == -1 {
		return errors.New("账号不存在")
	}

	s.accounts = append(s.accounts[:index], s.accounts[index+1:]...)
	delete(s.failures, email)
	delete(s.lastUsed, email)
	if len(filterPersistentAccounts(s.accounts)) == 0 {
		if guest, ok := s.ensureGuestReady(context.Background()); ok {
			s.accounts = append(s.accounts, guest)
		}
	}
	if err := s.store.DeleteAccount(email); err != nil && !isReadonlyStoreError(err) {
		return err
	}
	return nil
}

func (s *Service) RefreshAccount(ctx context.Context, email string) error {
	s.mu.Lock()
	var refreshed storage.Account
	for i, account := range s.accounts {
		if !strings.EqualFold(account.Email, email) {
			continue
		}
		if account.IsGuest() {
			if _, err := s.client.RefreshGuestCookieHeader(ctx); err != nil {
				s.mu.Unlock()
				return err
			}
			s.saveBrowserSessions()
			s.failures[email] = 0
			s.mu.Unlock()
			return nil
		}
		token, err := s.tokenMgr.Login(ctx, account.Email, account.Password)
		if err != nil {
			s.mu.Unlock()
			return err
		}
		exp, err := decodeExpiry(token)
		if err != nil {
			s.mu.Unlock()
			return err
		}
		s.accounts[i].Token = token
		s.accounts[i].Expires = exp
		s.failures[email] = 0
		refreshed = s.accounts[i]
		if err := s.store.SaveAccount(refreshed); err != nil {
			s.mu.Unlock()
			return err
		}
		s.mu.Unlock()
		s.refreshBrowserSessionForAccount(ctx, refreshed)
		return nil
	}
	s.mu.Unlock()
	return errors.New("账号不存在")
}

func (s *Service) CaptureGuestBrowserSession(ctx context.Context) (*qwen.BrowserSession, error) {
	session, err := s.client.CaptureGuestBrowserSession(ctx)
	if err != nil {
		return nil, err
	}
	s.saveBrowserSessions()
	return session, nil
}

func (s *Service) CaptureBrowserSessionWithCookie(ctx context.Context, cookieHeader string) (*qwen.BrowserSession, error) {
	session, err := s.client.CaptureBrowserSessionWithCookie(ctx, cookieHeader)
	if err != nil {
		return nil, err
	}
	s.saveBrowserSessions()
	return session, nil
}

func (s *Service) BrowserSessionSnapshot() map[string]any {
	return s.client.BrowserSessionSnapshot()
}

func (s *Service) saveBrowserSessions() {
	if err := s.store.SaveBrowserSessions(browserSessionsToStorage(s.client.BrowserSessions())); err != nil && !isReadonlyStoreError(err) {
		s.logger.WarnModule("ACCOUNT", "保存浏览器会话失败: %v", err)
	}
}

func (s *Service) RefreshAllAccounts(ctx context.Context, thresholdHours int) (int, error) {
	if thresholdHours <= 0 {
		thresholdHours = 24
	}

	accounts := s.Accounts()
	refreshed := 0
	for _, account := range accounts {
		if account.IsGuest() {
			if err := s.RefreshAccount(ctx, account.Email); err == nil {
				refreshed++
			}
			continue
		}
		if remainingHours(account.Expires) > float64(thresholdHours) {
			continue
		}
		if err := s.RefreshAccount(ctx, account.Email); err == nil {
			refreshed++
		}
	}
	return refreshed, nil
}

func (s *Service) GetAccountSession() (storage.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.initialized {
		return storage.Account{}, errors.New("账户管理器尚未初始化")
	}
	if len(s.accounts) == 0 {
		return storage.Account{}, errors.New("没有可用的账户令牌")
	}

	available := s.availableLocked()
	if len(available) == 0 {
		account := s.roundRobinLocked()
		if account.Email == "" {
			return storage.Account{}, errors.New("所有账户令牌都不可用")
		}
		s.lastUsed[account.Email] = time.Now()
		return account, nil
	}

	selected := available[0]
	selectedLastUsed := s.lastUsed[selected.Email]
	for _, account := range available[1:] {
		if s.lastUsed[account.Email].Before(selectedLastUsed) || selectedLastUsed.IsZero() {
			selected = account
			selectedLastUsed = s.lastUsed[account.Email]
		}
	}
	s.lastUsed[selected.Email] = time.Now()
	return selected, nil
}

func (s *Service) GetAccountSessionByEmail(email string) (storage.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.initialized {
		return storage.Account{}, errors.New("账户管理器尚未初始化")
	}
	for _, account := range s.availableLocked() {
		if strings.EqualFold(account.Email, strings.TrimSpace(email)) {
			s.lastUsed[account.Email] = time.Now()
			return account, nil
		}
	}
	return storage.Account{}, errors.New("指定账号当前不可用")
}

func (s *Service) GetAccountSessionExcluding(excluded map[string]struct{}) (storage.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.initialized {
		return storage.Account{}, errors.New("账户管理器尚未初始化")
	}
	if len(s.accounts) == 0 {
		return storage.Account{}, errors.New("没有可用的账户令牌")
	}

	filtered := make([]storage.Account, 0, len(s.accounts))
	for _, account := range s.availableLocked() {
		if _, ok := excluded[account.Email]; ok {
			continue
		}
		filtered = append(filtered, account)
	}
	if len(filtered) == 0 {
		return storage.Account{}, errors.New("没有符合条件的可用账户令牌")
	}

	selected := filtered[0]
	selectedLastUsed := s.lastUsed[selected.Email]
	for _, account := range filtered[1:] {
		if s.lastUsed[account.Email].Before(selectedLastUsed) || selectedLastUsed.IsZero() {
			selected = account
			selectedLastUsed = s.lastUsed[account.Email]
		}
	}
	s.lastUsed[selected.Email] = time.Now()
	return selected, nil
}

func (s *Service) availableLocked() []storage.Account {
	result := make([]storage.Account, 0, len(s.accounts))
	now := time.Now()
	for _, account := range s.accounts {
		if !account.IsGuest() && strings.TrimSpace(account.Token) == "" {
			continue
		}
		failures := s.failures[account.Email]
		if failures >= 3 {
			lastUsed := s.lastUsed[account.Email]
			if !lastUsed.IsZero() && now.Sub(lastUsed) < 5*time.Minute {
				continue
			}
			s.failures[account.Email] = 0
		}
		result = append(result, account)
	}
	return result
}

func (s *Service) roundRobinLocked() storage.Account {
	for range s.accounts {
		if s.currentIndex >= len(s.accounts) {
			s.currentIndex = 0
		}
		account := s.accounts[s.currentIndex]
		s.currentIndex++
		if account.IsGuest() || strings.TrimSpace(account.Token) != "" {
			return account
		}
	}
	return storage.Account{}
}

func (s *Service) RecordFailure(email string) {
	s.RecordFailureAndRefresh(context.Background(), email)
}

func (s *Service) RecordFailureAndRefresh(ctx context.Context, email string) {
	s.mu.Lock()
	s.failures[email]++
	s.lastUsed[email] = time.Now()
	account, ok := s.accountByEmailLocked(email)
	s.mu.Unlock()
	if ok {
		s.refreshBrowserSessionForAccount(ctx, account)
	}
}

func (s *Service) ResetFailure(email string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures[email] = 0
}

func (s *Service) accountByEmailLocked(email string) (storage.Account, bool) {
	for _, account := range s.accounts {
		if strings.EqualFold(account.Email, email) {
			return account, true
		}
	}
	return storage.Account{}, false
}

func (s *Service) refreshBrowserSessionForAccount(ctx context.Context, account storage.Account) {
	if !s.client.BrowserAuthEnabled() {
		return
	}
	if account.IsGuest() {
		if _, err := s.client.CaptureGuestBrowserSession(ctx); err != nil {
			s.logger.WarnModule("ACCOUNT", "游客浏览器会话采集失败: %v", err)
			return
		}
		s.saveBrowserSessions()
		return
	}
	cookieHeader := accountBrowserCookieHeader(account)
	if strings.TrimSpace(cookieHeader) == "" {
		return
	}
	if _, err := s.client.CaptureBrowserSessionForAccount(ctx, account.BrowserSessionKey(), cookieHeader); err != nil {
		s.logger.WarnModule("ACCOUNT", "账号浏览器会话采集失败 email=%s err=%v", account.Email, err)
		return
	}
	s.saveBrowserSessions()
}

func accountBrowserCookieHeader(account storage.Account) string {
	cookies := map[string]string{}
	mergeAccountCookieHeader(cookies, account.Cookie)
	token := strings.TrimSpace(account.Token)
	if token != "" {
		cookies["token"] = token
	}
	return formatAccountCookieMap(cookies)
}

func optionalCookie(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func mergeAccountCookieHeader(cookies map[string]string, header string) {
	for _, part := range strings.Split(header, ";") {
		name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" || value == "" {
			continue
		}
		cookies[name] = value
	}
}

func formatAccountCookieMap(cookies map[string]string) string {
	if len(cookies) == 0 {
		return ""
	}
	names := make([]string, 0, len(cookies))
	for name, value := range cookies {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+"="+cookies[name])
	}
	return strings.Join(parts, "; ")
}

func remainingHours(expires int64) float64 {
	if expires <= 0 {
		return -1
	}
	remaining := time.Unix(expires, 0).Sub(time.Now()).Hours()
	return float64(int(remaining*10)) / 10
}

func RuntimeForAccount(account storage.Account) RuntimeState {
	if account.IsGuest() {
		return RuntimeState{
			Status:         "valid",
			RemainingHours: -1,
			ExpiresAt:      "",
		}
	}
	if strings.TrimSpace(account.Token) == "" || account.Expires <= 0 {
		return RuntimeState{
			Status:         "invalid",
			RemainingHours: -1,
			ExpiresAt:      "",
		}
	}
	expAt := time.Unix(account.Expires, 0)
	remaining := expAt.Sub(time.Now())
	if remaining <= 0 {
		return RuntimeState{
			Status:         "expired",
			RemainingHours: 0,
			ExpiresAt:      expAt.UTC().Format(time.RFC3339),
		}
	}
	hours := float64(int(remaining.Hours()*10)) / 10
	status := "valid"
	if remaining <= 6*time.Hour {
		status = "expiringSoon"
	}
	return RuntimeState{
		Status:         status,
		RemainingHours: hours,
		ExpiresAt:      expAt.UTC().Format(time.RFC3339),
	}
}

func browserSessionsToStorage(sessions map[string]qwen.BrowserSession) map[string]storage.BrowserSession {
	result := make(map[string]storage.BrowserSession, len(sessions))
	for kind, session := range sessions {
		result[kind] = storage.BrowserSession{
			Headers:     headerToStorage(session.Headers),
			Cookie:      session.Cookie,
			CapturedAt:  session.CapturedAt,
			Guest:       session.Guest,
			SourceURL:   session.SourceURL,
			UserAgent:   session.UserAgent,
			HasCookie:   session.HasCookie,
			CookieNames: append([]string(nil), session.CookieNames...),
		}
	}
	return result
}

func browserSessionsFromStorage(sessions map[string]storage.BrowserSession) map[string]qwen.BrowserSession {
	result := make(map[string]qwen.BrowserSession, len(sessions))
	for kind, session := range sessions {
		result[kind] = qwen.BrowserSession{
			Headers:     headerFromStorage(session.Headers),
			Cookie:      session.Cookie,
			CapturedAt:  session.CapturedAt,
			Guest:       session.Guest,
			SourceURL:   session.SourceURL,
			UserAgent:   session.UserAgent,
			HasCookie:   session.HasCookie,
			CookieNames: append([]string(nil), session.CookieNames...),
		}
	}
	return result
}

func headerToStorage(headers http.Header) map[string][]string {
	if len(headers) == 0 {
		return map[string][]string{}
	}
	result := make(map[string][]string, len(headers))
	for key, values := range headers {
		result[key] = append([]string(nil), values...)
	}
	return result
}

func headerFromStorage(headers map[string][]string) http.Header {
	if len(headers) == 0 {
		return http.Header{}
	}
	result := make(http.Header, len(headers))
	for key, values := range headers {
		result[key] = append([]string(nil), values...)
	}
	return result
}

func (s *Service) BuildHealthStats() HealthStats {
	accounts := s.Accounts()
	stats := map[string]int{
		"total":        len(accounts),
		"valid":        0,
		"expiringSoon": 0,
		"expired":      0,
		"invalid":      0,
	}
	for _, account := range accounts {
		runtime := RuntimeForAccount(account)
		stats[runtime.Status]++
	}
	return HealthStats{
		Initialized: s.initialized,
		Accounts:    stats,
		Rotation:    s.rotationStats(),
	}
}

func (s *Service) rotationStats() RotationStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	usage := make(map[string]UsageStatus, len(s.accounts))
	available := 0
	now := time.Now()
	for _, account := range s.accounts {
		lastUsed := s.lastUsed[account.Email]
		failures := s.failures[account.Email]
		ok := account.IsGuest() || strings.TrimSpace(account.Token) != ""
		if ok && failures >= 3 && !lastUsed.IsZero() && now.Sub(lastUsed) < 5*time.Minute {
			ok = false
		}
		if ok {
			available++
		}
		usage[account.Email] = UsageStatus{
			Failures:  failures,
			LastUsed:  lastUsed.UnixMilli(),
			Available: ok,
		}
	}
	return RotationStats{
		Total:        len(s.accounts),
		Available:    available,
		InCooldown:   len(s.accounts) - available,
		CurrentIndex: s.currentIndex,
		UsageStats:   usage,
	}
}

func MaskSecret(value string, start, end int) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	if len(text) <= start+end {
		half := len(text) / 2
		if half == 0 {
			half = 1
		}
		return text[:half] + "***"
	}
	return fmt.Sprintf("%s***%s", text[:start], text[len(text)-end:])
}
