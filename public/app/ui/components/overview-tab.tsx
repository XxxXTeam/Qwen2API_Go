import {
  formatCompactNumber,
  formatDecimal,
  formatUptime,
  RequestMixCard,
  RequestTrendChart,
  TokenThroughputChart,
} from "./dashboard-charts";
import { MetricRow, StatCard } from "./primitives";
import type { OverviewResponse } from "../types";

export function OverviewTab({
  overview,
  modelCounts,
}: {
  overview: OverviewResponse | null;
  modelCounts: {
    total: number;
    thinking: number;
    search: number;
    image: number;
    video: number;
  };
}) {
  const accounts = overview?.accounts;
  const analytics = overview?.analytics;

  return (
    <div className="flex flex-col gap-6">
      {/* KPI Row 1 */}
      <div className="admin-stat-grid">
        <StatCard
          title="账号池总量"
          value={accounts?.total ?? "--"}
          description="分页管理，不把整池账号一次塞进浏览器"
        />
        <StatCard
          title="健康账号"
          value={accounts?.valid ?? "--"}
          description="有效期充足，可参与轮转"
          tone="success"
        />
        <StatCard
          title="即将过期"
          value={accounts?.expiringSoon ?? "--"}
          description="建议提前刷新，减少命中失效账号"
          tone="warning"
        />
        <StatCard
          title="模型总数"
          value={modelCounts.total}
          description={`Thinking ${modelCounts.thinking} / Search ${modelCounts.search}`}
          tone="danger"
        />
      </div>

      {/* KPI Row 2 */}
      <div className="admin-stat-grid">
        <StatCard
          title="业务 RPM"
          value={formatCompactNumber(analytics?.rpm)}
          description={`近 30 分钟均值 ${formatDecimal(analytics?.averageRpm)} rpm，不含后台管理请求`}
          tone="success"
        />
        <StatCard
          title="业务总请求"
          value={formatCompactNumber(analytics?.totals.requests)}
          description={`成功率 ${formatDecimal(analytics?.successRate, 2)}%，错误 ${formatCompactNumber(analytics?.totals.errors)}`}
        />
        <StatCard
          title="总输入 Token"
          value={formatCompactNumber(analytics?.totals.promptTokens)}
          description={`近 30 分钟输入输出合计 ${formatCompactNumber(analytics?.tokens30m)}`}
          tone="warning"
        />
        <StatCard
          title="总输出 Token"
          value={formatCompactNumber(analytics?.totals.completionTokens)}
          description={`累计总 Token ${formatCompactNumber(analytics?.totals.totalTokens)}`}
          tone="danger"
        />
      </div>

      {/* Main charts + side stats */}
      <div className="admin-grid-3">
        <div className="col-span-2 flex flex-col gap-4">
          <RequestTrendChart analytics={analytics} />
          <TokenThroughputChart analytics={analytics} />
        </div>
        <div className="flex flex-col gap-4">
          <RequestMixCard analytics={analytics} />

          <div className="admin-card">
            <div className="admin-card-header">
              <div>
                <h3>账号池健康</h3>
                <p>便于判断是否需要批量刷新或补录账号</p>
              </div>
            </div>
            <div className="admin-card-body">
              <MetricRow label="健康账号" value={accounts?.valid ?? 0} total={accounts?.total ?? 0} />
              <MetricRow label="即将过期" value={accounts?.expiringSoon ?? 0} total={accounts?.total ?? 0} />
              <MetricRow label="已过期" value={accounts?.expired ?? 0} total={accounts?.total ?? 0} />
              <MetricRow label="无效 / 缺失" value={accounts?.invalid ?? 0} total={accounts?.total ?? 0} />
            </div>
          </div>
        </div>
      </div>

      {/* Bottom cards */}
      <div className="admin-grid-4">
        <div className="admin-card">
          <div className="admin-card-header">
            <div>
              <h3>流量拆分</h3>
              <p>业务请求与后台访问彻底分离</p>
            </div>
          </div>
          <div className="admin-card-body space-y-3">
            <div className="flex justify-between text-sm">
              <span className="text-[var(--text-secondary)]">Chat</span>
              <strong>{formatCompactNumber(analytics?.totals.chat)}</strong>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-[var(--text-secondary)]">Models</span>
              <strong>{formatCompactNumber(analytics?.totals.models)}</strong>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-[var(--text-secondary)]">Image / Video</span>
              <strong>{formatCompactNumber((analytics?.totals.image ?? 0) + (analytics?.totals.video ?? 0))}</strong>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-[var(--text-secondary)]">Admin</span>
              <strong>{formatCompactNumber(analytics?.totals.admin)}</strong>
            </div>
          </div>
        </div>

        <div className="admin-card">
          <div className="admin-card-header">
            <div>
              <h3>服务参数</h3>
              <p>关键运行配置快速定位</p>
            </div>
          </div>
          <div className="admin-card-body space-y-3">
            <div className="flex justify-between text-sm">
              <span className="text-[var(--text-secondary)]">监听地址</span>
              <strong className="mono">{overview?.server.listenAddress}:{overview?.server.listenPort}</strong>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-[var(--text-secondary)]">数据模式</span>
              <strong>{overview?.server.dataSaveMode ?? "--"}</strong>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-[var(--text-secondary)]">并发数</span>
              <strong>{overview?.server.batchLoginConcurrency ?? "--"}</strong>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-[var(--text-secondary)]">搜索模式</span>
              <strong>{overview?.server.searchInfoMode ?? "--"}</strong>
            </div>
          </div>
        </div>

        <div className="admin-card">
          <div className="admin-card-header">
            <div>
              <h3>模型供给</h3>
              <p>当前后台可见模型池概况</p>
            </div>
          </div>
          <div className="admin-card-body space-y-3">
            <div className="flex justify-between text-sm">
              <span className="text-[var(--text-secondary)]">模型总数</span>
              <strong>{modelCounts.total}</strong>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-[var(--text-secondary)]">Thinking</span>
              <strong>{modelCounts.thinking}</strong>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-[var(--text-secondary)]">Search</span>
              <strong>{modelCounts.search}</strong>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-[var(--text-secondary)]">媒体模型</span>
              <strong>{modelCounts.image + modelCounts.video}</strong>
            </div>
          </div>
        </div>

        <div className="admin-card">
          <div className="admin-card-header">
            <div>
              <h3>密钥与时间</h3>
              <p>当前控制面的基础元数据</p>
            </div>
          </div>
          <div className="admin-card-body space-y-3">
            <div className="flex justify-between text-sm">
              <span className="text-[var(--text-secondary)]">API Keys</span>
              <strong>{overview?.apiKeys.total ?? "--"}</strong>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-[var(--text-secondary)]">管理员 Key</span>
              <strong>{overview?.apiKeys.admin ?? "--"}</strong>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-[var(--text-secondary)]">普通 Key</span>
              <strong>{overview?.apiKeys.regular ?? "--"}</strong>
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-[var(--text-secondary)]">生成时间</span>
              <strong>
                {overview?.generatedAt
                  ? new Date(overview.generatedAt).toLocaleTimeString("zh-CN", { hour12: false })
                  : "--"}
              </strong>
            </div>
          </div>
        </div>
      </div>

      {/* Operations Overview */}
      <div className="admin-card">
        <div className="admin-card-header">
          <div>
            <h3>运营指标</h3>
            <p>把业务流量、账号池状态、模型供给和后台交互拆成清晰的运营视图</p>
          </div>
        </div>
        <div className="admin-card-body">
          <div className="admin-grid-3">
            <div className="flex flex-col gap-3 p-4 border border-[var(--border)] rounded-lg bg-[var(--surface-hover)]">
              <span className="text-sm text-[var(--text-secondary)]">服务在线</span>
              <strong className="text-xl">{formatUptime(analytics?.uptimeSeconds)}</strong>
            </div>
            <div className="flex flex-col gap-3 p-4 border border-[var(--border)] rounded-lg bg-[var(--surface-hover)]">
              <span className="text-sm text-[var(--text-secondary)]">30 分钟请求</span>
              <strong className="text-xl">{formatCompactNumber(analytics?.requests30m)}</strong>
            </div>
            <div className="flex flex-col gap-3 p-4 border border-[var(--border)] rounded-lg bg-[var(--surface-hover)]">
              <span className="text-sm text-[var(--text-secondary)]">后台请求</span>
              <strong className="text-xl">{formatCompactNumber(analytics?.adminRequests30m)}</strong>
            </div>
            <div className="flex flex-col gap-3 p-4 border border-[var(--border)] rounded-lg bg-[var(--surface-hover)]">
              <span className="text-sm text-[var(--text-secondary)]">请求峰值</span>
              <strong className="text-xl">{formatCompactNumber(analytics?.peakRequests)}</strong>
            </div>
            <div className="flex flex-col gap-3 p-4 border border-[var(--border)] rounded-lg bg-[var(--surface-hover)]">
              <span className="text-sm text-[var(--text-secondary)]">Token 峰值</span>
              <strong className="text-xl">{formatCompactNumber(analytics?.peakTokens)}</strong>
            </div>
            <div className="flex flex-col gap-3 p-4 border border-[var(--border)] rounded-lg bg-[var(--surface-hover)]">
              <span className="text-sm text-[var(--text-secondary)]">上传请求</span>
              <strong className="text-xl">{formatCompactNumber(analytics?.totals.upload)}</strong>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
