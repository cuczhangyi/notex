const DEFAULT_TIMEOUT_MS = 30_000;

export interface RequestOptions extends RequestInit {
  timeoutMs?: number;
  withAuth?: boolean;
}

/**
 * 统一请求封装，复用现有后端 API 协议
 */
export async function request<T>(url: string, options: RequestOptions = {}): Promise<T> {
  const {
    timeoutMs = DEFAULT_TIMEOUT_MS,
    withAuth = true,
    headers,
    ...rest
  } = options;

  const token = localStorage.getItem("token");
  const mergedHeaders: Record<string, string> = {};
  const isFormDataBody = typeof FormData !== "undefined" && rest.body instanceof FormData;
  if (!isFormDataBody) {
    mergedHeaders["Content-Type"] = "application/json";
  }
  if (headers) {
    Object.assign(mergedHeaders, headers as Record<string, string>);
  }

  if (withAuth && token) {
    mergedHeaders["Authorization"] = `Bearer ${token}`;
  }

  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), timeoutMs);

  try {
    const resp = await fetch(url, {
      ...rest,
      headers: mergedHeaders,
      signal: controller.signal,
    });

    if (!resp.ok) {
      const text = await resp.text();
      throw new Error(text || `HTTP ${resp.status}`);
    }
    if (resp.status === 204) {
      return undefined as T;
    }
    const contentType = resp.headers.get("content-type") || "";
    if (contentType.includes("application/json")) {
      return (await resp.json()) as T;
    }
    return (await resp.text()) as T;
  } finally {
    window.clearTimeout(timer);
  }
}
