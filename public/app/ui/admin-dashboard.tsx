"use client";

import { Button, Card, Chip, Input, Switch, Tabs } from "@heroui/react";
import { useAdminConsole } from "./hooks/use-admin-console";
import { AccountsTab } from "./components/accounts-tab";
import { DebugTab } from "./components/debug-tab";
import { ModelsTab } from "./components/models-tab";
import { OverviewTab } from "./components/overview-tab";
import { SettingsTab } from "./components/settings-tab";
import { UploadsTab } from "./components/uploads-tab";
import { StatCard } from "./components/primitives";
import type { TabKey } from "./types";

export function AdminDashboard() {
  const { state, actions } = useAdminConsole();

  if (!state.verified) {
    return (
      <main className="dashboard-shell">
        <div className="hero-backdrop hero-backdrop-left" />
        <div className="hero-backdrop hero-backdrop-right" />
        <section className="login-panel">
          <Card className="panel login-card">
            <Card.Header className="panel-header">
              <div className="login-topbar">
                <Chip color="accent" variant="soft">Qwen2API Console</Chip>
                <Switch isSelected={state.themeMode === "dark"} onChange={() => actions.toggleTheme()}>
                  暗色模式
                </Switch>
              </div>
              <Card.Title>超级管理后台</Card.Title>
              <Card.Description>使用管理员密钥登录。页面直接消费 `/api/*` 与公开 `/models` 接口。</Card.Description>
            </Card.Header>
            <Card.Content className="login-form">
              <Input placeholder="输入管理员 API Key" type="password" value={state.apiKeyInput} onChange={(e) => actions.setApiKeyInput(e.target.value)} />
              <div className="login-actions">
                <Button variant="primary" onPress={() => void actions.verifyAdmin()}>进入管理台</Button>
                <Button
                  variant="secondary"
                  onPress={() => {
                    actions.setApiKeyInput("");
                    if (typeof window !== "undefined") {
                      window.localStorage.removeItem("qwen2api-admin-key");
                    }
                  }}
                >
                  清空
                </Button>
              </div>
              {state.toast ? <p className={`toast toast-${state.toast.type}`}>{state.toast.message}</p> : null}
            </Card.Content>
          </Card>
        </section>
      </main>
    );
  }

  return (
    <main className="dashboard-shell">
      <div className="hero-backdrop hero-backdrop-left" />
      <div className="hero-backdrop hero-backdrop-right" />

      <section className="dashboard-topbar">
        <Card className="panel hero-panel">
          <Card.Content className="hero-layout">
            <div className="hero-copy">
              <div>
                <p className="eyebrow">HeroUI v3 Admin Surface</p>
                <h1>Qwen2API Web 管理面板</h1>
                <p className="subtle">面向大规模账号池的服务端分页后台，覆盖概览、账号、设置、模型与接口调试。</p>
              </div>
              <div className="hero-tags">
                <Chip color="accent" variant="soft">服务端分页</Chip>
                <Chip color="success" variant="soft">健康视图</Chip>
                <Chip color="warning" variant="soft">任务追踪</Chip>
              </div>
            </div>
            <div className="hero-side">
              <div className="hero-side-card">
                <span>当前主题</span>
                <strong>{state.themeMode === "dark" ? "Dark Console" : "Light Console"}</strong>
              </div>
              <div className="hero-side-card">
                <span>后台状态</span>
                <strong>{state.overview?.accounts.initialized ? "账号池已初始化" : "等待初始化"}</strong>
              </div>
              <div className="topbar-actions">
                <Switch isSelected={state.themeMode === "dark"} onChange={() => actions.toggleTheme()}>
                  暗色模式
                </Switch>
                <Button variant="secondary" onPress={() => void actions.refreshShell()}>
                  {state.loadingShell ? "刷新中..." : "刷新总览"}
                </Button>
                <Button variant="ghost" onPress={actions.logout}>退出登录</Button>
              </div>
            </div>
          </Card.Content>
        </Card>
      </section>

      {state.toast ? <div className={`floating-toast toast-${state.toast.type}`}>{state.toast.message}</div> : null}

      <section className="stats-grid stats-ribbon">
        <StatCard
          title="账号池总量"
          value={state.overview?.accounts.total ?? "--"}
          description="后端聚合统计，不把所有账号一次塞进浏览器。"
        />
        <StatCard
          title="健康账号"
          value={state.overview?.accounts.valid ?? "--"}
          description="剩余有效期大于 6 小时的账号数量。"
          tone="success"
        />
        <StatCard
          title="即将过期"
          value={state.overview?.accounts.expiringSoon ?? "--"}
          description="适合提前刷新，避免轮询命中失效账号。"
          tone="warning"
        />
        <StatCard
          title="失效 / 异常"
          value={(state.overview?.accounts.expired ?? 0) + (state.overview?.accounts.invalid ?? 0)}
          description="已过期或 token 缺失的账号。"
          tone="danger"
        />
      </section>

      <Tabs
        selectedKey={state.activeTab}
        onSelectionChange={(key) => actions.setActiveTab(String(key) as TabKey)}
        className="tabs-root"
      >
        <Tabs.ListContainer className="tabs-nav">
          <Tabs.List aria-label="管理面板导航">
            <Tabs.Tab id="overview">总览</Tabs.Tab>
            <Tabs.Tab id="accounts">账号池</Tabs.Tab>
            <Tabs.Tab id="settings">系统设置</Tabs.Tab>
            <Tabs.Tab id="models">模型能力</Tabs.Tab>
            <Tabs.Tab id="uploads">文件上传</Tabs.Tab>
            <Tabs.Tab id="debug">接口调试</Tabs.Tab>
          </Tabs.List>
        </Tabs.ListContainer>

        <Tabs.Panel id="overview" className="tab-panel">
          <OverviewTab overview={state.overview} modelCounts={state.modelCounts} />
        </Tabs.Panel>

        <Tabs.Panel id="accounts" className="tab-panel">
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
        </Tabs.Panel>

        <Tabs.Panel id="settings" className="tab-panel">
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
            saveSettings={actions.saveSettings}
          />
        </Tabs.Panel>

        <Tabs.Panel id="models" className="tab-panel">
          <ModelsTab models={state.filteredModels} keyword={state.modelKeyword} setKeyword={actions.setModelKeyword} />
        </Tabs.Panel>

        <Tabs.Panel id="uploads" className="tab-panel">
          <UploadsTab apiKey={state.apiKey} />
        </Tabs.Panel>

        <Tabs.Panel id="debug" className="tab-panel">
          <DebugTab apiKey={state.apiKey} />
        </Tabs.Panel>
      </Tabs>
    </main>
  );
}
