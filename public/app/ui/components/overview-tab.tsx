import { Card } from "@heroui/react";
import {
  formatCompactNumber,
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
    <>
      <div className="panel-grid panel-grid-4 dashboard-kpi-grid">
        <StatCard
          title="RPM"
          value={analytics?.rpm ?? "--"}
          description={`最近一分钟请求量，30 分钟平均 ${analytics?.averageRpm ?? "--"} rpm。`}
          tone="success"
        />
        <StatCard
          title="总请求"
          value={formatCompactNumber(analytics?.totals.requests)}
          description={`成功率 ${analytics?.successRate ?? "--"}%，累计错误 ${analytics?.totals.errors ?? 0} 次。`}
        />
        <StatCard
          title="总输入 Token"
          value={formatCompactNumber(analytics?.totals.promptTokens)}
          description="累计 prompt token 消耗。"
          tone="warning"
        />
        <StatCard
          title="总输出 Token"
          value={formatCompactNumber(analytics?.totals.completionTokens)}
          description={`总 token ${formatCompactNumber(analytics?.totals.totalTokens)}。`}
          tone="danger"
        />
      </div>

      <div className="overview-stage">
        <Card className="panel overview-primary-card">
          <Card.Header className="panel-header">
            <Card.Title>实时流量大屏</Card.Title>
            <Card.Description>把请求速率、Token 吞吐和服务运行状态压成一张主舞台卡片。</Card.Description>
          </Card.Header>
          <Card.Content className="stack-lg">
            <div className="overview-hero-metrics">
              <div className="overview-hero-metric">
                <span>服务在线时长</span>
                <strong>{formatUptime(analytics?.uptimeSeconds)}</strong>
              </div>
              <div className="overview-hero-metric">
                <span>Chat 请求</span>
                <strong>{formatCompactNumber(analytics?.totals.chat)}</strong>
              </div>
              <div className="overview-hero-metric">
                <span>Models 请求</span>
                <strong>{formatCompactNumber(analytics?.totals.models)}</strong>
              </div>
              <div className="overview-hero-metric">
                <span>媒体请求</span>
                <strong>{formatCompactNumber((analytics?.totals.image ?? 0) + (analytics?.totals.video ?? 0))}</strong>
              </div>
            </div>

            <RequestTrendChart analytics={analytics} />
            <TokenThroughputChart analytics={analytics} />
          </Card.Content>
        </Card>

        <div className="overview-side-column">
          <RequestMixCard analytics={analytics} />

          <Card className="panel">
            <Card.Header className="panel-header">
              <Card.Title>账号健康分布</Card.Title>
              <Card.Description>按可用状态拆分账号池结构。</Card.Description>
            </Card.Header>
            <Card.Content className="stack-md">
              <MetricRow label="健康账号" value={accounts?.valid ?? 0} total={accounts?.total ?? 0} />
              <MetricRow label="即将过期" value={accounts?.expiringSoon ?? 0} total={accounts?.total ?? 0} />
              <MetricRow label="已过期" value={accounts?.expired ?? 0} total={accounts?.total ?? 0} />
              <MetricRow label="无效 / 缺失" value={accounts?.invalid ?? 0} total={accounts?.total ?? 0} />
            </Card.Content>
          </Card>

          <Card className="panel">
            <Card.Header className="panel-header">
              <Card.Title>服务参数速查</Card.Title>
              <Card.Description>把常用运行参数压缩成右侧索引卡。</Card.Description>
            </Card.Header>
            <Card.Content className="overview-inline-stats">
              <div>
                <span>监听地址</span>
                <strong>{overview?.server.listenAddress}:{overview?.server.listenPort}</strong>
              </div>
              <div>
                <span>数据模式</span>
                <strong>{overview?.server.dataSaveMode ?? "--"}</strong>
              </div>
              <div>
                <span>并发数</span>
                <strong>{overview?.server.batchLoginConcurrency ?? "--"}</strong>
              </div>
              <div>
                <span>API Keys</span>
                <strong>{overview?.apiKeys.total ?? "--"}</strong>
              </div>
            </Card.Content>
          </Card>
        </div>
      </div>

      <div className="panel-grid panel-grid-3 stat-cluster">
        <StatCard
          title="模型映射"
          value={modelCounts.total}
          description={`Thinking ${modelCounts.thinking} / Search ${modelCounts.search}`}
          tone="success"
        />
        <StatCard
          title="媒体模型"
          value={modelCounts.image + modelCounts.video}
          description={`图像 ${modelCounts.image} / 视频 ${modelCounts.video}`}
          tone="warning"
        />
        <StatCard
          title="生成时间"
          value={overview?.generatedAt ? new Date(overview.generatedAt).toLocaleTimeString("zh-CN", { hour12: false }) : "--"}
          description="当前大屏数据的最近生成时间。"
        />
      </div>
    </>
  );
}
