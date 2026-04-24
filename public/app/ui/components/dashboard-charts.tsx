import { Card } from "@heroui/react";
import type { OverviewResponse } from "../types";

type Analytics = OverviewResponse["analytics"];

export function formatCompactNumber(value: number | undefined) {
  const safeValue = Number(value || 0);
  return new Intl.NumberFormat("zh-CN", {
    notation: safeValue >= 10000 ? "compact" : "standard",
    maximumFractionDigits: 1,
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
    <div className="chart-card">
      <div className="chart-header">
        <div>
          <h3>30 分钟请求趋势</h3>
          <p>展示实时 RPM 波动与请求峰值。</p>
        </div>
        <strong>{requestPeak} peak</strong>
      </div>
      <div className="bar-chart">
        {minuteSeries.map((item) => (
          <div className="bar-chart-item" key={`req-${item.time}`}>
            <div
              className="bar-chart-bar bar-chart-bar-request"
              style={{ height: `${Math.max(8, (item.requests / requestPeak) * 100)}%` }}
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
    <div className="chart-card">
      <div className="chart-header">
        <div>
          <h3>30 分钟 Token 吞吐</h3>
          <p>按分钟展示输入输出总吞吐。</p>
        </div>
        <strong>{formatCompactNumber(tokenPeak)} peak</strong>
      </div>
      <div className="bar-chart token-chart">
        {minuteSeries.map((item) => (
          <div className="bar-chart-item" key={`token-${item.time}`}>
            <div className="bar-chart-stack">
              <div
                className="bar-chart-bar bar-chart-bar-output"
                style={{ height: `${Math.max(6, (item.completionTokens / tokenPeak) * 100)}%` }}
                title={`${item.label} / 输出 ${item.completionTokens}`}
              />
              <div
                className="bar-chart-bar bar-chart-bar-input"
                style={{ height: `${Math.max(6, (item.promptTokens / tokenPeak) * 100)}%` }}
                title={`${item.label} / 输入 ${item.promptTokens}`}
              />
            </div>
            <span>{item.label}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

export function RequestMixCard({ analytics }: { analytics: Analytics | undefined }) {
  const mixTotal = Math.max(1, ...(analytics?.requestMix.map((item) => item.value) || [1]));

  return (
    <Card className="panel">
      <Card.Header className="panel-header">
        <Card.Title>请求结构分布</Card.Title>
        <Card.Description>按接口类型拆解流量组成。</Card.Description>
      </Card.Header>
      <Card.Content className="stack-md">
        {(analytics?.requestMix || []).map((item) => (
          <div className="mix-row" key={item.label}>
            <div className="mix-row-head">
              <span>{item.label}</span>
              <strong>{item.value}</strong>
            </div>
            <div className="mix-row-track">
              <div className="mix-row-fill" style={{ width: `${(item.value / mixTotal) * 100}%` }} />
            </div>
          </div>
        ))}
      </Card.Content>
    </Card>
  );
}
