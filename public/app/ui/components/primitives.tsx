import type { ReactNode } from "react";
import type { Tone } from "../types";

export function StatCard({
  title,
  value,
  description,
  tone = "default",
}: {
  title: string;
  value: string | number;
  description: string;
  tone?: Tone;
}) {
  return (
    <div className={`admin-stat-card ${tone === "default" ? "" : tone}`}>
      <div className="label">{title}</div>
      <div className="value">{value}</div>
      <div className="desc">{description}</div>
    </div>
  );
}

export function SectionTitle({
  title,
  description,
  action,
}: {
  title: string;
  description: string;
  action?: ReactNode;
}) {
  return (
    <div className="admin-section-title">
      <div>
        <h2>{title}</h2>
        <p>{description}</p>
      </div>
      {action}
    </div>
  );
}

export function MetricRow({
  label,
  value,
  total,
}: {
  label: string;
  value: number;
  total: number;
}) {
  const ratio = total > 0 ? (value / total) * 100 : 0;
  return (
    <div className="admin-metric">
      <div className="admin-metric-head">
        <span>{label}</span>
        <strong>{value}</strong>
      </div>
      <div className="admin-progress">
        <div className="admin-progress-fill" style={{ width: `${ratio}%` }} />
      </div>
    </div>
  );
}

export function EndpointItem({
  method,
  path,
  summary,
}: {
  method: string;
  path: string;
  summary: string;
}) {
  const methodColor: Record<string, string> = {
    GET: "bg-[var(--success-light)] text-[var(--success)]",
    POST: "bg-[var(--primary-light)] text-[var(--primary)]",
    DELETE: "bg-[var(--danger-light)] text-[var(--danger)]",
    PUT: "bg-[var(--warning-light)] text-[var(--warning)]",
  };
  return (
    <div className="admin-endpoint">
      <div>
        <div className="flex items-center gap-2">
          <span className={`px-2 py-0.5 rounded text-xs font-semibold ${methodColor[method] || methodColor.POST}`}>
            {method}
          </span>
          <code>{path}</code>
        </div>
        <p>{summary}</p>
      </div>
    </div>
  );
}
