import type { Dispatch, SetStateAction } from "react";
import type { AccountItem, AccountsResponse, BatchTaskResponse, Filters } from "../types";
import { formatDateTime, formatHours, getStatusTone } from "../utils";
import { SectionTitle } from "./primitives";
import { Input, ProgressBar } from "@heroui/react";

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
    <div className="flex flex-col gap-6">
      {/* Action deck */}
      <div className="admin-grid-2">
        <div className="admin-card">
          <div className="admin-card-header">
            <div>
              <h3>新增账号</h3>
              <p>面向临时补录的快速入口，保持表单聚焦和单一操作路径</p>
            </div>
          </div>
          <div className="admin-card-body flex flex-col gap-4">
            <Input
              placeholder="email@example.com"
              type="email"
              value={newAccountEmail}
              onChange={(e) => actions.setNewAccountEmail(e.target.value)}
            />
            <Input
              placeholder="账号密码"
              type="password"
              value={newAccountPassword}
              onChange={(e) => actions.setNewAccountPassword(e.target.value)}
            />
            <button className="admin-btn admin-btn-primary self-start" onClick={() => void actions.createAccount()}>
              创建账号
            </button>
          </div>
        </div>

        <div className="admin-card">
          <div className="admin-card-header">
            <div>
              <h3>批量导入任务</h3>
              <p>使用 email:password 每行一条，异步任务会自动轮询进度</p>
            </div>
          </div>
          <div className="admin-card-body flex flex-col gap-4">
            <textarea
              className="admin-textarea"
              rows={6}
              placeholder={"a@example.com:pass123\nb@example.com:pass456"}
              value={batchAccountsText}
              onChange={(e) => actions.setBatchAccountsText(e.target.value)}
            />
            <button className="admin-btn admin-btn-secondary self-start" onClick={() => void actions.createBatchTask()}>
              创建批量任务
            </button>
            {batchTask ? (
              <div className="admin-task-box">
                <div className="admin-task-header">
                  <strong className="text-sm">{batchTask.message}</strong>
                  <span className={`admin-tag ${getStatusTone(batchTask.status)}`}>{batchTask.status}</span>
                </div>
                <ProgressBar value={batchTask.progress} />
                <div className="admin-task-meta">
                  <span>进度 {batchTask.completed}/{batchTask.total}</span>
                  <span>成功 {batchTask.success}</span>
                  <span>失败 {batchTask.failed}</span>
                </div>
              </div>
            ) : null}
          </div>
        </div>
      </div>

      {/* Account list */}
      <div className="admin-card">
        <div className="admin-card-header">
          <SectionTitle
            title="账号列表"
            description="服务端分页、状态筛选、排序和搜索，适合几万账号规模"
            action={
              <button className="admin-btn admin-btn-ghost admin-btn-sm" onClick={() => void actions.refreshAccounts()}>
                {loadingAccounts ? "加载中..." : "刷新列表"}
              </button>
            }
          />
        </div>
        <div className="admin-card-body">
          <div className="admin-toolbar">
            <Input
              placeholder="搜索邮箱关键词"
              value={draftKeyword}
              onChange={(e) => actions.setDraftKeyword(e.target.value)}
              className="w-64"
            />
            <select
              className="admin-select w-36"
              value={filters.status}
              onChange={(e) => actions.setFilters((c) => ({ ...c, status: e.target.value, page: 1 }))}
            >
              <option value="all">全部状态</option>
              <option value="valid">健康</option>
              <option value="expiringSoon">即将过期</option>
              <option value="expired">已过期</option>
              <option value="invalid">无效</option>
            </select>
            <select
              className="admin-select w-36"
              value={filters.sortBy}
              onChange={(e) => actions.setFilters((c) => ({ ...c, sortBy: e.target.value, page: 1 }))}
            >
              <option value="expires">按到期时间</option>
              <option value="email">按邮箱</option>
              <option value="status">按状态</option>
            </select>
            <select
              className="admin-select w-28"
              value={filters.sortOrder}
              onChange={(e) =>
                actions.setFilters((c) => ({ ...c, sortOrder: e.target.value as "asc" | "desc", page: 1 }))
              }
            >
              <option value="desc">降序</option>
              <option value="asc">升序</option>
            </select>
            <select
              className="admin-select w-28"
              value={String(filters.pageSize)}
              onChange={(e) =>
                actions.setFilters((c) => ({ ...c, pageSize: Number(e.target.value), page: 1 }))
              }
            >
              <option value="25">每页 25</option>
              <option value="50">每页 50</option>
              <option value="100">每页 100</option>
              <option value="200">每页 200</option>
            </select>
          </div>

          <div className="admin-chips">
            <span className="admin-tag primary">当前结果 {accounts?.total ?? 0}</span>
            <span className="admin-tag success">健康 {accounts?.filteredStats.valid ?? 0}</span>
            <span className="admin-tag warning">即将过期 {accounts?.filteredStats.expiringSoon ?? 0}</span>
            <span className="admin-tag danger">
              异常 {(accounts?.filteredStats.expired ?? 0) + (accounts?.filteredStats.invalid ?? 0)}
            </span>
          </div>

          <div className="admin-table-wrap">
            <table className="admin-table">
              <thead>
                <tr>
                  <th>邮箱</th>
                  <th>状态</th>
                  <th>剩余时长</th>
                  <th>到期时间</th>
                  <th>密码</th>
                  <th>令牌</th>
                  <th className="text-right">操作</th>
                </tr>
              </thead>
              <tbody>
                {accounts?.data.map((account) => (
                  <AccountRow
                    key={account.email}
                    account={account}
                    refreshAccount={actions.refreshAccount}
                    deleteAccount={actions.deleteAccount}
                  />
                ))}
                {!accounts?.data.length ? (
                  <tr>
                    <td colSpan={7} className="empty">
                      没有匹配到账号数据
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>

          <div className="admin-pagination">
            <span>
              第 {accounts?.page ?? 1} / {accounts?.totalPages ?? 1} 页，共 {accounts?.total ?? 0} 条
            </span>
            <div className="admin-pagination-actions">
              <button
                className="admin-btn admin-btn-ghost admin-btn-sm"
                onClick={() => actions.setFilters((c) => ({ ...c, page: Math.max(1, c.page - 1) }))}
              >
                上一页
              </button>
              <button
                className="admin-btn admin-btn-secondary admin-btn-sm"
                onClick={() =>
                  actions.setFilters((c) => ({
                    ...c,
                    page: Math.min(accounts?.totalPages ?? c.page, c.page + 1),
                  }))
                }
              >
                下一页
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
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
      <td className="font-medium">{account.email}</td>
      <td>
        <span className={`admin-tag ${getStatusTone(account.status)}`}>{account.status}</span>
      </td>
      <td>{formatHours(account.remainingHours)}</td>
      <td>{formatDateTime(account.expiresAt)}</td>
      <td className="mono">{account.password || "-"}</td>
      <td className="mono">{account.token || "-"}</td>
      <td className="text-right">
        <div className="flex justify-end gap-2">
          <button
            className="admin-btn admin-btn-secondary admin-btn-sm"
            onClick={() => void refreshAccount(account.email)}
          >
            刷新
          </button>
          <button
            className="admin-btn admin-btn-danger admin-btn-sm"
            onClick={() => void deleteAccount(account.email)}
          >
            删除
          </button>
        </div>
      </td>
    </tr>
  );
}
