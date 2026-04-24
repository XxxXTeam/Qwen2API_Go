export const STORAGE_KEY = "qwen2api-admin-key";

export async function apiRequest<T>(
  path: string,
  options: RequestInit = {},
  apiKey?: string,
): Promise<T> {
  const isFormData = typeof FormData !== "undefined" && options.body instanceof FormData;
  const response = await fetch(path, {
    ...options,
    headers: {
      ...(isFormData ? {} : { "Content-Type": "application/json" }),
      ...(apiKey ? { Authorization: `Bearer ${apiKey}` } : {}),
      ...(options.headers || {}),
    },
    cache: "no-store",
  });

  const text = await response.text();
  const data = text ? JSON.parse(text) : {};

  if (!response.ok) {
    const message =
      typeof data === "object" && data !== null
        ? String((data as { error?: string; message?: string }).error || (data as { message?: string }).message || `请求失败 (${response.status})`)
        : `请求失败 (${response.status})`;
    throw new Error(message);
  }

  return data as T;
}
