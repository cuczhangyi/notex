const DEFAULT_TIMEOUT_MS = 30_000;
const TOKEN_KEY = "token";

/**
 * HTTP 请求错误
 */
export class RequestError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
    this.name = "RequestError";
  }
}

/**
 * 处理鉴权失效：清理本地 token 并跳转首页
 */
function handleUnauthorized(status: number, withAuth: boolean) {
  if (!withAuth || status !== 401) {
    return;
  }
  localStorage.removeItem(TOKEN_KEY);
  document.cookie = "token=; path=/; expires=Thu, 01 Jan 1970 00:00:00 GMT";
  window.location.assign("/");
}

export interface RequestOptions extends RequestInit {
  timeoutMs?: number;
  withAuth?: boolean;
}

export interface SSEEventPayload<T = unknown> {
  event: string;
  data: T;
}

export interface SSERequestOptions extends RequestInit {
  withAuth?: boolean;
  onEvent?: (event: SSEEventPayload) => void;
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

  const token = localStorage.getItem(TOKEN_KEY);
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
      handleUnauthorized(resp.status, withAuth);
      throw new RequestError(resp.status, text || `HTTP ${resp.status}`);
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

/**
 * 发起 SSE 请求并按事件流解析服务端返回。
 */
export async function requestSSE<T>(
  url: string,
  options: SSERequestOptions = {},
): Promise<T> {
  const { withAuth = true, headers, onEvent, ...rest } = options;
  const token = localStorage.getItem(TOKEN_KEY);

  const mergedHeaders: Record<string, string> = {
    Accept: "text/event-stream",
    "Content-Type": "application/json",
  };
  if (headers) {
    Object.assign(mergedHeaders, headers as Record<string, string>);
  }
  if (withAuth && token) {
    mergedHeaders["Authorization"] = `Bearer ${token}`;
  }

  const resp = await fetch(url, {
    ...rest,
    headers: mergedHeaders,
  });
  if (!resp.ok) {
    const text = await resp.text();
    handleUnauthorized(resp.status, withAuth);
    throw new RequestError(resp.status, text || `HTTP ${resp.status}`);
  }
  if (!resp.body) {
    throw new Error("SSE 响应流为空");
  }

  const reader = resp.body.getReader();
  const decoder = new TextDecoder("utf-8");
  let buffer = "";
  let result: T | undefined;
  let streamError = "";

  const handleChunk = (chunk: string) => {
    const lines = chunk.split("\n");
    let eventName = "message";
    const dataLines: string[] = [];
    for (const line of lines) {
      if (line.startsWith("event:")) {
        eventName = line.slice(6).trim();
      } else if (line.startsWith("data:")) {
        dataLines.push(line.slice(5).trim());
      }
    }
    const rawData = dataLines.join("\n");
    const parsedData = rawData ? (JSON.parse(rawData) as unknown) : null;
    onEvent?.({ event: eventName, data: parsedData });

    if (eventName === "error") {
      if (parsedData && typeof parsedData === "object" && "error" in parsedData) {
        streamError = String((parsedData as { error: unknown }).error);
      } else {
        streamError = rawData || "SSE 请求失败";
      }
    }
    if (eventName === "result") {
      result = parsedData as T;
    }
  };

  while (true) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }
    buffer += decoder.decode(value, { stream: true });

    let separatorIndex = buffer.indexOf("\n\n");
    while (separatorIndex !== -1) {
      const eventBlock = buffer.slice(0, separatorIndex).trim();
      buffer = buffer.slice(separatorIndex + 2);
      if (eventBlock) {
        handleChunk(eventBlock);
      }
      separatorIndex = buffer.indexOf("\n\n");
    }
  }

  if (buffer.trim()) {
    handleChunk(buffer.trim());
  }

  if (streamError) {
    throw new Error(streamError);
  }
  if (typeof result === "undefined") {
    throw new Error("SSE 未返回最终结果");
  }
  return result;
}
