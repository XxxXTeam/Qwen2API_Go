import { Card, Chip } from "@heroui/react";
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
    <div className="stack-lg">
      <div className="panel-grid panel-grid-4 dashboard-kpi-grid">
        <StatCard
          title="账号池总量"
          value={accounts?.total ?? "--"}
          description="分页管理，不把整池账号一次塞进浏览器。"
        />
        <StatCard
          title="健康账号"
          value={accounts?.valid ?? "--"}
          description="有效期充足，可参与轮转。"
          tone="success"
        />
        <StatCard
          title="即将过期"
          value={accounts?.expiringSoon ?? "--"}
          description="建议提前刷新，减少命中失效账号。"
          tone="warning"
        />
        <StatCard
          title="模型总数"
          value={modelCounts.total}
          description={`Thinking ${modelCounts.thinking} / Search ${modelCounts.search}`}
          tone="danger"
        />
      </div>

      <div className="panel-grid panel-grid-4 dashboard-kpi-grid">
        <StatCard
          title="业务 RPM"
          value={formatCompactNumber(analytics?.rpm)}
          description={`近 30 分钟均值 ${formatDecimal(analytics?.averageRpm)} rpm，不含后台管理请求。`}
          tone="success"
        />
        <StatCard
          title="业务总请求"
          value={formatCompactNumber(analytics?.totals.requests)}
          description={`成功率 ${formatDecimal(analytics?.successRate, 2)}%，错误 ${formatCompactNumber(analytics?.totals.errors)}。`}
        />
        <StatCard
          title="总输入 Token"
          value={formatCompactNumber(analytics?.totals.promptTokens)}
          description={`近 30 分钟输入输出合计 ${formatCompactNumber(analytics?.tokens30m)}。`}
          tone="warning"
        />
        <StatCard
          title="总输出 Token"
          value={formatCompactNumber(analytics?.totals.completionTokens)}
          description={`累计总 Token ${formatCompactNumber(analytics?.totals.totalTokens)}。`}
          tone="danger"
        />
      </div>

      <div className="overview-command">
        <Card className="panel overview-command-card">
          <Card.Header className="panel-header">
            <div className="overview-command-head">
              <div>
                <p className="eyebrow">Operations Overview</p>
                <Card.Title>控制台总览</Card.Title>
                <Card.Description>把业务流量、账号池状态、模型供给和后台交互拆成清晰的运营视图。</Card.Description>
              </div>
              <div className="hero-tags">
                <Chip color="success" variant="soft">业务流量独立统计</Chip>
                <Chip color="warning" variant="soft">Token 保底估算</Chip>
                <Chip color="accent" variant="soft">后台请求分离</Chip>
              </div>
            </div>
          </Card.Header>
          <Card.Content className="stack-lg">
            <div className="overview-hero-metrics">
              <div className="overview-hero-metric">
                <span>服务在线</span>
                <strong>{formatUptime(analytics?.uptimeSeconds)}</strong>
              </div>
              <div className="overview-hero-metric">
                <span>30 分钟请求</span>
                <strong>{formatCompactNumber(analytics?.requests30m)}</strong>
              </div>
              <div className="overview-hero-metric">
                <span>后台请求</span>
                <strong>{formatCompactNumber(analytics?.adminRequests30m)}</strong>
              </div>
              <div className="overview-hero-metric">
                <span>请求峰值</span>
                <strong>{formatCompactNumber(analytics?.peakRequests)}</strong>
              </div>
              <div className="overview-hero-metric">
                <span>Token 峰值</span>
                <strong>{formatCompactNumber(analytics?.peakTokens)}</strong>
              </div>
              <div className="overview-hero-metric">
                <span>上传请求</span>
                <strong>{formatCompactNumber(analytics?.totals.upload)}</strong>
              </div>
            </div>

            <div className="overview-stage">
              <div className="stack-lg">
                <RequestTrendChart analytics={analytics} />
                <TokenThroughputChart analytics={analytics} />
              </div>

              <div className="overview-side-column">
                <RequestMixCard analytics={analytics} />

                <Card className="panel">
                  <Card.Header className="panel-header">
                    <Card.Title>账号池健康</Card.Title>
                    <Card.Description>便于判断是否需要批量刷新或补录账号。</Card.Description>
                  </Card.Header>
                  <Card.Content className="stack-md">
                    <MetricRow label="健康账号" value={accounts?.valid ?? 0} total={accounts?.total ?? 0} />
                    <MetricRow label="即将过期" value={accounts?.expiringSoon ?? 0} total={accounts?.total ?? 0} />
                    <MetricRow label="已过期" value={accounts?.expired ?? 0} total={accounts?.total ?? 0} />
                    <MetricRow label="无效 / 缺失" value={accounts?.invalid ?? 0} total={accounts?.total ?? 0} />
                  </Card.Content>
                </Card>
              </div>
            </div>
          </Card.Content>
        </Card>
      </div>

      <div className="panel-grid panel-grid-4">
        <Card className="panel">
          <Card.Header className="panel-header">
            <Card.Title>流量拆分</Card.Title>
            <Card.Description>业务请求与后台访问彻底分离。</Card.Description>
          </Card.Header>
          <Card.Content className="overview-inline-stats">
            <div>
              <span>Chat</span>
              <strong>{formatCompactNumber(analytics?.totals.chat)}</strong>
            </div>
            <div>
              <span>Models</span>
              <strong>{formatCompactNumber(analytics?.totals.models)}</strong>
            </div>
            <div>
              <span>Image / Video</span>
              <strong>{formatCompactNumber((analytics?.totals.image ?? 0) + (analytics?.totals.video ?? 0))}</strong>
            </div>
            <div>
              <span>Admin</span>
              <strong>{formatCompactNumber(analytics?.totals.admin)}</strong>
            </div>
          </Card.Content>
        </Card>

        <Card className="panel">
          <Card.Header className="panel-header">
            <Card.Title>服务参数</Card.Title>
            <Card.Description>关键运行配置快速定位。</Card.Description>
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
              <span>搜索模式</span>
              <strong>{overview?.server.searchInfoMode ?? "--"}</strong>
            </div>
          </Card.Content>
        </Card>

        <Card className="panel">
          <Card.Header className="panel-header">
            <Card.Title>模型供给</Card.Title>
            <Card.Description>当前后台可见模型池概况。</Card.Description>
          </Card.Header>
          <Card.Content className="overview-inline-stats">
            <div>
              <span>模型总数</span>
              <strong>{modelCounts.total}</strong>
            </div>
            <div>
              <span>Thinking</span>
              <strong>{modelCounts.thinking}</strong>
            </div>
            <div>
              <span>Search</span>
              <strong>{modelCounts.search}</strong>
            </div>
            <div>
              <span>媒体模型</span>
              <strong>{modelCounts.image + modelCounts.video}</strong>
            </div>
          </Card.Content>
        </Card>

        <Card className="panel">
          <Card.Header className="panel-header">
            <Card.Title>密钥与时间</Card.Title>
            <Card.Description>当前控制面的基础元数据。</Card.Description>
          </Card.Header>
          <Card.Content className="overview-inline-stats">
            <div>
              <span>API Keys</span>
              <strong>{overview?.apiKeys.total ?? "--"}</strong>
            </div>
            <div>
              <span>管理员 Key</span>
              <strong>{overview?.apiKeys.admin ?? "--"}</strong>
            </div>
            <div>
              <span>普通 Key</span>
              <strong>{overview?.apiKeys.regular ?? "--"}</strong>
            </div>
            <div>
              <span>生成时间</span>
              <strong>{overview?.generatedAt ? new Date(overview.generatedAt).toLocaleTimeString("zh-CN", { hour12: false }) : "--"}</strong>
            </div>
          </Card.Content>
        </Card>
      </div>
    </div>
  );
}
