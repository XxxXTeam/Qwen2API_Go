import { Card, Chip, Input } from "@heroui/react";
import type { ModelItem } from "../types";
import { formatCompactNumber } from "./dashboard-charts";
import { SectionTitle } from "./primitives";

export function ModelsTab({
  models,
  keyword,
  setKeyword,
}: {
  models: ModelItem[];
  keyword: string;
  setKeyword: (value: string) => void;
}) {
  const activeModels = models.filter((model) => (model.usage?.totalTokens || 0) > 0).length;
  const totals = models.reduce(
    (acc, model) => {
      acc.prompt += model.usage?.promptTokens || 0;
      acc.completion += model.usage?.completionTokens || 0;
      acc.total += model.usage?.totalTokens || 0;
      return acc;
    },
    { prompt: 0, completion: 0, total: 0 },
  );

  return (
    <Card className="panel">
      <Card.Header className="panel-header">
        <SectionTitle
          title="模型能力矩阵"
          description="读取后台受保护 `/api/models`，查看模型变体能力与累计输入/输出 Token。"
          action={<Input placeholder="搜索模型" value={keyword} onChange={(e) => setKeyword(e.target.value)} />}
        />
      </Card.Header>
      <Card.Content className="stack-lg">
        <div className="model-summary-strip">
          <div className="overview-kpi-item">
            <span>当前模型数</span>
            <strong>{formatCompactNumber(models.length)}</strong>
          </div>
          <div className="overview-kpi-item">
            <span>活跃变体</span>
            <strong>{formatCompactNumber(activeModels)}</strong>
          </div>
          <div className="overview-kpi-item">
            <span>累计输入</span>
            <strong>{formatCompactNumber(totals.prompt)}</strong>
          </div>
          <div className="overview-kpi-item">
            <span>累计输出</span>
            <strong>{formatCompactNumber(totals.completion)}</strong>
          </div>
          <div className="overview-kpi-item">
            <span>累计总量</span>
            <strong>{formatCompactNumber(totals.total)}</strong>
          </div>
        </div>
        <div className="model-grid">
          {models.map((model) => (
            <Card className="panel model-card" key={model.id}>
              <Card.Header className="panel-header">
                <div className="model-card-head">
                  <div className="stack-sm">
                    <Card.Title>{model.id}</Card.Title>
                    <Card.Description>{model.display_name || model.name || model.upstream_id || "-"}</Card.Description>
                  </div>
                  <div className="model-token-pill">
                    <span>总 Token</span>
                    <strong>{formatCompactNumber(model.usage?.totalTokens)}</strong>
                  </div>
                </div>
              </Card.Header>
              <Card.Content className="stack-sm">
                <div className="chips-inline">
                  {model.id.includes("thinking") ? <Chip color="success" variant="soft">Thinking</Chip> : null}
                  {model.id.includes("search") ? <Chip color="warning" variant="soft">Search</Chip> : null}
                  {model.id.includes("image") ? <Chip color="accent" variant="soft">Image</Chip> : null}
                  {model.id.includes("video") ? <Chip color="danger" variant="soft">Video</Chip> : null}
                  {model.usage?.totalTokens ? <Chip color="success" variant="soft">活跃</Chip> : null}
                </div>
                <div className="kv-stack">
                  <div><span>请求名</span><strong>{model.name || model.id}</strong></div>
                  <div><span>上游 ID</span><strong>{model.upstream_id || "-"}</strong></div>
                  <div><span>显示名</span><strong>{model.display_name || "-"}</strong></div>
                  <div><span>输入 Token</span><strong>{formatCompactNumber(model.usage?.promptTokens)}</strong></div>
                  <div><span>输出 Token</span><strong>{formatCompactNumber(model.usage?.completionTokens)}</strong></div>
                  <div><span>总 Token</span><strong>{formatCompactNumber(model.usage?.totalTokens)}</strong></div>
                </div>
              </Card.Content>
            </Card>
          ))}
        </div>
      </Card.Content>
    </Card>
  );
}
