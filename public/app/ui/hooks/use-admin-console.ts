"use client";

import { useCallback, useDeferredValue, useEffect, useMemo, useState, useTransition } from "react";
import { apiRequest, STORAGE_KEY } from "../api";
import type {
  AccountsResponse,
  BatchTaskResponse,
  Filters,
  ModelItem,
  ModelsResponse,
  OverviewResponse,
  SettingsResponse,
  TabKey,
  ThemeMode,
  ToastState,
  VerifyResponse,
} from "../types";

const DEFAULT_FILTERS: Filters = {
  keyword: "",
  status: "all",
  sortBy: "expires",
  sortOrder: "desc",
  page: 1,
  pageSize: 50,
};
const THEME_STORAGE_KEY = "qwen2api-theme";

function getInitialTheme(): ThemeMode {
  if (typeof window === "undefined") {
    return "light";
  }

  const savedTheme = window.localStorage.getItem(THEME_STORAGE_KEY);
  if (savedTheme === "light" || savedTheme === "dark") {
    return savedTheme;
  }

  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

export function useAdminConsole() {
  const [apiKeyInput, setApiKeyInput] = useState(() => {
    if (typeof window === "undefined") {
      return "";
    }
    return window.localStorage.getItem(STORAGE_KEY) || "";
  });
  const [apiKey, setApiKey] = useState(() => {
    if (typeof window === "undefined") {
      return "";
    }
    return window.localStorage.getItem(STORAGE_KEY) || "";
  });
  const [verified, setVerified] = useState(false);
  const [toast, setToast] = useState<ToastState>(null);
  const [overview, setOverview] = useState<OverviewResponse | null>(null);
  const [settings, setSettings] = useState<SettingsResponse | null>(null);
  const [accounts, setAccounts] = useState<AccountsResponse | null>(null);
  const [models, setModels] = useState<ModelItem[]>([]);
  const [batchTask, setBatchTask] = useState<BatchTaskResponse | null>(null);
  const [filters, setFilters] = useState<Filters>(DEFAULT_FILTERS);
  const [draftKeyword, setDraftKeyword] = useState("");
  const deferredKeyword = useDeferredValue(draftKeyword);
  const [modelKeyword, setModelKeyword] = useState("");
  const deferredModelKeyword = useDeferredValue(modelKeyword);
  const [newAccountEmail, setNewAccountEmail] = useState("");
  const [newAccountPassword, setNewAccountPassword] = useState("");
  const [batchAccountsText, setBatchAccountsText] = useState("");
  const [addKeyValue, setAddKeyValue] = useState("");
  const [thresholdHours, setThresholdHours] = useState("24");
  const [savingSettings, setSavingSettings] = useState(false);
  const [activeTab, setActiveTab] = useState<TabKey>("overview");
  const [themeMode, setThemeMode] = useState<ThemeMode>(getInitialTheme);
  const [loadingShell, startShellTransition] = useTransition();
  const [loadingAccounts, startAccountsTransition] = useTransition();

  const filteredModels = useMemo(() => {
    const keyword = deferredModelKeyword.trim().toLowerCase();
    if (!keyword) {
      return models;
    }

    return models.filter((model) =>
      [model.id, model.name, model.display_name, model.upstream_id]
        .filter(Boolean)
        .some((item) => String(item).toLowerCase().includes(keyword)),
    );
  }, [deferredModelKeyword, models]);

  const modelCounts = useMemo(
    () => ({
      total: models.length,
      thinking: models.filter((item) => item.id.includes("thinking")).length,
      search: models.filter((item) => item.id.includes("search")).length,
      image: models.filter((item) => item.id.includes("image")).length,
      video: models.filter((item) => item.id.includes("video")).length,
    }),
    [models],
  );

  const loadShell = useCallback(async (overrideKey?: string) => {
    const requestKey = overrideKey || apiKey;
    if (!requestKey) {
      return;
    }

    try {
      const [overviewRes, settingsRes, modelsRes] = await Promise.all([
        apiRequest<OverviewResponse>("/api/dashboard/overview", {}, requestKey),
        apiRequest<SettingsResponse>("/api/settings", {}, requestKey),
        apiRequest<ModelsResponse>("/models"),
      ]);

      setOverview(overviewRes);
      setSettings(settingsRes);
      setModels(modelsRes.data || []);
    } catch (error) {
      setToast({ type: "error", message: error instanceof Error ? error.message : "加载控制台失败" });
    }
  }, [apiKey]);

  const loadAccounts = useCallback(async (overrideKey?: string) => {
    const requestKey = overrideKey || apiKey;
    if (!requestKey) {
      return;
    }

    try {
      const query = new URLSearchParams({
        page: String(filters.page),
        pageSize: String(filters.pageSize),
        keyword: filters.keyword,
        status: filters.status,
        sortBy: filters.sortBy,
        sortOrder: filters.sortOrder,
      });

      const response = await apiRequest<AccountsResponse>(`/api/getAllAccounts?${query.toString()}`, {}, requestKey);
      setAccounts(response);
    } catch (error) {
      setToast({ type: "error", message: error instanceof Error ? error.message : "加载账号列表失败" });
    }
  }, [apiKey, filters.page, filters.pageSize, filters.keyword, filters.sortBy, filters.sortOrder, filters.status]);

  const pollBatchTask = useCallback(async (taskId: string) => {
    if (!apiKey) {
      return;
    }

    try {
      const response = await apiRequest<BatchTaskResponse>(`/api/batchTasks/${taskId}`, {}, apiKey);
      setBatchTask(response);

      if (response.status === "completed") {
        setToast({ type: "success", message: response.message });
        await loadAccounts();
        await loadShell();
      }
    } catch (error) {
      setToast({ type: "error", message: error instanceof Error ? error.message : "批量任务查询失败" });
    }
  }, [apiKey, loadAccounts, loadShell]);

  const verifyAdmin = useCallback(async (key: string, silent = false) => {
    try {
      const result = await apiRequest<VerifyResponse>("/verify", {
        method: "POST",
        body: JSON.stringify({ apiKey: key }),
      });

      if (!result.isAdmin) {
        throw new Error("当前 API Key 不是管理员密钥");
      }

      window.localStorage.setItem(STORAGE_KEY, key);
      setApiKey(key);
      setVerified(true);
      if (!silent) {
        setToast({ type: "success", message: "管理员验证成功，已载入控制台。" });
      }

      startShellTransition(() => {
        void loadShell(key);
      });
    } catch (error) {
      setVerified(false);
      setApiKey("");
      window.localStorage.removeItem(STORAGE_KEY);
      setToast({ type: "error", message: error instanceof Error ? error.message : "验证失败" });
    }
  }, [loadShell]);

  useEffect(() => {
    if (apiKey) {
      const timeout = window.setTimeout(() => {
        void verifyAdmin(apiKey, true);
      }, 0);

      return () => window.clearTimeout(timeout);
    }
  }, [apiKey, verifyAdmin]);

  useEffect(() => {
    if (verified && apiKey) {
      startAccountsTransition(() => {
        void loadAccounts();
      });
    }
  }, [verified, apiKey, filters.page, filters.pageSize, filters.sortBy, filters.sortOrder, filters.status, loadAccounts]);

  useEffect(() => {
    const trimmed = deferredKeyword.trim();
    const timeout = window.setTimeout(() => {
      setFilters((current) => {
        if (current.keyword === trimmed && current.page === 1) {
          return current;
        }
        return { ...current, keyword: trimmed, page: 1 };
      });
    }, 250);

    return () => window.clearTimeout(timeout);
  }, [deferredKeyword]);

  useEffect(() => {
    if (!verified || !apiKey || !batchTask?.taskId) {
      return;
    }
    if (batchTask.status === "completed" || batchTask.status === "failed") {
      return;
    }

    const timer = window.setInterval(() => {
      void pollBatchTask(batchTask.taskId);
    }, 2000);

    return () => window.clearInterval(timer);
  }, [verified, apiKey, batchTask, pollBatchTask]);

  useEffect(() => {
    if (!toast) {
      return;
    }

    const timeout = window.setTimeout(() => setToast(null), 4000);
    return () => window.clearTimeout(timeout);
  }, [toast]);

  useEffect(() => {
    document.documentElement.dataset.theme = themeMode;
    document.documentElement.classList.toggle("dark", themeMode === "dark");
    window.localStorage.setItem(THEME_STORAGE_KEY, themeMode);
  }, [themeMode]);

  async function submitAction<T = unknown>(path: string, body?: Record<string, unknown>, method = "POST") {
    if (!apiKey) {
      return null;
    }

    return apiRequest<T>(
      path,
      {
        method,
        body: body ? JSON.stringify(body) : undefined,
      },
      apiKey,
    );
  }

  async function saveSettings(path: string, body: Record<string, unknown>, successMessage: string) {
    try {
      setSavingSettings(true);
      await submitAction(path, body);
      setToast({ type: "success", message: successMessage });
      await loadShell();
    } catch (error) {
      setToast({ type: "error", message: error instanceof Error ? error.message : "保存失败" });
    } finally {
      setSavingSettings(false);
    }
  }

  return {
    state: {
      apiKeyInput,
      verified,
      toast,
      overview,
      settings,
      accounts,
      filteredModels,
      modelCounts,
      batchTask,
      filters,
      draftKeyword,
      modelKeyword,
      newAccountEmail,
      newAccountPassword,
      batchAccountsText,
      addKeyValue,
      thresholdHours,
      savingSettings,
      activeTab,
      themeMode,
      loadingShell,
      loadingAccounts,
      apiKey,
    },
    actions: {
      setApiKeyInput,
      verifyAdmin: () => verifyAdmin(apiKeyInput),
      logout: () => {
        setVerified(false);
        setApiKey("");
        setApiKeyInput("");
        window.localStorage.removeItem(STORAGE_KEY);
      },
      refreshShell: () => loadShell(),
      refreshAccounts: () => loadAccounts(),
      setFilters,
      setDraftKeyword,
      setModelKeyword,
      setNewAccountEmail,
      setNewAccountPassword,
      setBatchAccountsText,
      setAddKeyValue,
      setThresholdHours,
      setSettings,
      setActiveTab,
      toggleTheme: () => setThemeMode((current) => (current === "light" ? "dark" : "light")),
      createAccount: async () => {
        await submitAction("/api/setAccount", {
          email: newAccountEmail,
          password: newAccountPassword,
        });
        setNewAccountEmail("");
        setNewAccountPassword("");
        setToast({ type: "success", message: "账号创建成功。" });
        await loadAccounts();
        await loadShell();
      },
      createBatchTask: async () => {
        const response = await submitAction<BatchTaskResponse>("/api/setAccounts", {
          accounts: batchAccountsText,
          async: true,
        });
        if (response) {
          setBatchTask(response);
          setBatchAccountsText("");
          setToast({ type: "info", message: "批量任务已创建，后台正在处理。" });
        }
      },
      deleteAccount: async (email: string) => {
        await submitAction("/api/deleteAccount", { email }, "DELETE");
        setToast({ type: "success", message: `已删除 ${email}` });
        await loadAccounts();
        await loadShell();
      },
      refreshAccount: async (email: string) => {
        await submitAction("/api/refreshAccount", { email });
        setToast({ type: "success", message: `${email} 刷新成功` });
        await loadAccounts();
        await loadShell();
      },
      refreshAllAccounts: async (force: boolean) => {
        const path = force ? "/api/forceRefreshAllAccounts" : "/api/refreshAllAccounts";
        const body = force ? {} : { thresholdHours: Number(thresholdHours) || 24 };
        await submitAction(path, body);
        setToast({ type: "success", message: force ? "已触发全量强刷。" : "已触发阈值刷新。" });
        await loadAccounts();
        await loadShell();
      },
      addRegularKey: async () => {
        await submitAction("/api/addRegularKey", { apiKey: addKeyValue });
        setAddKeyValue("");
        setToast({ type: "success", message: "普通 API Key 已添加。" });
        await loadShell();
      },
      deleteRegularKey: async (key: string) => {
        await submitAction("/api/deleteRegularKey", { apiKey: key });
        setToast({ type: "success", message: "普通 API Key 已删除。" });
        await loadShell();
      },
      saveSettings,
    },
  };
}
