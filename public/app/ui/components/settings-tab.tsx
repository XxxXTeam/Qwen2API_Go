import { Button, Card, Input, Switch } from "@heroui/react";
import type { Dispatch, SetStateAction } from "react";
import type { SettingsResponse } from "../types";

export function SettingsTab({
  settings,
  savingSettings,
  addKeyValue,
  thresholdHours,
  setAddKeyValue,
  setThresholdHours,
  setSettings,
  addRegularKey,
  deleteRegularKey,
  refreshAllAccounts,
  reloadRuntimeConfig,
  saveSettings,
}: {
  settings: SettingsResponse | null;
  savingSettings: boolean;
  addKeyValue: string;
  thresholdHours: string;
  setAddKeyValue: (value: string) => void;
  setThresholdHours: (value: string) => void;
  setSettings: Dispatch<SetStateAction<SettingsResponse | null>>;
  addRegularKey: () => Promise<void>;
  deleteRegularKey: (key: string) => Promise<void>;
  refreshAllAccounts: (force: boolean) => Promise<void>;
  reloadRuntimeConfig: () => Promise<void>;
  saveSettings: (path: string, body: Record<string, unknown>, successMessage: string) => Promise<void>;
}) {
  const enabledStrategies = [
    settings?.autoRefresh ?? false,
    settings?.outThink ?? false,
    settings?.simpleModelMap ?? false,
  ].filter(Boolean).length;

  return (
    <div className="settings-workspace stack-lg">
      <div className="settings-overview-grid">
        <div className="settings-overview-card">
          <span>已启用策略</span>
          <strong>{enabledStrategies}/3</strong>
          <p className="panel-copy">自动刷新、思考输出、模型映射三个核心开关的当前启用数。</p>
        </div>
        <div className="settings-overview-card">
          <span>普通 Key 数量</span>
          <strong>{settings?.regularKeys.length ?? 0}</strong>
          <p className="panel-copy">当前系统里登记的常规访问密钥数量。</p>
        </div>
        <div className="settings-overview-card">
          <span>刷新周期</span>
          <strong>{settings?.autoRefreshInterval ?? 21600}s</strong>
          <p className="panel-copy">账号令牌自动刷新的时间间隔。</p>
        </div>
        <div className="settings-overview-card">
          <span>搜索模式</span>
          <strong>{settings?.searchInfoMode === "table" ? "表格模式" : "文本模式"}</strong>
          <p className="panel-copy">控制搜索结果在系统中的默认呈现方式。</p>
        </div>
      </div>

      <div className="settings-layout">
        <Card className="panel settings-main-card">
          <Card.Header className="panel-header">
            <Card.Title>运行策略</Card.Title>
            <Card.Description>把策略开关、刷新参数和模型映射收敛到主操作面板，避免信息发散。</Card.Description>
          </Card.Header>
          <Card.Content className="stack-lg">
            <div className="settings-section">
              <div className="settings-section-heading">
                <strong>策略开关</strong>
                <span>用卡片形式管理高频开关，降低误触和阅读成本。</span>
              </div>
              <div className="settings-switch-grid">
                <div className="settings-switch-card">
                  <div className="settings-card-head">
                    <div>
                      <strong>自动刷新账号令牌</strong>
                      <p className="panel-copy">按设定周期自动刷新账号 token，减少人工维护。</p>
                    </div>
                    <Switch isSelected={settings?.autoRefresh ?? false} onChange={(value) => setSettings((c) => c ? { ...c, autoRefresh: value } : c)} />
                  </div>
                  <Button className="action-button" variant="primary" isDisabled={!settings || savingSettings} onPress={() => settings && void saveSettings("/api/setAutoRefresh", { autoRefresh: settings.autoRefresh, autoRefreshInterval: settings.autoRefreshInterval }, "自动刷新设置已更新。")}><span className="button-icon">↻</span><span>保存自动刷新</span></Button>
                </div>
                <div className="settings-switch-card">
                  <div className="settings-card-head">
                    <div>
                      <strong>输出思考过程</strong>
                      <p className="panel-copy">控制是否向客户端暴露 thinking 内容。</p>
                    </div>
                    <Switch isSelected={settings?.outThink ?? false} onChange={(value) => setSettings((c) => c ? { ...c, outThink: value } : c)} />
                  </div>
                  <Button className="action-button" variant="ghost" isDisabled={!settings || savingSettings} onPress={() => settings && void saveSettings("/api/setOutThink", { outThink: settings.outThink }, "思考输出设置已更新。")}><span className="button-icon">⋯</span><span>保存思考输出</span></Button>
                </div>
                <div className="settings-switch-card">
                  <div className="settings-card-head">
                    <div>
                      <strong>简化模型映射</strong>
                      <p className="panel-copy">收敛变体展示，降低模型列表复杂度。</p>
                    </div>
                    <Switch isSelected={settings?.simpleModelMap ?? false} onChange={(value) => setSettings((c) => c ? { ...c, simpleModelMap: value } : c)} />
                  </div>
                  <Button className="action-button" variant="secondary" isDisabled={!settings || savingSettings} onPress={() => settings && void saveSettings("/api/simple-model-map", { simpleModelMap: settings.simpleModelMap }, "模型映射设置已更新。")}><span className="button-icon">◎</span><span>保存模型映射</span></Button>
                </div>
              </div>
            </div>

            <div className="settings-section">
              <div className="settings-section-heading">
                <strong>运行参数</strong>
                <span>把刷新周期、并发数和搜索模式收敛到统一字段区。</span>
              </div>
              <div className="settings-field-grid">
                <div className="settings-field-card">
                  <span>自动刷新间隔（秒）</span>
                  <Input placeholder="自动刷新间隔（秒）" type="number" value={String(settings?.autoRefreshInterval ?? 21600)} onChange={(e) => setSettings((c) => c ? { ...c, autoRefreshInterval: Number(e.target.value) || 0 } : c)} />
                  <Button className="action-button" variant="primary" isDisabled={!settings || savingSettings} onPress={() => settings && void saveSettings("/api/setAutoRefresh", { autoRefresh: settings.autoRefresh, autoRefreshInterval: settings.autoRefreshInterval }, "自动刷新设置已更新。")}><span className="button-icon">↻</span><span>保存刷新参数</span></Button>
                </div>
                <div className="settings-field-card">
                  <span>批量登录并发</span>
                  <Input placeholder="批量登录并发" type="number" value={String(settings?.batchLoginConcurrency ?? 5)} onChange={(e) => setSettings((c) => c ? { ...c, batchLoginConcurrency: Number(e.target.value) || 1 } : c)} />
                  <Button className="action-button" variant="secondary" isDisabled={!settings || savingSettings} onPress={() => settings && void saveSettings("/api/setBatchLoginConcurrency", { batchLoginConcurrency: settings.batchLoginConcurrency }, "批量登录并发已更新。")}><span className="button-icon">≋</span><span>保存并发</span></Button>
                </div>
                <div className="settings-field-card">
                  <span>搜索信息模式</span>
                  <select className="app-select" value={settings?.searchInfoMode ?? "text"} onChange={(e) => setSettings((c) => c ? { ...c, searchInfoMode: e.target.value as "table" | "text" } : c)}>
                    <option value="text">搜索文本模式</option>
                    <option value="table">搜索表格模式</option>
                  </select>
                  <Button className="action-button" variant="outline" isDisabled={!settings || savingSettings} onPress={() => settings && void saveSettings("/api/search-info-mode", { searchInfoMode: settings.searchInfoMode }, "搜索模式已更新。")}><span className="button-icon">⌕</span><span>保存搜索模式</span></Button>
                </div>
              </div>
            </div>
          </Card.Content>
        </Card>

        <div className="settings-side-stack">
          <Card className="panel settings-side-card">
            <Card.Header className="panel-header">
              <Card.Title>访问密钥</Card.Title>
              <Card.Description>统一管理普通 API Key，避免和运行策略混在一起。</Card.Description>
            </Card.Header>
            <Card.Content className="stack-lg">
              <div className="settings-inline-form">
                <Input placeholder="新增普通 API Key" value={addKeyValue} onChange={(e) => setAddKeyValue(e.target.value)} />
                <Button className="action-button" variant="primary" onPress={() => void addRegularKey()}><span className="button-icon">＋</span><span>添加 Key</span></Button>
              </div>

              <div className="settings-key-panel">
                <div className="settings-section-heading">
                  <strong>现有 Key 列表</strong>
                  <span>保留独立列表，便于删除和核对。</span>
                </div>
                <div className="key-list">
                  {settings?.regularKeys.map((key) => (
                    <div className="key-row" key={key}>
                      <span className="mono">{key}</span>
                      <Button className="action-button" variant="danger" onPress={() => void deleteRegularKey(key)}><span className="button-icon">×</span><span>删除</span></Button>
                    </div>
                  ))}
                  {!settings?.regularKeys.length ? <p className="panel-copy">当前没有普通 API Key。</p> : null}
                </div>
              </div>
            </Card.Content>
          </Card>

          <Card className="panel settings-side-card">
            <Card.Header className="panel-header">
              <Card.Title>刷新与热更新</Card.Title>
              <Card.Description>账号刷新和 `.env` 重载放在同一组，方便运维操作。</Card.Description>
            </Card.Header>
            <Card.Content className="stack-lg">
              <div className="settings-refresh-panel">
                <div className="settings-field-card">
                  <span>即将过期阈值（小时）</span>
                  <Input placeholder="即将过期阈值（小时）" type="number" value={thresholdHours} onChange={(e) => setThresholdHours(e.target.value)} />
                </div>

                <div className="settings-danger-actions">
                  <Button className="action-button" variant="secondary" onPress={() => void refreshAllAccounts(false)}><span className="button-icon">↺</span><span>阈值刷新</span></Button>
                  <Button className="action-button" variant="danger" onPress={() => void refreshAllAccounts(true)}><span className="button-icon">!</span><span>强制全刷</span></Button>
                </div>
              </div>

              <div className="settings-section">
                <div className="settings-section-heading">
                  <strong>配置热更新</strong>
                  <span>后台保存会立即生效并写入 `.env`；手动改 `.env` 后可在这里重载。</span>
                </div>
                <Button className="action-button" variant="primary" isDisabled={savingSettings} onPress={() => void reloadRuntimeConfig()}>
                  <span className="button-icon">⟳</span><span>重新加载 .env</span>
                </Button>
              </div>

              <div className="settings-risk-note">
                <strong>操作提醒</strong>
                <p className="panel-copy">阈值刷新会优先处理接近过期的账号；强制全刷会对整个账号池重新登录；`.env` 重载只影响运行参数，不会重建已初始化组件。</p>
              </div>
            </Card.Content>
          </Card>
        </div>
      </div>
    </div>
  );
}
