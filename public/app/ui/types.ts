export type VerifyResponse = {
  status: number;
  message: string;
  isAdmin: boolean;
};

export type OverviewResponse = {
  server: {
    listenAddress: string;
    listenPort: number;
    dataSaveMode: string;
    cacheMode: string;
    searchInfoMode: string;
    outThink: boolean;
    autoRefresh: boolean;
    autoRefreshInterval: number;
    batchLoginConcurrency: number;
    logLevel: string;
    enableFileLog: boolean;
  };
  apiKeys: {
    total: number;
    admin: number;
    regular: number;
  };
  accounts: {
    initialized: boolean;
    total: number;
    valid: number;
    expiringSoon: number;
    expired: number;
    invalid: number;
  };
  analytics: {
    startedAt: string;
    uptimeSeconds: number;
    rpm: number;
    averageRpm: number;
    successRate: number;
    totals: {
      requests: number;
      chat: number;
      models: number;
      image: number;
      video: number;
      errors: number;
      promptTokens: number;
      completionTokens: number;
      totalTokens: number;
    };
    minuteSeries: Array<{
      time: string;
      label: string;
      requests: number;
      chat: number;
      models: number;
      image: number;
      video: number;
      errors: number;
      promptTokens: number;
      completionTokens: number;
      totalTokens: number;
    }>;
    requestMix: Array<{
      label: string;
      value: number;
    }>;
  };
  rotation: Record<string, unknown>;
  generatedAt: string;
};

export type SettingsResponse = {
  adminKey: string | null;
  regularKeys: string[];
  autoRefresh: boolean;
  autoRefreshInterval: number;
  batchLoginConcurrency: number;
  outThink: boolean;
  searchInfoMode: "table" | "text";
  simpleModelMap: boolean;
};

export type AccountStatus = "valid" | "expiringSoon" | "expired" | "invalid";

export type AccountItem = {
  email: string;
  password: string;
  token: string;
  expires: number | null;
  expiresAt: string | null;
  status: AccountStatus;
  remainingHours: number;
};

export type AccountsResponse = {
  overallStats: Record<string, number>;
  filteredStats: Record<string, number>;
  total: number;
  page: number;
  pageSize: number;
  totalPages: number;
  keyword: string;
  status: string;
  sortBy: string;
  sortOrder: "asc" | "desc";
  maskSensitive: boolean;
  data: AccountItem[];
};

export type ModelItem = {
  id: string;
  name: string;
  upstream_id?: string;
  display_name?: string;
};

export type ModelsResponse = {
  data: ModelItem[];
};

export type BatchTaskResponse = {
  taskId: string;
  status: string;
  message: string;
  total: number;
  valid: number;
  skipped: number;
  invalid: number;
  processed: number;
  completed: number;
  pending: number;
  success: number;
  failed: number;
  progress: number;
  concurrency: number;
  activeEmails: string[];
  failedEmails: string[];
  recentResults: Array<{
    email: string;
    status: string;
    message: string;
  }>;
  createdAt: number;
  startedAt: number | null;
  finishedAt: number | null;
};

export type UploadItem = {
  filename: string;
  content_type: string;
  size: number;
  url: string;
  file_id: string;
};

export type UploadResponse = {
  object: string;
  data: UploadItem[];
};

export type Filters = {
  keyword: string;
  status: string;
  sortBy: string;
  sortOrder: "asc" | "desc";
  page: number;
  pageSize: number;
};

export type ToastState = {
  type: "success" | "error" | "info";
  message: string;
} | null;

export type Tone = "default" | "success" | "warning" | "danger";

export type TabKey = "overview" | "accounts" | "settings" | "models" | "uploads" | "debug";

export type ThemeMode = "light" | "dark";
