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
        <div className="model-grid">
          {models.map((model) => (
            <Card className="panel model-card" key={model.id}>
              <Card.Header className="panel-header">
                <Card.Title>{model.id}</Card.Title>
                <Card.Description>{model.display_name || model.name || model.upstream_id || "-"}</Card.Description>
              </Card.Header>
              <Card.Content className="stack-sm">
                <div className="chips-inline">
                  {model.id.includes("thinking") ? <Chip color="success" variant="soft">Thinking</Chip> : null}
                  {model.id.includes("search") ? <Chip color="warning" variant="soft">Search</Chip> : null}
                  {model.id.includes("image") ? <Chip color="accent" variant="soft">Image</Chip> : null}
                  {model.id.includes("video") ? <Chip color="danger" variant="soft">Video</Chip> : null}
                </div>
                <div className="kv-stack">
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
