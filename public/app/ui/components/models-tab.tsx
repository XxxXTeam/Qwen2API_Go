import { Input } from "@heroui/react";
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
    <div className="admin-card">
      <div className="admin-card-header">
        <SectionTitle
          title="模型能力矩阵"
          description="读取后台受保护 /api/models，查看模型变体能力与累计输入/输出 Token"
          action={<Input placeholder="搜索模型" value={keyword} onChange={(e) => setKeyword(e.target.value)} className="w-64" />}
        />
      </div>
      <div className="admin-card-body">
        <div className="admin-stat-grid mb-6">
          <div className="admin-stat-card primary">
            <div className="label">当前模型数</div>
            <div className="value">{formatCompactNumber(models.length)}</div>
          </div>
          <div className="admin-stat-card success">
            <div className="label">活跃变体</div>
            <div className="value">{formatCompactNumber(activeModels)}</div>
          </div>
          <div className="admin-stat-card warning">
            <div className="label">累计输入</div>
            <div className="value">{formatCompactNumber(totals.prompt)}</div>
          </div>
          <div className="admin-stat-card danger">
            <div className="label">累计输出</div>
            <div className="value">{formatCompactNumber(totals.completion)}</div>
          </div>
        </div>

        <div className="admin-model-grid">
          {models.map((model) => (
            <div className="admin-model-card" key={model.id}>
              <div className="flex items-start justify-between gap-3 mb-3">
                <div className="min-w-0">
                  <h4 className="truncate">{model.id}</h4>
                  <p className="id truncate">{model.display_name || model.name || model.upstream_id || "-"}</p>
                </div>
                <div className="flex flex-col items-end gap-1 flex-shrink-0">
                  <span className="text-xs text-[var(--text-muted)]">总 Token</span>
                  <strong className="text-lg">{formatCompactNumber(model.usage?.totalTokens)}</strong>
                </div>
              </div>

              <div className="flex flex-wrap gap-2 mb-4">
                {model.id.includes("thinking") ? <span className="admin-tag success">Thinking</span> : null}
                {model.id.includes("search") ? <span className="admin-tag warning">Search</span> : null}
                {model.id.includes("image") ? <span className="admin-tag primary">Image</span> : null}
                {model.id.includes("video") ? <span className="admin-tag danger">Video</span> : null}
                {model.usage?.totalTokens ? <span className="admin-tag success">活跃</span> : null}
              </div>

              <div className="space-y-2 text-sm">
                <div className="flex justify-between">
                  <span className="text-[var(--text-secondary)]">请求名</span>
                  <strong>{model.name || model.id}</strong>
                </div>
                <div className="flex justify-between">
                  <span className="text-[var(--text-secondary)]">上游 ID</span>
                  <strong className="mono">{model.upstream_id || "-"}</strong>
                </div>
                <div className="flex justify-between">
                  <span className="text-[var(--text-secondary)]">显示名</span>
                  <strong>{model.display_name || "-"}</strong>
                </div>
                <div className="flex justify-between">
                  <span className="text-[var(--text-secondary)]">输入 Token</span>
                  <strong>{formatCompactNumber(model.usage?.promptTokens)}</strong>
                </div>
                <div className="flex justify-between">
                  <span className="text-[var(--text-secondary)]">输出 Token</span>
                  <strong>{formatCompactNumber(model.usage?.completionTokens)}</strong>
                </div>
                <div className="flex justify-between">
                  <span className="text-[var(--text-secondary)]">总 Token</span>
                  <strong>{formatCompactNumber(model.usage?.totalTokens)}</strong>
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
