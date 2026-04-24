import { Card, Chip, ProgressBar } from "@heroui/react";
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
    <Card className={`panel panel-stat panel-${tone}`}>
      <Card.Header className="panel-header">
        <Card.Description>{title}</Card.Description>
        <div className="panel-kpi">{value}</div>
      </Card.Header>
      <Card.Content>
        <p className="panel-copy">{description}</p>
      </Card.Content>
    </Card>
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
    <div className="section-title">
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
    <div className="metric-row">
      <div className="metric-row-head">
        <span>{label}</span>
        <strong>{value}</strong>
      </div>
      <ProgressBar value={ratio} />
      <div className="metric-row-foot">{ratio.toFixed(1)}%</div>
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
  return (
    <div className="endpoint-item">
      <div className="endpoint-head">
        <Chip color="accent" variant="soft">
          {method}
        </Chip>
        <code>{path}</code>
      </div>
      <p>{summary}</p>
    </div>
  );
}
