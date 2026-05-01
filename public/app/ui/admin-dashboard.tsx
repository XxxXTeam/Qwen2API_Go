"use client";

import {
  LayoutDashboard,
  Users,
  Settings,
  Brain,
  Upload,
  Bug,
  Menu,
  Moon,
  Sun,
  RefreshCw,
  LogOut,
  ChevronLeft,
  ChevronRight,
} from "lucide-react";
import { Input } from "@heroui/react";
import { useAdminConsole } from "./hooks/use-admin-console";
import { AccountsTab } from "./components/accounts-tab";
import { DebugTab } from "./components/debug-tab";
import { ModelsTab } from "./components/models-tab";
import { OverviewTab } from "./components/overview-tab";
import { SettingsTab } from "./components/settings-tab";
import { UploadsTab } from "./components/uploads-tab";
import { formatCompactNumber } from "./components/dashboard-charts";
import type { TabKey } from "./types";

const NAV_ITEMS: Array<{ key: TabKey; label: string; icon: React.ReactNode }> = [
  { key: "overview", label: "数据总览", icon: <LayoutDashboard size={18} /> },
  { key: "accounts", label: "账号池", icon: <Users size={18} /> },
  { key: "settings", label: "系统设置", icon: <Settings size={18} /> },
  { key: "models", label: "模型能力", icon: <Brain size={18} /> },
  { key: "uploads", label: "文件上传", icon: <Upload size={18} /> },
  { key: "debug", label: "接口调试", icon: <Bug size={18} /> },
];

export function AdminDashboard() {
  const { state, actions } = useAdminConsole();

  if (!state.verified) {
    return (
      <div className="admin-login">
        <div className="admin-login-card">
          <div className="flex items-center gap-3 mb-6">
            <div className="admin-sidebar-logo">Q2</div>
            <div>
              <h1>Qwen2API 管理后台</h1>
              <p className="text-[13px] text-[var(--text-secondary)] mt-1">管理员密钥登录</p>
            </div>
          </div>

          <div className="flex flex-col gap-4">
            <Input
              placeholder="输入管理员 API Key"
              type="password"
              value={state.apiKeyInput}
              onChange={(e) => actions.setApiKeyInput(e.target.value)}
              className="w-full"
            />
            <div className="flex gap-3">
              <button
                className="admin-btn admin-btn-primary flex-1"
                onClick={() => void actions.verifyAdmin()}
              >
                进入管理台
              </button>
              <button
                className="admin-btn admin-btn-secondary"
                onClick={() => {
                  actions.setApiKeyInput("");
                  if (typeof window !== "undefined") {
                    window.localStorage.removeItem("qwen2api-admin-key");
                  }
                }}
              >
                清空
              </button>
            </div>
          </div>

          {state.toast ? (
            <div
              className={`mt-4 p-3 rounded-lg text-sm font-medium ${
                state.toast.type === "error"
                  ? "bg-[var(--danger-light)] text-[var(--danger)]"
                  : state.toast.type === "success"
                  ? "bg-[var(--success-light)] text-[var(--success)]"
                  : "bg-[var(--primary-light)] text-[var(--primary)]"
              }`}
            >
              {state.toast.message}
            </div>
          ) : null}
        </div>
      </div>
    );
  }

  const currentTab = NAV_ITEMS.find((item) => item.key === state.activeTab);

  return (
    <div className="admin-root">
      {state.toast ? (
        <div className={`admin-toast ${state.toast.type}`}>
          {state.toast.message}
        </div>
      ) : null}

      {/* Sidebar */}
      <aside className={`admin-sidebar ${state.sidebarCollapsed ? "collapsed" : ""}`}>
        <div className="admin-sidebar-header">
          <div className="admin-sidebar-logo">Q2</div>
          <span className="admin-sidebar-title">Qwen2API</span>
          <button
            className="admin-btn admin-btn-ghost admin-btn-sm ml-auto"
            onClick={actions.toggleSidebar}
            title={state.sidebarCollapsed ? "展开" : "收起"}
          >
            {state.sidebarCollapsed ? <ChevronRight size={16} /> : <ChevronLeft size={16} />}
          </button>
        </div>

        <nav className="admin-sidebar-nav">
          {NAV_ITEMS.map((item) => (
            <button
              key={item.key}
              type="button"
              className={`admin-nav-item ${state.activeTab === item.key ? "active" : ""}`}
              onClick={() => actions.setActiveTab(item.key)}
              title={item.label}
            >
              {item.icon}
              <span>{item.label}</span>
            </button>
          ))}
        </nav>

        <div className="admin-sidebar-footer">
          <button className="admin-nav-item" onClick={actions.toggleTheme}>
            {state.themeMode === "dark" ? <Sun size={18} /> : <Moon size={18} />}
            <span>{state.themeMode === "dark" ? "浅色模式" : "深色模式"}</span>
          </button>
          <button className="admin-nav-item" onClick={() => void actions.refreshShell()}>
            <RefreshCw size={18} className={state.loadingShell ? "animate-spin" : ""} />
            <span>{state.loadingShell ? "刷新中..." : "刷新数据"}</span>
          </button>
          <button className="admin-nav-item" onClick={actions.logout}>
            <LogOut size={18} />
            <span>退出登录</span>
          </button>
        </div>
      </aside>

      {/* Main */}
      <div className={`admin-main ${state.sidebarCollapsed ? "collapsed" : ""}`}>
        {/* Header */}
        <header className="admin-header">
          <div className="admin-header-left">
            <button
              className="admin-btn admin-btn-ghost admin-btn-sm lg:hidden"
              onClick={actions.toggleSidebar}
            >
              <Menu size={16} />
            </button>
            <span className="admin-page-title">{currentTab?.label || "管理后台"}</span>
          </div>
          <div className="admin-header-right">
            <div className="hidden md:flex items-center gap-6 text-sm text-[var(--text-secondary)] mr-4">
              <div className="flex flex-col items-end">
                <span className="text-xs text-[var(--text-muted)]">业务请求</span>
                <span className="font-semibold text-[var(--text)]">
                  {formatCompactNumber(state.overview?.analytics.totals.requests)}
                </span>
              </div>
              <div className="flex flex-col items-end">
                <span className="text-xs text-[var(--text-muted)]">有效账号</span>
                <span className="font-semibold text-[var(--text)]">
                  {formatCompactNumber(state.overview?.accounts.valid)}
                </span>
              </div>
              <div className="flex flex-col items-end">
                <span className="text-xs text-[var(--text-muted)]">模型数</span>
                <span className="font-semibold text-[var(--text)]">
                  {formatCompactNumber(state.modelCounts.total)}
                </span>
              </div>
            </div>
          </div>
        </header>

        {/* Content */}
        <main className="admin-content">
          {state.activeTab === "overview" ? (
            <OverviewTab overview={state.overview} modelCounts={state.modelCounts} />
          ) : null}

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

          {state.activeTab === "models" ? (
            <ModelsTab models={state.filteredModels} keyword={state.modelKeyword} setKeyword={actions.setModelKeyword} />
          ) : null}

          {state.activeTab === "uploads" ? <UploadsTab apiKey={state.apiKey} /> : null}
          {state.activeTab === "debug" ? <DebugTab apiKey={state.apiKey} models={state.filteredModels} /> : null}
        </main>
      </div>
    </div>
  );
}
