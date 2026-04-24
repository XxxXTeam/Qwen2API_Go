"use client";

import { Button, Card, Input } from "@heroui/react";
import { useMemo, useState } from "react";
import { apiRequest } from "../api";
import type { UploadItem, UploadResponse } from "../types";
import { EndpointItem, SectionTitle } from "./primitives";

export function UploadsTab({ apiKey }: { apiKey: string }) {
  const [files, setFiles] = useState<File[]>([]);
  const [loading, setLoading] = useState(false);
  const [results, setResults] = useState<UploadItem[]>([]);
  const [error, setError] = useState("");

  const totalSize = useMemo(
    () => files.reduce((sum, file) => sum + file.size, 0),
    [files],
  );

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
    <div className="panel-grid panel-grid-2">
      <Card className="panel">
        <Card.Header className="panel-header">
          <Card.Title>文件上传</Card.Title>
          <Card.Description>使用当前登录的 API Key 直接调用后端 `/v1/uploads`，由服务端转传到 OSS。</Card.Description>
        </Card.Header>
        <Card.Content className="stack-lg">
          <SectionTitle
            title="上传面板"
            description="支持一次选择多个文件，上传后返回 OSS 链接和 file_id。"
          />

          <Input
            type="file"
            multiple
            onChange={(event) => setFiles(Array.from(event.target.files || []))}
          />

          <div className="upload-summary">
            <span>已选文件: {files.length}</span>
            <span>总大小: {formatSize(totalSize)}</span>
          </div>

          <div className="wrap-actions">
            <Button variant="primary" isDisabled={!files.length || loading} onPress={() => void submitUploads()}>
              {loading ? "上传中..." : "开始上传"}
            </Button>
            <Button
              variant="secondary"
              isDisabled={loading}
              onPress={() => {
                setFiles([]);
                setResults([]);
                setError("");
              }}
            >
              清空
            </Button>
          </div>

          {error ? <div className="toast toast-error">{error}</div> : null}

          <div className="stack-md">
            {files.map((file) => (
              <div className="upload-file-row" key={`${file.name}-${file.size}-${file.lastModified}`}>
                <strong>{file.name}</strong>
                <span>{file.type || "application/octet-stream"}</span>
                <span>{formatSize(file.size)}</span>
              </div>
            ))}
            {!files.length ? <p className="panel-copy">暂未选择文件。</p> : null}
          </div>
        </Card.Content>
      </Card>

      <Card className="panel">
        <Card.Header className="panel-header">
          <Card.Title>上传结果与接口说明</Card.Title>
          <Card.Description>可直接复制返回的 OSS URL，也能拿 `file_id` 做后续关联。</Card.Description>
        </Card.Header>
        <Card.Content className="stack-lg">
          <div className="stack-md">
            <EndpointItem method="POST" path="/v1/uploads" summary="统一文件/图片/视频上传接口，支持 multipart、raw body、JSON base64。" />
            <EndpointItem method="POST" path="/v1/files/upload" summary="`/v1/uploads` 的兼容别名。" />
          </div>

          <pre className="code-block">{`curl -X POST /v1/uploads \\
  -H "Authorization: Bearer ${apiKey ? "***已登录***" : "sk-admin"}" \\
  -F "files=@demo.png" \\
  -F "files=@demo.mp4"`}</pre>

          <div className="stack-md">
            {results.map((item) => (
              <div className="upload-result-card" key={`${item.file_id}-${item.url}`}>
                <div className="upload-result-head">
                  <strong>{item.filename}</strong>
                  <span>{formatSize(item.size)}</span>
                </div>
                <div className="stack-sm">
                  <span className="panel-copy">类型: {item.content_type}</span>
                  <span className="panel-copy">file_id: {item.file_id}</span>
                  <a className="upload-link" href={item.url} target="_blank" rel="noreferrer">
                    {item.url}
                  </a>
                </div>
              </div>
            ))}
            {!results.length ? <p className="panel-copy">上传成功后，结果会显示在这里。</p> : null}
          </div>
        </Card.Content>
      </Card>
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
