import type { Tone } from "./types";

export function formatDateTime(value: string | number | null | undefined) {
  if (!value) {
    return "未设置";
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "无效时间";
  }

  return date.toLocaleString("zh-CN", { hour12: false });
}

export function formatHours(hours: number) {
  if (hours < 0) {
    return "不可用";
  }

  if (hours < 1) {
    return `${Math.round(hours * 60)} 分钟`;
  }

  return `${hours.toFixed(hours < 10 ? 1 : 0)} 小时`;
}

export function getStatusTone(status: string): Tone {
  if (status === "valid") {
    return "success";
  }
  if (status === "expiringSoon") {
    return "warning";
  }
  if (status === "expired" || status === "invalid") {
    return "danger";
  }
  return "default";
}
