import { Button, Card, Chip, Input, ProgressBar } from "@heroui/react";
import type { Dispatch, SetStateAction } from "react";
import type { AccountItem, AccountsResponse, BatchTaskResponse, Filters } from "../types";
import { formatDateTime, formatHours, getStatusTone } from "../utils";
import { SectionTitle } from "./primitives";

type AccountsActions = {
  setNewAccountEmail: (value: string) => void;
  setNewAccountPassword: (value: string) => void;
  setBatchAccountsText: (value: string) => void;
  createAccount: () => Promise<void>;
  createBatchTask: () => Promise<void>;
  refreshAccounts: () => Promise<void>;
  setDraftKeyword: (value: string) => void;
  setFilters: Dispatch<SetStateAction<Filters>>;
  refreshAccount: (email: string) => Promise<void>;
  deleteAccount: (email: string) => Promise<void>;
};

export function AccountsTab({
  accounts,
  batchTask,
  filters,
  draftKeyword,
  newAccountEmail,
  newAccountPassword,
  batchAccountsText,
  loadingAccounts,
  actions,
}: {
  accounts: AccountsResponse | null;
  batchTask: BatchTaskResponse | null;
  filters: Filters;
  draftKeyword: string;
  newAccountEmail: string;
  newAccountPassword: string;
  batchAccountsText: string;
  loadingAccounts: boolean;
  actions: AccountsActions;
}) {
  return (
    <>
      <div className="action-deck">
        <Card className="panel action-panel">
          <Card.Header className="panel-header">
            <Card.Title>新增账号</Card.Title>
            <Card.Description>面向临时补录的快速入口，保持表单聚焦和单一操作路径。</Card.Description>
          </Card.Header>
          <Card.Content className="stack-md">
            <Input placeholder="email@example.com" type="email" value={newAccountEmail} onChange={(e) => actions.setNewAccountEmail(e.target.value)} />
            <Input placeholder="账号密码" type="password" value={newAccountPassword} onChange={(e) => actions.setNewAccountPassword(e.target.value)} />
            <Button variant="primary" onPress={() => void actions.createAccount()}>创建账号</Button>
          </Card.Content>
        </Card>

        <Card className="panel action-panel action-panel-wide">
          <Card.Header className="panel-header">
            <Card.Title>批量导入任务</Card.Title>
            <Card.Description>使用 `email:password` 每行一条，异步任务会自动轮询进度，不阻塞当前视图。</Card.Description>
          </Card.Header>
          <Card.Content className="stack-md">
            <textarea className="app-textarea" rows={8} placeholder={"a@example.com:pass123\nb@example.com:pass456"} value={batchAccountsText} onChange={(e) => actions.setBatchAccountsText(e.target.value)} />
            <Button variant="secondary" onPress={() => void actions.createBatchTask()}>创建批量任务</Button>
            {batchTask ? (
              <div className="task-box">
                <div className="task-header">
                  <strong>{batchTask.message}</strong>
                  <Chip color={getStatusTone(batchTask.status)} variant="soft">{batchTask.status}</Chip>
                </div>
                <ProgressBar value={batchTask.progress} />
                <div className="task-meta">
                  <span>进度 {batchTask.completed}/{batchTask.total}</span>
                  <span>成功 {batchTask.success}</span>
                  <span>失败 {batchTask.failed}</span>
                </div>
              </div>
            ) : null}
          </Card.Content>
        </Card>
      </div>

      <Card className="panel">
        <Card.Header className="panel-header">
          <SectionTitle
            title="账号列表"
            description="服务端分页、状态筛选、排序和搜索，适合几万账号规模。"
            action={<Button variant="ghost" onPress={() => void actions.refreshAccounts()}>{loadingAccounts ? "加载中..." : "刷新列表"}</Button>}
          />
        </Card.Header>
        <Card.Content className="stack-lg">
          <div className="toolbar-grid">
            <Input placeholder="搜索邮箱关键词" value={draftKeyword} onChange={(e) => actions.setDraftKeyword(e.target.value)} />
            <select className="app-select" value={filters.status} onChange={(e) => actions.setFilters((c) => ({ ...c, status: e.target.value, page: 1 }))}>
              <option value="all">全部状态</option>
              <option value="valid">健康</option>
              <option value="expiringSoon">即将过期</option>
              <option value="expired">已过期</option>
              <option value="invalid">无效</option>
            </select>
            <select className="app-select" value={filters.sortBy} onChange={(e) => actions.setFilters((c) => ({ ...c, sortBy: e.target.value, page: 1 }))}>
              <option value="expires">按到期时间</option>
              <option value="email">按邮箱</option>
              <option value="status">按状态</option>
            </select>
            <select className="app-select" value={filters.sortOrder} onChange={(e) => actions.setFilters((c) => ({ ...c, sortOrder: e.target.value as "asc" | "desc", page: 1 }))}>
              <option value="desc">降序</option>
              <option value="asc">升序</option>
            </select>
            <select className="app-select" value={String(filters.pageSize)} onChange={(e) => actions.setFilters((c) => ({ ...c, pageSize: Number(e.target.value), page: 1 }))}>
              <option value="25">每页 25</option>
              <option value="50">每页 50</option>
              <option value="100">每页 100</option>
              <option value="200">每页 200</option>
            </select>
          </div>

          <div className="account-overview">
            <Chip color="accent" variant="soft">当前结果 {accounts?.total ?? 0}</Chip>
            <Chip color="success" variant="soft">健康 {accounts?.filteredStats.valid ?? 0}</Chip>
            <Chip color="warning" variant="soft">即将过期 {accounts?.filteredStats.expiringSoon ?? 0}</Chip>
            <Chip color="danger" variant="soft">异常 {(accounts?.filteredStats.expired ?? 0) + (accounts?.filteredStats.invalid ?? 0)}</Chip>
          </div>

          <div className="table-wrap">
            <table className="app-table">
              <thead>
                <tr>
                  <th>邮箱</th>
                  <th>状态</th>
                  <th>剩余时长</th>
                  <th>到期时间</th>
                  <th>密码</th>
                  <th>令牌</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {accounts?.data.map((account) => (
                  <AccountRow key={account.email} account={account} refreshAccount={actions.refreshAccount} deleteAccount={actions.deleteAccount} />
                ))}
                {!accounts?.data.length ? (
                  <tr><td colSpan={7} className="table-empty">没有匹配到账号数据。</td></tr>
                ) : null}
              </tbody>
            </table>
          </div>

          <div className="pagination-bar">
            <div className="pagination-copy">第 {accounts?.page ?? 1} / {accounts?.totalPages ?? 1} 页，共 {accounts?.total ?? 0} 条</div>
            <div className="row-actions">
              <Button variant="ghost" onPress={() => actions.setFilters((c) => ({ ...c, page: Math.max(1, c.page - 1) }))}>上一页</Button>
              <Button variant="secondary" onPress={() => actions.setFilters((c) => ({ ...c, page: Math.min(accounts?.totalPages ?? c.page, c.page + 1) }))}>下一页</Button>
            </div>
          </div>
        </Card.Content>
      </Card>
    </>
  );
}

function AccountRow({
  account,
  refreshAccount,
  deleteAccount,
}: {
  account: AccountItem;
  refreshAccount: (email: string) => Promise<void>;
  deleteAccount: (email: string) => Promise<void>;
}) {
  return (
    <tr>
      <td>{account.email}</td>
      <td><Chip color={getStatusTone(account.status)} variant="soft">{account.status}</Chip></td>
      <td>{formatHours(account.remainingHours)}</td>
      <td>{formatDateTime(account.expiresAt)}</td>
      <td className="mono">{account.password || "-"}</td>
      <td className="mono">{account.token || "-"}</td>
      <td>
        <div className="row-actions">
          <Button variant="secondary" onPress={() => void refreshAccount(account.email)}>刷新</Button>
          <Button variant="danger" onPress={() => void deleteAccount(account.email)}>删除</Button>
        </div>
      </td>
    </tr>
  );
}
