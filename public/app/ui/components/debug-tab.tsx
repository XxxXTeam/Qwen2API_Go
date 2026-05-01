"use client";

import { Input } from "@heroui/react";
import { useMemo, useState } from "react";
import { apiRequest } from "../api";
import type { ChatCompletionResponse, ModelItem } from "../types";
import { EndpointItem } from "./primitives";

export function DebugTab({ apiKey, models }: { apiKey: string; models: ModelItem[] }) {
  const availableModels = useMemo(() => models.map((item) => item.id), [models]);
  const [model, setModel] = useState("");
  const [systemPrompt, setSystemPrompt] = useState("你是一个用于后台调试的助手，请直接、简洁回答。");
  const [message, setMessage] = useState("你好，请简单介绍一下你自己。");
  const [temperature, setTemperature] = useState("0.7");
  const [maxTokens, setMaxTokens] = useState("1024");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<ChatCompletionResponse | null>(null);
  const [raw, setRaw] = useState("");
  const selectedModel = model || availableModels[0] || "";

  async function submitDebugChat() {
    if (!apiKey || !selectedModel || !message.trim()) {
      return;
    }

    try {
      setLoading(true);
      setError("");
      setResult(null);
      setRaw("");

      const messages: Array<{ role: string; content: string }> = [];
      if (systemPrompt.trim()) {
        messages.push({ role: "system", content: systemPrompt.trim() });
      }
      messages.push({ role: "user", content: message.trim() });

      const response = await apiRequest<ChatCompletionResponse>(
        "/v1/chat/completions",
        {
          method: "POST",
          body: JSON.stringify({
            model: selectedModel,
            stream: false,
            temperature: Number(temperature) || 0,
            max_tokens: Number(maxTokens) || 1024,
            messages,
          }),
        },
        apiKey,
      );
      setResult(response);
      setRaw(JSON.stringify(response, null, 2));
    } catch (err) {
      setError(err instanceof Error ? err.message : "调试请求失败");
    } finally {
      setLoading(false);
    }
  }

  const content = result?.choices?.[0]?.message?.content || "";

  return (
    <div className="admin-grid-2">
      <div className="admin-card">
        <div className="admin-card-header">
          <div>
            <h3>对话调试台</h3>
            <p>选择真实模型并直接发起 /v1/chat/completions 请求，用当前登录 Key 调试对话结果</p>
          </div>
        </div>
        <div className="admin-card-body flex flex-col gap-5">
          <div className="admin-form-grid">
            <div className="admin-form-group">
              <label>调试模型</label>
              <select className="admin-select" value={selectedModel} onChange={(e) => setModel(e.target.value)}>
                {availableModels.map((item) => (
                  <option key={item} value={item}>
                    {item}
                  </option>
                ))}
              </select>
            </div>
            <div className="admin-form-group">
              <label>Temperature</label>
              <Input type="number" value={temperature} onChange={(e) => setTemperature(e.target.value)} />
            </div>
            <div className="admin-form-group">
              <label>Max Tokens</label>
              <Input type="number" value={maxTokens} onChange={(e) => setMaxTokens(e.target.value)} />
            </div>
          </div>

          <div className="admin-grid-2">
            <div className="admin-form-group">
              <label>System Prompt</label>
              <textarea
                className="admin-textarea"
                rows={5}
                value={systemPrompt}
                onChange={(e) => setSystemPrompt(e.target.value)}
              />
            </div>
            <div className="admin-form-group">
              <label>User Message</label>
              <textarea
                className="admin-textarea"
                rows={5}
                value={message}
                onChange={(e) => setMessage(e.target.value)}
              />
            </div>
          </div>

          <div className="flex gap-3">
            <button
              className="admin-btn admin-btn-primary"
              disabled={!selectedModel || !message.trim() || loading}
              onClick={() => void submitDebugChat()}
            >
              {loading ? "请求中..." : "发送调试请求"}
            </button>
            <button
              className="admin-btn admin-btn-ghost"
              disabled={loading}
              onClick={() => {
                setResult(null);
                setRaw("");
                setError("");
              }}
            >
              清空结果
            </button>
          </div>

          {error ? (
            <div className="p-3 rounded-lg bg-[var(--danger-light)] text-[var(--danger)] text-sm font-medium">
              {error}
            </div>
          ) : null}

          <div className="admin-grid-2">
            <div className="admin-form-group">
              <label>模型回复</label>
              <div className="admin-debug-box">{content || "发送请求后，这里会显示模型返回内容。"}</div>
            </div>
            <div className="admin-form-group">
              <label>Token Usage</label>
              <div className="space-y-2 text-sm p-4 border border-[var(--border)] rounded-lg bg-[var(--bg)]">
                <div className="flex justify-between">
                  <span className="text-[var(--text-secondary)]">输入</span>
                  <strong>{result?.usage?.prompt_tokens ?? 0}</strong>
                </div>
                <div className="flex justify-between">
                  <span className="text-[var(--text-secondary)]">输出</span>
                  <strong>{result?.usage?.completion_tokens ?? 0}</strong>
                </div>
                <div className="flex justify-between">
                  <span className="text-[var(--text-secondary)]">总计</span>
                  <strong>{result?.usage?.total_tokens ?? 0}</strong>
                </div>
                <div className="flex justify-between">
                  <span className="text-[var(--text-secondary)]">模型</span>
                  <strong className="mono">{result?.model ?? selectedModel ?? "-"}</strong>
                </div>
              </div>
            </div>
          </div>

          <div className="admin-form-group">
            <label>原始响应 JSON</label>
            <pre className="admin-code">{raw || "{ }"}</pre>
          </div>
        </div>
      </div>

      <div className="admin-card">
        <div className="admin-card-header">
          <div>
            <h3>接口速览</h3>
            <p>当前后台里最常用的调试与运维接口</p>
          </div>
        </div>
        <div className="admin-card-body flex flex-col gap-1">
          <EndpointItem method="POST" path="/verify" summary="管理员登录校验" />
          <EndpointItem method="GET" path="/api/dashboard/overview" summary="仪表盘总览聚合接口" />
          <EndpointItem method="GET" path="/api/getAllAccounts" summary="服务端分页账号查询接口" />
          <EndpointItem method="GET" path="/api/models" summary="后台受保护模型列表，可用于调试模型选择。" />
          <EndpointItem method="POST" path="/v1/chat/completions" summary="真实聊天调试入口，支持当前登录 Key 直接联调。" />
          <EndpointItem method="POST" path="/v1/uploads" summary="独立 OSS 上传接口，支持 multipart / JSON base64 / raw body。" />

          <pre className="admin-code mt-4">{`curl -X POST /v1/chat/completions \\
  -H "Authorization: Bearer ${apiKey ? "***已登录***" : "sk-admin"}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model":"${selectedModel || "qwen3-235b-a22b"}",
    "stream":false,
    "messages":[{"role":"user","content":"你好"}]
  }'`}</pre>
        </div>
      </div>
    </div>
  );
}
