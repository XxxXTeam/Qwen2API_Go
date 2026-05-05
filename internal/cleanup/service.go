package cleanup

import (
	"context"
	"strings"
	"sync"
	"time"

	"qwen2api/internal/account"
	"qwen2api/internal/config"
	"qwen2api/internal/logging"
	"qwen2api/internal/qwen"
	"qwen2api/internal/storage"
)

const cleanupInterval = 1 * time.Hour
const cleanupThreshold = 24 * time.Hour

type Service struct {
	cfg        config.Config
	runtime    *config.Runtime
	accounts   *account.Service
	client     *qwen.Client
	tracker    storage.ChatTracker
	logger     *logging.Logger
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

func NewService(cfg config.Config, runtime *config.Runtime, accounts *account.Service, client *qwen.Client, tracker storage.ChatTracker, logger *logging.Logger) *Service {
	return &Service{
		cfg:      cfg,
		runtime:  runtime,
		accounts: accounts,
		client:   client,
		tracker:  tracker,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

func (s *Service) Start() {
	s.wg.Add(1)
	go s.run()
}

func (s *Service) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *Service) run() {
	defer s.wg.Done()

	// Run immediately on start, then every hour.
	s.runOnce()

	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runOnce()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Service) runOnce() {
	mode := s.currentMode()
	if mode == 0 {
		return
	}

	s.logger.InfoModule("CLEANUP", "开始对话清理, 模式=%d", mode)

	if mode == 1 {
		s.cleanupProgramChats()
	} else if mode == 2 {
		s.cleanupAllChats()
	}
}

func (s *Service) currentMode() int {
	if s.runtime != nil {
		return s.runtime.Snapshot().ChatCleanupMode
	}
	return s.cfg.ChatCleanupMode
}

// Mode 1: delete only chats that were created by this program and unused for >24h.
func (s *Service) cleanupProgramChats() {
	if s.tracker == nil {
		return
	}
	usages, err := s.tracker.ListChatUsages()
	if err != nil {
		s.logger.WarnModule("CLEANUP", "读取程序对话记录失败: %v", err)
		return
	}

	cutoff := time.Now().Add(-cleanupThreshold).Unix()
	deleted := 0
	failed := 0

	for _, usage := range usages {
		if usage.UpdatedAt > cutoff {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(usage.AccountEmail), storage.AccountSourceGuest) {
			_ = s.tracker.DeleteChatUsage(usage.AccountEmail, usage.ChatID)
			continue
		}

		account, err := s.accounts.GetAccountSessionByEmail(usage.AccountEmail)
		if err != nil {
			s.logger.WarnModule("CLEANUP", "获取账号失败 email=%s err=%v", usage.AccountEmail, err)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err = s.client.DeleteChat(ctx, account.Token, usage.ChatID)
		cancel()

		if err != nil {
			s.logger.WarnModule("CLEANUP", "删除对话失败 email=%s chat_id=%s err=%v", usage.AccountEmail, usage.ChatID, err)
			failed++
		} else {
			s.logger.InfoModule("CLEANUP", "删除对话成功 email=%s chat_id=%s", usage.AccountEmail, usage.ChatID)
			_ = s.tracker.DeleteChatUsage(usage.AccountEmail, usage.ChatID)
			deleted++
		}
	}

	s.logger.InfoModule("CLEANUP", "程序对话清理完成 删除=%d 失败=%d", deleted, failed)
}

// Mode 2: delete ALL chats unused for >24h by listing them via API.
func (s *Service) cleanupAllChats() {
	accounts := s.accounts.Accounts()
	for _, acc := range accounts {
		if acc.IsGuest() || strings.TrimSpace(acc.Token) == "" {
			continue
		}
		s.cleanupAccountAllChats(acc)
	}
}

func (s *Service) cleanupAccountAllChats(acc storage.Account) {
	cutoff := time.Now().Add(-cleanupThreshold).Unix()
	deleted := 0
	failed := 0
	page := 1

	for {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		chats, err := s.client.ListChats(ctx, acc.Token, page)
		cancel()
		if err != nil {
			s.logger.WarnModule("CLEANUP", "列出对话失败 email=%s page=%d err=%v", acc.Email, page, err)
			break
		}
		if len(chats) == 0 {
			break
		}

		for _, chat := range chats {
			if chat.UpdatedAt > cutoff {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			err := s.client.DeleteChat(ctx, acc.Token, chat.ID)
			cancel()
			if err != nil {
				s.logger.WarnModule("CLEANUP", "删除对话失败 email=%s chat_id=%s err=%v", acc.Email, chat.ID, err)
				failed++
			} else {
				s.logger.InfoModule("CLEANUP", "删除对话成功 email=%s chat_id=%s", acc.Email, chat.ID)
				deleted++
			}
		}

		page++
	}

	s.logger.InfoModule("CLEANUP", "全量对话清理完成 email=%s 删除=%d 失败=%d", acc.Email, deleted, failed)
}
