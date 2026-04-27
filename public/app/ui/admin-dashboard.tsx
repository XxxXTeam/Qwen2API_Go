"use client";

import { Button, Card, Chip, Input } from "@heroui/react";
import { useAdminConsole } from "./hooks/use-admin-console";
import { AccountsTab } from "./components/accounts-tab";
import { DebugTab } from "./components/debug-tab";
import { ModelsTab } from "./components/models-tab";
import { OverviewTab } from "./components/overview-tab";
import { SettingsTab } from "./components/settings-tab";
import { UploadsTab } from "./components/uploads-tab";
import { formatCompactNumber } from "./components/dashboard-charts";
import type { TabKey } from "./types";

const NAV_ITEMS: Array<{ key: TabKey; label: string; short: string }> = [
  { key: "overview", label: "总览", short: "概" },
  { key: "accounts", label: "账号池", short: "账" },
  { key: "settings", label: "系统设置", short: "设" },
  { key: "models", label: "模型能力", short: "模" },
  { key: "uploads", label: "文件上传", short: "传" },
  { key: "debug", label: "接口调试", short: "调" },
];

function ThemeIconButton({
  themeMode,
  onPress,
}: {
  themeMode: "light" | "dark";
  onPress: () => void;
}) {
  return (
    <Button
      isIconOnly
      className="theme-icon-button"
      variant="ghost"
      onPress={onPress}
      aria-label={themeMode === "dark" ? "切换到浅色模式" : "切换到暗色模式"}
    >
      <span className="theme-icon-glyph" aria-hidden="true">{themeMode === "dark" ? "☀" : "☾"}</span>
    </Button>
  );
}

export function AdminDashboard() {
  const { state, actions } = useAdminConsole();

  if (!state.verified) {
    return (
      <main className="dashboard-shell dashboard-shell-login">
        <div className="hero-backdrop hero-backdrop-left" />
        <div className="hero-backdrop hero-backdrop-right" />
        <section className="login-panel">
          <Card className="panel login-card">
            <Card.Header className="panel-header">
              <div className="login-topbar">
                <Chip color="accent" variant="soft">Qwen2API Console</Chip>
                <ThemeIconButton themeMode={state.themeMode} onPress={actions.toggleTheme} />
              </div>
              <Card.Title>超级管理后台</Card.Title>
              <Card.Description>使用管理员密钥登录，仅访问受保护的 `/api/*` 管理接口。</Card.Description>
            </Card.Header>
            <Card.Content className="login-form">
              <Input placeholder="输入管理员 API Key" type="password" value={state.apiKeyInput} onChange={(e) => actions.setApiKeyInput(e.target.value)} />
              <div className="login-actions">
                <Button className="action-button" variant="primary" onPress={() => void actions.verifyAdmin()}><span className="button-icon">→</span><span>进入管理台</span></Button>
                <Button
                  className="action-button"
                  variant="secondary"
                  onPress={() => {
                    actions.setApiKeyInput("");
                    if (typeof window !== "undefined") {
                      window.localStorage.removeItem("qwen2api-admin-key");
                    }
                  }}
                >
                  <span className="button-icon">×</span><span>清空</span>
                </Button>
              </div>
              {state.toast ? <p className={`toast toast-${state.toast.type}`}>{state.toast.message}</p> : null}
            </Card.Content>
          </Card>
        </section>
      </main>
    );
  }

  const currentTab = NAV_ITEMS.find((item) => item.key === state.activeTab);

  return (
    <main className="dashboard-shell">
      <div className="hero-backdrop hero-backdrop-left" />
      <div className="hero-backdrop hero-backdrop-right" />

      {state.toast ? <div className={`floating-toast toast-${state.toast.type}`}>{state.toast.message}</div> : null}

      <div className={`console-layout ${state.sidebarCollapsed ? "console-layout-collapsed" : ""}`}>
        <aside className={`console-sidebar ${state.sidebarCollapsed ? "collapsed" : ""}`}>
          <div className="sidebar-top">
            <div className="sidebar-brand">
              <div className="sidebar-brand-mark">
                <span>Q2</span>
              </div>
              {!state.sidebarCollapsed ? (
                <div className="sidebar-brand-copy">
                  <strong>Qwen2API</strong>
                  <span>Operations Console</span>
                </div>
              ) : null}
            </div>
            <Button className="sidebar-collapse-button" variant="ghost" onPress={actions.toggleSidebar}>
              <span className="button-icon">{state.sidebarCollapsed ? "»" : "«"}</span>
              {!state.sidebarCollapsed ? <span>{state.sidebarCollapsed ? "展开" : "收起"}</span> : null}
            </Button>
          </div>

          <div className="sidebar-status">
            <Chip color="success" variant="soft">在线</Chip>
            {!state.sidebarCollapsed ? <span>{state.overview?.accounts.initialized ? "账号池已初始化" : "等待初始化"}</span> : null}
          </div>

          {!state.sidebarCollapsed ? <p className="sidebar-group-label">导航菜单</p> : null}
          <nav className="sidebar-nav">
            {NAV_ITEMS.map((item) => (
              <button
                key={item.key}
                type="button"
                className={`sidebar-nav-item ${state.activeTab === item.key ? "active" : ""}`}
                onClick={() => actions.setActiveTab(item.key)}
                title={item.label}
              >
                <span className="sidebar-nav-icon">{state.sidebarCollapsed ? item.short : item.short}</span>
                {!state.sidebarCollapsed ? <span>{item.label}</span> : null}
              </button>
            ))}
          </nav>

          <div className="sidebar-footer">
            <ThemeIconButton themeMode={state.themeMode} onPress={actions.toggleTheme} />
            <Button className="action-button" variant="secondary" onPress={() => void actions.refreshShell()}>
              <span className="button-icon">↻</span>
              {!state.sidebarCollapsed ? <span>{state.loadingShell ? "刷新中..." : "刷新数据"}</span> : null}
            </Button>
            <Button className="action-button" variant="ghost" onPress={actions.logout}>
              <span className="button-icon">⇥</span>
              {!state.sidebarCollapsed ? <span>退出登录</span> : null}
            </Button>
          </div>
        </aside>

        <section className="console-main">
          <header className="console-header">
            <div className="console-header-copy">
              <p className="eyebrow">Admin Workspace</p>
              <h1>{currentTab?.label || "管理后台"}</h1>
              <p className="subtle">业务流量、账号池和模型供给都在一个可折叠的控制台里管理。</p>
            </div>
            <div className="console-header-meta">
              <div className="hero-side-card">
                <span>业务请求</span>
                <strong>{formatCompactNumber(state.overview?.analytics.totals.requests)}</strong>
              </div>
              <div className="hero-side-card">
                <span>后台请求</span>
                <strong>{formatCompactNumber(state.overview?.analytics.totals.admin)}</strong>
              </div>
              <div className="hero-side-card">
                <span>最近生成</span>
                <strong>{state.overview?.generatedAt ? new Date(state.overview.generatedAt).toLocaleTimeString("zh-CN", { hour12: false }) : "--"}</strong>
              </div>
            </div>
          </header>

          <section className="console-summary-strip">
            <div className="hero-side-card">
              <span>累计 Token</span>
              <strong>{formatCompactNumber(state.overview?.analytics.totals.totalTokens)}</strong>
            </div>
            <div className="hero-side-card">
              <span>当前 RPM</span>
              <strong>{formatCompactNumber(state.overview?.analytics.rpm)}</strong>
            </div>
            <div className="hero-side-card">
              <span>有效账号</span>
              <strong>{formatCompactNumber(state.overview?.accounts.valid)}</strong>
            </div>
            <div className="hero-side-card">
              <span>模型变体</span>
              <strong>{formatCompactNumber(state.modelCounts.total)}</strong>
            </div>
          </section>

          <section className="console-panel-area">
            {state.activeTab === "overview" ? <OverviewTab overview={state.overview} modelCounts={state.modelCounts} /> : null}

            {state.activeTab === "accounts" ? (
              <AccountsTab
                accounts={state.accounts}
                batchTask={state.batchTask}
                filters={state.filters}
                draftKeyword={state.draftKeyword}
                newAccountEmail={state.newAccountEmail}
                newAccountPassword={state.newAccountPassword}
                batchAccountsText={state.batchAccountsText}
                loadingAccounts={state.loadingAccounts}
                actions={{
                  setNewAccountEmail: actions.setNewAccountEmail,
                  setNewAccountPassword: actions.setNewAccountPassword,
                  setBatchAccountsText: actions.setBatchAccountsText,
                  createAccount: actions.createAccount,
                  createBatchTask: actions.createBatchTask,
                  refreshAccounts: actions.refreshAccounts,
                  setDraftKeyword: actions.setDraftKeyword,
                  setFilters: actions.setFilters,
                  refreshAccount: actions.refreshAccount,
                  deleteAccount: actions.deleteAccount,
                }}
              />
            ) : null}

            {state.activeTab === "settings" ? (
              <SettingsTab
                settings={state.settings}
                savingSettings={state.savingSettings}
                addKeyValue={state.addKeyValue}
                thresholdHours={state.thresholdHours}
                setAddKeyValue={actions.setAddKeyValue}
                setThresholdHours={actions.setThresholdHours}
                setSettings={actions.setSettings}
                addRegularKey={actions.addRegularKey}
                deleteRegularKey={actions.deleteRegularKey}
                refreshAllAccounts={actions.refreshAllAccounts}
                reloadRuntimeConfig={actions.reloadRuntimeConfig}
                saveSettings={actions.saveSettings}
              />
            ) : null}

            {state.activeTab === "models" ? <ModelsTab models={state.filteredModels} keyword={state.modelKeyword} setKeyword={actions.setModelKeyword} /> : null}
            {state.activeTab === "uploads" ? <UploadsTab apiKey={state.apiKey} /> : null}
            {state.activeTab === "debug" ? <DebugTab apiKey={state.apiKey} models={state.filteredModels} /> : null}
          </section>
        </section>
      </div>
    </main>
  );
}
