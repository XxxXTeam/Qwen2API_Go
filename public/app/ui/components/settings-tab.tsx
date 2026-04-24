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
  saveSettings: (path: string, body: Record<string, unknown>, successMessage: string) => Promise<void>;
}) {
  return (
    <div className="settings-layout">
      <Card className="panel settings-main-card">
        <Card.Header className="panel-header">
          <Card.Title>运行策略</Card.Title>
          <Card.Description>把策略开关、刷新参数和模型映射收敛到主操作面板，避免信息发散。</Card.Description>
        </Card.Header>
        <Card.Content className="stack-lg">
          <div className="switch-list">
            <Switch isSelected={settings?.autoRefresh ?? false} onChange={(value) => setSettings((c) => c ? { ...c, autoRefresh: value } : c)}>自动刷新账号令牌</Switch>
            <Switch isSelected={settings?.outThink ?? false} onChange={(value) => setSettings((c) => c ? { ...c, outThink: value } : c)}>输出思考过程</Switch>
            <Switch isSelected={settings?.simpleModelMap ?? false} onChange={(value) => setSettings((c) => c ? { ...c, simpleModelMap: value } : c)}>简化模型映射</Switch>
          </div>

          <div className="toolbar-grid">
            <Input placeholder="自动刷新间隔（秒）" type="number" value={String(settings?.autoRefreshInterval ?? 21600)} onChange={(e) => setSettings((c) => c ? { ...c, autoRefreshInterval: Number(e.target.value) || 0 } : c)} />
            <Input placeholder="批量登录并发" type="number" value={String(settings?.batchLoginConcurrency ?? 5)} onChange={(e) => setSettings((c) => c ? { ...c, batchLoginConcurrency: Number(e.target.value) || 1 } : c)} />
            <select className="app-select" value={settings?.searchInfoMode ?? "text"} onChange={(e) => setSettings((c) => c ? { ...c, searchInfoMode: e.target.value as "table" | "text" } : c)}>
              <option value="text">搜索文本模式</option>
              <option value="table">搜索表格模式</option>
            </select>
          </div>

          <div className="wrap-actions">
            <Button variant="primary" isDisabled={!settings || savingSettings} onPress={() => settings && void saveSettings("/api/setAutoRefresh", { autoRefresh: settings.autoRefresh, autoRefreshInterval: settings.autoRefreshInterval }, "自动刷新设置已更新。")}>保存自动刷新</Button>
            <Button variant="secondary" isDisabled={!settings || savingSettings} onPress={() => settings && void saveSettings("/api/setBatchLoginConcurrency", { batchLoginConcurrency: settings.batchLoginConcurrency }, "批量登录并发已更新。")}>保存并发</Button>
            <Button variant="ghost" isDisabled={!settings || savingSettings} onPress={() => settings && void saveSettings("/api/setOutThink", { outThink: settings.outThink }, "思考输出设置已更新。")}>保存思考输出</Button>
            <Button variant="outline" isDisabled={!settings || savingSettings} onPress={() => settings && void saveSettings("/api/search-info-mode", { searchInfoMode: settings.searchInfoMode }, "搜索模式已更新。")}>保存搜索模式</Button>
            <Button variant="secondary" isDisabled={!settings || savingSettings} onPress={() => settings && void saveSettings("/api/simple-model-map", { simpleModelMap: settings.simpleModelMap }, "模型映射设置已更新。")}>保存模型映射</Button>
          </div>
        </Card.Content>
      </Card>

      <Card className="panel settings-side-card">
        <Card.Header className="panel-header">
          <Card.Title>密钥与刷新任务</Card.Title>
          <Card.Description>高风险和全局操作放到侧边区，降低误触成本。</Card.Description>
        </Card.Header>
        <Card.Content className="stack-lg">
          <div className="toolbar-grid">
            <Input placeholder="新增普通 API Key" value={addKeyValue} onChange={(e) => setAddKeyValue(e.target.value)} />
            <Button variant="primary" onPress={() => void addRegularKey()}>添加 Key</Button>
          </div>

          <div className="key-list">
            {settings?.regularKeys.map((key) => (
              <div className="key-row" key={key}>
                <span className="mono">{key}</span>
                <Button variant="danger" onPress={() => void deleteRegularKey(key)}>删除</Button>
              </div>
            ))}
            {!settings?.regularKeys.length ? <p className="panel-copy">当前没有普通 API Key。</p> : null}
          </div>

          <div className="toolbar-grid">
            <Input placeholder="即将过期阈值（小时）" type="number" value={thresholdHours} onChange={(e) => setThresholdHours(e.target.value)} />
            <Button variant="secondary" onPress={() => void refreshAllAccounts(false)}>阈值刷新</Button>
            <Button variant="danger" onPress={() => void refreshAllAccounts(true)}>强制全刷</Button>
          </div>
        </Card.Content>
      </Card>
    </div>
  );
}
