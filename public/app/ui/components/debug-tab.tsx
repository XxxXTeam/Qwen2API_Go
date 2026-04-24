import { Card } from "@heroui/react";
import { EndpointItem } from "./primitives";

export function DebugTab({ apiKey }: { apiKey: string }) {
  return (
    <div className="panel-grid panel-grid-2">
      <Card className="panel">
        <Card.Header className="panel-header">
          <Card.Title>接口速览</Card.Title>
          <Card.Description>常用接口的职责和访问方式。</Card.Description>
        </Card.Header>
        <Card.Content className="stack-md">
          <EndpointItem method="POST" path="/verify" summary="管理员登录校验" />
          <EndpointItem method="GET" path="/api/dashboard/overview" summary="仪表盘总览聚合接口" />
          <EndpointItem method="GET" path="/api/getAllAccounts" summary="服务端分页账号查询接口" />
          <EndpointItem method="POST" path="/api/setAccounts" summary="异步批量导入账号" />
          <EndpointItem method="POST" path="/v1/uploads" summary="独立 OSS 上传接口，支持 multipart / JSON base64 / raw body。" />
          <EndpointItem method="GET" path="/models" summary="公开模型列表" />
        </Card.Content>
      </Card>

      <Card className="panel">
        <Card.Header className="panel-header">
          <Card.Title>请求样例</Card.Title>
          <Card.Description>方便快速复制到 curl / Postman。</Card.Description>
        </Card.Header>
        <Card.Content className="stack-md">
          <pre className="code-block">{`curl -X POST /verify \\
  -H "Content-Type: application/json" \\
  -d '{"apiKey":"sk-admin"}'`}</pre>
          <pre className="code-block">{`curl /api/getAllAccounts?page=1&pageSize=50&status=valid \\
  -H "Authorization: Bearer ${apiKey ? "***已登录***" : "sk-admin"}"`}</pre>
          <pre className="code-block">{`curl -X POST /v1/uploads \\
  -H "Authorization: Bearer ${apiKey ? "***已登录***" : "sk-admin"}" \\
  -F "files=@demo.png"`}</pre>
          <pre className="code-block">{`curl /models`}</pre>
        </Card.Content>
      </Card>
    </div>
  );
}
