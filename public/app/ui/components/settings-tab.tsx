import type { Dispatch, SetStateAction } from "react";
import type { SettingsResponse } from "../types";
import { Input, Switch } from "@heroui/react";

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
    <div className="flex flex-col gap-6">
      {/* Overview stats */}
      <div className="admin-stat-grid">
        <div className="admin-stat-card primary">
          <div className="label">已启用策略</div>
          <div className="value">{enabledStrategies}/3</div>
          <div className="desc">自动刷新、思考输出、模型映射三个核心开关的当前启用数</div>
        </div>
        <div className="admin-stat-card primary">
          <div className="label">普通 Key 数量</div>
          <div className="value">{settings?.regularKeys.length ?? 0}</div>
          <div className="desc">当前系统里登记的常规访问密钥数量</div>
        </div>
        <div className="admin-stat-card primary">
          <div className="label">刷新周期</div>
          <div className="value">{settings?.autoRefreshInterval ?? 21600}s</div>
          <div className="desc">账号令牌自动刷新的时间间隔</div>
        </div>
        <div className="admin-stat-card primary">
          <div className="label">搜索模式</div>
          <div className="value">{settings?.searchInfoMode === "table" ? "表格模式" : "文本模式"}</div>
          <div className="desc">控制搜索结果在系统中的默认呈现方式</div>
        </div>
      </div>

      <div className="admin-settings-grid">
        <div className="flex flex-col gap-4">
          {/* Strategies */}
          <div className="admin-card">
            <div className="admin-card-header">
              <div>
                <h3>运行策略</h3>
                <p>把策略开关、刷新参数和模型映射收敛到主操作面板，避免信息发散</p>
              </div>
            </div>
            <div className="admin-card-body flex flex-col gap-6">
              <div>
                <h4 className="text-sm font-semibold mb-1">策略开关</h4>
                <p className="text-xs text-[var(--text-secondary)] mb-4">用卡片形式管理高频开关，降低误触和阅读成本</p>
                <div className="flex flex-col gap-3">
                  <div className="admin-switch-card">
                    <div>
                      <strong>自动刷新账号令牌</strong>
                      <p>按设定周期自动刷新账号 token，减少人工维护</p>
                    </div>
                    <div className="flex flex-col items-end gap-3">
                      <Switch
                        isSelected={settings?.autoRefresh ?? false}
                        onChange={(value) => setSettings((c) => (c ? { ...c, autoRefresh: value } : c))}
                      >
                        <Switch.Control>
                          <Switch.Thumb />
                        </Switch.Control>
                      </Switch>
                      <button
                        className="admin-btn admin-btn-primary admin-btn-sm"
                        disabled={!settings || savingSettings}
                        onClick={() =>
                          settings &&
                          void saveSettings(
                            "/api/setAutoRefresh",
                            { autoRefresh: settings.autoRefresh, autoRefreshInterval: settings.autoRefreshInterval },
                            "自动刷新设置已更新。"
                          )
                        }
                      >
                        保存自动刷新
                      </button>
                    </div>
                  </div>

                  <div className="admin-switch-card">
                    <div>
                      <strong>输出思考过程</strong>
                      <p>控制是否向客户端暴露 thinking 内容</p>
                    </div>
                    <div className="flex flex-col items-end gap-3">
                      <Switch
                        isSelected={settings?.outThink ?? false}
                        onChange={(value) => setSettings((c) => (c ? { ...c, outThink: value } : c))}
                      >
                        <Switch.Control>
                          <Switch.Thumb />
                        </Switch.Control>
                      </Switch>
                      <button
                        className="admin-btn admin-btn-ghost admin-btn-sm"
                        disabled={!settings || savingSettings}
                        onClick={() =>
                          settings &&
                          void saveSettings("/api/setOutThink", { outThink: settings.outThink }, "思考输出设置已更新。")
                        }
                      >
                        保存思考输出
                      </button>
                    </div>
                  </div>

                  <div className="admin-switch-card">
                    <div>
                      <strong>简化模型映射</strong>
                      <p>收敛变体展示，降低模型列表复杂度</p>
                    </div>
                    <div className="flex flex-col items-end gap-3">
                      <Switch
                        isSelected={settings?.simpleModelMap ?? false}
                        onChange={(value) => setSettings((c) => (c ? { ...c, simpleModelMap: value } : c))}
                      >
                        <Switch.Control>
                          <Switch.Thumb />
                        </Switch.Control>
                      </Switch>
                      <button
                        className="admin-btn admin-btn-secondary admin-btn-sm"
                        disabled={!settings || savingSettings}
                        onClick={() =>
                          settings &&
                          void saveSettings(
                            "/api/simple-model-map",
                            { simpleModelMap: settings.simpleModelMap },
                            "模型映射设置已更新。"
                          )
                        }
                      >
                        保存模型映射
                      </button>
                    </div>
                  </div>
                </div>
              </div>

              <div>
                <h4 className="text-sm font-semibold mb-1">运行参数</h4>
                <p className="text-xs text-[var(--text-secondary)] mb-4">把刷新周期、并发数和搜索模式收敛到统一字段区</p>
                <div className="admin-form-grid">
                  <div className="admin-form-group">
                    <label>自动刷新间隔（秒）</label>
                    <Input
                      placeholder="自动刷新间隔（秒）"
                      type="number"
                      value={String(settings?.autoRefreshInterval ?? 21600)}
                      onChange={(e) =>
                        setSettings((c) => (c ? { ...c, autoRefreshInterval: Number(e.target.value) || 0 } : c))
                      }
                    />
                    <button
                      className="admin-btn admin-btn-primary admin-btn-sm self-start mt-1"
                      disabled={!settings || savingSettings}
                      onClick={() =>
                        settings &&
                        void saveSettings(
                          "/api/setAutoRefresh",
                          { autoRefresh: settings.autoRefresh, autoRefreshInterval: settings.autoRefreshInterval },
                          "自动刷新设置已更新。"
                        )
                      }
                    >
                      保存刷新参数
                    </button>
                  </div>
                  <div className="admin-form-group">
                    <label>批量登录并发</label>
                    <Input
                      placeholder="批量登录并发"
                      type="number"
                      value={String(settings?.batchLoginConcurrency ?? 5)}
                      onChange={(e) =>
                        setSettings((c) => (c ? { ...c, batchLoginConcurrency: Number(e.target.value) || 1 } : c))
                      }
                    />
                    <button
                      className="admin-btn admin-btn-secondary admin-btn-sm self-start mt-1"
                      disabled={!settings || savingSettings}
                      onClick={() =>
                        settings &&
                        void saveSettings(
                          "/api/setBatchLoginConcurrency",
                          { batchLoginConcurrency: settings.batchLoginConcurrency },
                          "批量登录并发已更新。"
                        )
                      }
                    >
                      保存并发
                    </button>
                  </div>
                  <div className="admin-form-group">
                    <label>搜索信息模式</label>
                    <select
                      className="admin-select"
                      value={settings?.searchInfoMode ?? "text"}
                      onChange={(e) =>
                        setSettings((c) => (c ? { ...c, searchInfoMode: e.target.value as "table" | "text" } : c))
                      }
                    >
                      <option value="text">搜索文本模式</option>
                      <option value="table">搜索表格模式</option>
                    </select>
                    <button
                      className="admin-btn admin-btn-secondary admin-btn-sm self-start mt-1"
                      disabled={!settings || savingSettings}
                      onClick={() =>
                        settings &&
                        void saveSettings(
                          "/api/search-info-mode",
                          { searchInfoMode: settings.searchInfoMode },
                          "搜索模式已更新。"
                        )
                      }
                    >
                      保存搜索模式
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div className="flex flex-col gap-4">
          {/* API Keys */}
          <div className="admin-card">
            <div className="admin-card-header">
              <div>
                <h3>访问密钥</h3>
                <p>统一管理普通 API Key，避免和运行策略混在一起</p>
              </div>
            </div>
            <div className="admin-card-body flex flex-col gap-4">
              <div className="flex gap-3">
                <Input
                  placeholder="新增普通 API Key"
                  value={addKeyValue}
                  onChange={(e) => setAddKeyValue(e.target.value)}
                  className="flex-1"
                />
                <button className="admin-btn admin-btn-primary" onClick={() => void addRegularKey()}>
                  添加 Key
                </button>
              </div>

              <div>
                <h4 className="text-sm font-semibold mb-3">现有 Key 列表</h4>
                {settings?.regularKeys.map((key) => (
                  <div className="admin-key-row" key={key}>
                    <span className="truncate">{key}</span>
                    <button className="admin-btn admin-btn-danger admin-btn-sm" onClick={() => void deleteRegularKey(key)}>
                      删除
                    </button>
                  </div>
                ))}
                {!settings?.regularKeys.length ? (
                  <p className="text-sm text-[var(--text-muted)]">当前没有普通 API Key</p>
                ) : null}
              </div>
            </div>
          </div>

          {/* Refresh & Hot reload */}
          <div className="admin-card">
            <div className="admin-card-header">
              <div>
                <h3>刷新与热更新</h3>
                <p>账号刷新和 .env 重载放在同一组，方便运维操作</p>
              </div>
            </div>
            <div className="admin-card-body flex flex-col gap-5">
              <div className="admin-form-group">
                <label>即将过期阈值（小时）</label>
                <Input
                  placeholder="即将过期阈值（小时）"
                  type="number"
                  value={thresholdHours}
                  onChange={(e) => setThresholdHours(e.target.value)}
                />
              </div>

              <div className="flex gap-3">
                <button className="admin-btn admin-btn-secondary flex-1" onClick={() => void refreshAllAccounts(false)}>
                  阈值刷新
                </button>
                <button className="admin-btn admin-btn-danger flex-1" onClick={() => void refreshAllAccounts(true)}>
                  强制全刷
                </button>
              </div>

              <div className="border-t border-[var(--border)] pt-4">
                <h4 className="text-sm font-semibold mb-1">配置热更新</h4>
                <p className="text-xs text-[var(--text-secondary)] mb-3">
                  后台保存会立即生效并写入 .env；手动改 .env 后可在这里重载
                </p>
                <button
                  className="admin-btn admin-btn-primary"
                  disabled={savingSettings}
                  onClick={() => void reloadRuntimeConfig()}
                >
                  重新加载 .env
                </button>
              </div>

              <div className="p-4 rounded-lg border border-[var(--danger)] bg-[var(--danger-light)] text-sm">
                <strong className="text-[var(--danger)] block mb-1">操作提醒</strong>
                <p className="text-[var(--text-secondary)]">
                  阈值刷新会优先处理接近过期的账号；强制全刷会对整个账号池重新登录；.env 重载只影响运行参数，不会重建已初始化组件
                </p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
