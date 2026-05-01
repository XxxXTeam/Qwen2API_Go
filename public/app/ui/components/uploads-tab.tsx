"use client";

import { Input } from "@heroui/react";
import { useMemo, useState } from "react";
import { apiRequest } from "../api";
import type { UploadItem, UploadResponse } from "../types";
import { EndpointItem } from "./primitives";

export function UploadsTab({ apiKey }: { apiKey: string }) {
  const [files, setFiles] = useState<File[]>([]);
  const [loading, setLoading] = useState(false);
  const [results, setResults] = useState<UploadItem[]>([]);
  const [error, setError] = useState("");

  const totalSize = useMemo(() => files.reduce((sum, file) => sum + file.size, 0), [files]);

  async function submitUploads() {
    if (!files.length || !apiKey) {
      return;
    }

    const formData = new FormData();
    for (const file of files) {
      formData.append("files", file);
    }

    try {
      setLoading(true);
      setError("");
      const response = await apiRequest<UploadResponse>(
        "/v1/uploads",
        {
          method: "POST",
          body: formData,
        },
        apiKey,
      );
      setResults(response.data || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "上传失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="admin-grid-2">
      <div className="admin-card">
        <div className="admin-card-header">
          <div>
            <h3>文件上传</h3>
            <p>使用当前登录的 API Key 直接调用后端 /v1/uploads，由服务端转传到 OSS</p>
          </div>
        </div>
        <div className="admin-card-body flex flex-col gap-5">
          <Input
            type="file"
            multiple
            onChange={(event) => setFiles(Array.from(event.target.files || []))}
          />

          <div className="flex gap-6 text-sm text-[var(--text-secondary)]">
            <span>已选文件: <strong className="text-[var(--text)]">{files.length}</strong></span>
            <span>总大小: <strong className="text-[var(--text)]">{formatSize(totalSize)}</strong></span>
          </div>

          <div className="flex gap-3">
            <button
              className="admin-btn admin-btn-primary"
              disabled={!files.length || loading}
              onClick={() => void submitUploads()}
            >
              {loading ? "上传中..." : "开始上传"}
            </button>
            <button
              className="admin-btn admin-btn-secondary"
              disabled={loading}
              onClick={() => {
                setFiles([]);
                setResults([]);
                setError("");
              }}
            >
              清空
            </button>
          </div>

          {error ? (
            <div className="p-3 rounded-lg bg-[var(--danger-light)] text-[var(--danger)] text-sm font-medium">
              {error}
            </div>
          ) : null}

          <div className="flex flex-col gap-2">
            {files.map((file) => (
              <div className="admin-upload-row" key={`${file.name}-${file.size}-${file.lastModified}`}>
                <strong className="text-sm">{file.name}</strong>
                <span className="text-xs text-[var(--text-secondary)]">{file.type || "application/octet-stream"}</span>
                <span className="text-xs text-[var(--text-secondary)]">{formatSize(file.size)}</span>
              </div>
            ))}
            {!files.length ? <p className="text-sm text-[var(--text-muted)]">暂未选择文件</p> : null}
          </div>
        </div>
      </div>

      <div className="admin-card">
        <div className="admin-card-header">
          <div>
            <h3>上传结果与接口说明</h3>
            <p>可直接复制返回的 OSS URL，也能拿 file_id 做后续关联</p>
          </div>
        </div>
        <div className="admin-card-body flex flex-col gap-5">
          <div className="flex flex-col gap-1">
            <EndpointItem method="POST" path="/v1/uploads" summary="统一文件/图片/视频上传接口，支持 multipart、raw body、JSON base64。" />
            <EndpointItem method="POST" path="/v1/files/upload" summary="/v1/uploads 的兼容别名。" />
          </div>

          <pre className="admin-code">{`curl -X POST /v1/uploads \\
  -H "Authorization: Bearer ${apiKey ? "***已登录***" : "sk-admin"}" \\
  -F "files=@demo.png" \\
  -F "files=@demo.mp4"`}</pre>

          <div className="flex flex-col gap-3">
            {results.map((item) => (
              <div className="admin-upload-result" key={`${item.file_id}-${item.url}`}>
                <div className="flex items-center justify-between mb-2">
                  <strong className="text-sm">{item.filename}</strong>
                  <span className="text-xs text-[var(--text-secondary)]">{formatSize(item.size)}</span>
                </div>
                <div className="flex flex-col gap-1 text-xs text-[var(--text-secondary)]">
                  <span>类型: {item.content_type}</span>
                  <span>file_id: <span className="mono">{item.file_id}</span></span>
                  <a
                    className="text-[var(--primary)] hover:underline truncate"
                    href={item.url}
                    target="_blank"
                    rel="noreferrer"
                  >
                    {item.url}
                  </a>
                </div>
              </div>
            ))}
            {!results.length ? <p className="text-sm text-[var(--text-muted)]">上传成功后，结果会显示在这里</p> : null}
          </div>
        </div>
      </div>
    </div>
  );
}

function formatSize(size: number) {
  if (size < 1024) {
    return `${size} B`;
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`;
  }
  if (size < 1024 * 1024 * 1024) {
    return `${(size / 1024 / 1024).toFixed(1)} MB`;
  }
  return `${(size / 1024 / 1024 / 1024).toFixed(2)} GB`;
}
