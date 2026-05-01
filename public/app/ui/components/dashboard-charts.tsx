import type { OverviewResponse } from "../types";

type Analytics = OverviewResponse["analytics"];

export function formatCompactNumber(value: number | undefined) {
  const safeValue = Number(value || 0);
  return new Intl.NumberFormat("zh-CN", {
    notation: safeValue >= 10000 ? "compact" : "standard",
    maximumFractionDigits: 1,
  }).format(safeValue);
}

export function formatDecimal(value: number | undefined, digits = 1) {
  const safeValue = Number(value || 0);
  return new Intl.NumberFormat("zh-CN", {
    minimumFractionDigits: 0,
    maximumFractionDigits: digits,
  }).format(safeValue);
}

export function formatUptime(seconds: number | undefined) {
  const safeSeconds = Math.max(0, Number(seconds || 0));
  const hours = Math.floor(safeSeconds / 3600);
  const minutes = Math.floor((safeSeconds % 3600) / 60);
  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }
  return `${minutes}m`;
}

export function RequestTrendChart({ analytics }: { analytics: Analytics | undefined }) {
  const minuteSeries = analytics?.minuteSeries || [];
  const requestPeak = Math.max(1, ...minuteSeries.map((item) => item.requests));

  return (
    <div className="admin-chart-card">
      <div className="admin-chart-header">
        <div>
          <h4>30 分钟请求趋势</h4>
          <p>实时 RPM 波动与请求峰值</p>
        </div>
        <strong className="text-sm text-[var(--primary)]">{requestPeak} peak</strong>
      </div>
      <div className="admin-bar-chart">
        {minuteSeries.map((item) => (
          <div className="admin-bar-chart-item" key={`req-${item.time}`}>
            <div
              className="admin-bar-chart-bar"
              style={{ height: `${Math.max(4, (item.requests / requestPeak) * 100)}%` }}
              title={`${item.label} / ${item.requests} req`}
            />
            <span>{item.label}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

export function TokenThroughputChart({ analytics }: { analytics: Analytics | undefined }) {
  const minuteSeries = analytics?.minuteSeries || [];
  const tokenPeak = Math.max(1, ...minuteSeries.map((item) => item.totalTokens));

  return (
    <div className="admin-chart-card">
      <div className="admin-chart-header">
        <div>
          <h4>30 分钟 Token 吞吐</h4>
          <p>按分钟展示输入输出总吞吐</p>
        </div>
        <strong className="text-sm text-[var(--primary)]">{formatCompactNumber(tokenPeak)} peak</strong>
      </div>
      <div className="admin-bar-chart">
        {minuteSeries.map((item) => (
          <div className="admin-bar-chart-item" key={`token-${item.time}`}>
            <div
              className="admin-bar-chart-bar"
              style={{
                height: `${Math.max(4, (item.totalTokens / tokenPeak) * 100)}%`,
                background:
                  item.totalTokens > 0
                    ? `linear-gradient(180deg, var(--primary) 60%, var(--warning) 100%)`
                    : undefined,
              }}
              title={`${item.label} / 输入 ${item.promptTokens} / 输出 ${item.completionTokens}`}
            />
            <span>{item.label}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

export function RequestMixCard({ analytics }: { analytics: Analytics | undefined }) {
  const mixTotal = Math.max(1, ...(analytics?.requestMix.map((item) => item.value) || [1]));
  const colors = ["var(--primary)", "var(--success)", "var(--warning)", "var(--danger)", "var(--text-muted)"];

  return (
    <div className="admin-card">
      <div className="admin-card-header">
        <div>
          <h3>请求结构分布</h3>
          <p>按接口类型拆解流量组成</p>
        </div>
      </div>
      <div className="admin-card-body">
        {(analytics?.requestMix || []).map((item, idx) => (
          <div className="admin-mix-row" key={item.label}>
            <div className="admin-mix-row-head">
              <span>{item.label}</span>
              <strong>{item.value}</strong>
            </div>
            <div className="admin-mix-track">
              <div
                className="admin-mix-fill"
                style={{
                  width: `${(item.value / mixTotal) * 100}%`,
                  background: colors[idx % colors.length],
                }}
              />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
