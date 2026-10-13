/**
 * BeaconTower · toy 端 HTTP 客户端（唯一网络出口）
 * 契约对齐 web/src/api/http.js（classic 端既有实现，doc/04 §2/§5/§6）：
 * - 同源 fetch + credentials（会话 cookie bt_session 由浏览器自动携带）
 * - 非 GET 自动附 X-CSRF-Token（从可读 cookie bt_csrf 双提交）
 * - 响应包 {code,msg,data}，code===0 取 data；业务错误码归一化为 ApiError
 * - 非幂等请求不自动重试（防重复创建）
 */

export const API_BASE = '/api';
export const REQUEST_TIMEOUT_MS = 8000;

export class ApiError extends Error {
  code: string;
  status: number;
  bizCode: number;
  retryable: boolean;
  constructor(opts: { code?: string; message?: string; status?: number; bizCode?: number; retryable?: boolean } = {}) {
    super(opts.message ?? '请求失败');
    this.name = 'ApiError';
    this.code = opts.code ?? 'UNKNOWN';
    this.status = opts.status ?? 0;
    this.bizCode = opts.bizCode ?? 0;
    this.retryable = opts.retryable ?? false;
  }
}

/** 会话失效广播：401/1002 到达时通知页面层（监听方负责弹登录） */
export function notifyUnauthorized() {
  try {
    window.dispatchEvent(new CustomEvent('bt:unauthorized'));
  } catch {
    /* 无 window 环境 */
  }
}

function getCsrfToken(): string {
  try {
    if (typeof document === 'undefined' || !document.cookie) return '';
    const m = document.cookie.match(/(?:^|;\s*)bt_csrf=([^;]*)/);
    return m ? decodeURIComponent(m[1]) : '';
  } catch {
    return '';
  }
}

function envelopeError(bizCode: number, msg: unknown, status: number): ApiError {
  const text = typeof msg === 'string' && msg ? msg : '';
  switch (bizCode) {
    case 1002:
      return new ApiError({ code: 'UNAUTHORIZED', message: text || '登录已过期', status, bizCode });
    case 1003:
      return new ApiError({ code: 'FORBIDDEN', message: text || 'CSRF 校验失败', status, bizCode });
    case 1005:
      return new ApiError({ code: 'UNAUTHORIZED', message: text || '用户名或密码错误', status, bizCode });
    case 1006:
    case 429:
      return new ApiError({ code: 'RATE_LIMITED', message: text || '操作过于频繁，请稍后再试', status, bizCode, retryable: true });
    default:
      return new ApiError({ code: 'HTTP_ERROR', message: text || `请求失败（业务码 ${bizCode}）`, status, bizCode, retryable: status >= 500 });
  }
}

type Method = 'GET' | 'POST' | 'PUT' | 'DELETE';

export async function request<T = unknown>(
  path: string,
  options: { method?: Method; body?: unknown; timeoutMs?: number; retries?: number } = {},
): Promise<T> {
  const { method = 'GET', body, timeoutMs = REQUEST_TIMEOUT_MS, retries } = options;
  // 非幂等方法默认不重试：POST 已达服务端但响应丢失时，自动重放会造成重复创建
  const safeRetries = retries ?? (method === 'GET' ? 1 : 0);
  const url = `${API_BASE}${path}`;
  let lastError: ApiError | null = null;

  for (let attempt = 0; attempt <= safeRetries; attempt++) {
    const ctrl = new AbortController();
    const timer = setTimeout(() => ctrl.abort(), timeoutMs);
    try {
      const headers: Record<string, string> = {};
      if (body !== undefined) headers['Content-Type'] = 'application/json';
      if (method !== 'GET') {
        const csrf = getCsrfToken();
        if (csrf) headers['X-CSRF-Token'] = csrf;
      }
      const res = await fetch(url, {
        method,
        credentials: 'same-origin',
        headers,
        body: body === undefined ? undefined : JSON.stringify(body),
        signal: ctrl.signal,
      });
      clearTimeout(timer);

      // HTTP 401/403/429 附带的 {code,msg} 优先按业务码解包，保留后端原 msg
      if (res.status === 401 || res.status === 403 || res.status === 429) {
        try {
          const data = await res.clone().json();
          if (typeof data?.code === 'number' && data.code !== 0) {
            const err = envelopeError(data.code, data.msg ?? data.message, res.status);
            if (err.code === 'UNAUTHORIZED') notifyUnauthorized();
            throw err;
          }
        } catch (e) {
          if (e instanceof ApiError) throw e;
          /* 非 JSON 错误体，沿用下方默认文案 */
        }
      }
      if (res.status === 401) {
        notifyUnauthorized();
        throw new ApiError({ code: 'UNAUTHORIZED', message: '登录已过期，请重新登录', status: 401 });
      }
      if (res.status === 403) {
        throw new ApiError({ code: 'FORBIDDEN', message: '无权限执行该操作', status: 403 });
      }
      if (res.status === 429) {
        throw new ApiError({ code: 'RATE_LIMITED', message: '操作过于频繁，请稍后再试', status: 429, retryable: true });
      }
      if (!res.ok) {
        let msg = `请求失败（HTTP ${res.status}）`;
        try {
          const data = await res.json();
          if (data?.msg) msg = data.msg;
          else if (data?.message) msg = data.message;
          if (typeof data?.code === 'number' && data.code !== 0) throw envelopeError(data.code, data.msg ?? data.message, res.status);
        } catch (e) {
          if (e instanceof ApiError) throw e;
          /* 非 JSON 错误体，沿用默认文案 */
        }
        throw new ApiError({ code: 'HTTP_ERROR', message: msg, status: res.status, retryable: res.status >= 500 });
      }
      if (res.status === 204) return null as T;
      const payload = await res.json();
      // 成功响应包 {code:0,msg,data} → 返回 data；非包装 JSON 原样返回
      if (payload && typeof payload === 'object' && 'code' in payload) {
        if (payload.code !== 0) throw envelopeError(payload.code, payload.msg ?? payload.message, res.status);
        return ('data' in payload ? payload.data : payload) as T;
      }
      return payload as T;
    } catch (e) {
      clearTimeout(timer);
      if (e instanceof ApiError) throw e;
      if (e instanceof Error && e.name === 'AbortError') {
        lastError = new ApiError({ code: 'TIMEOUT', message: `请求超时（${path}）`, retryable: true });
      } else {
        lastError = new ApiError({ code: 'NETWORK', message: e instanceof Error ? e.message : `网络异常（${path}）`, retryable: true });
      }
      if (!lastError.retryable || attempt === safeRetries) throw lastError;
      // 指数退避后重试：300ms / 600ms
      await new Promise((r) => setTimeout(r, 300 * (attempt + 1)));
    }
  }
  throw lastError ?? new ApiError({});
}

export const http = {
  get: <T = unknown>(path: string, opts?: { timeoutMs?: number; retries?: number }) =>
    request<T>(path, { ...opts, method: 'GET' }),
  post: <T = unknown>(path: string, body?: unknown, opts?: { timeoutMs?: number; retries?: number }) =>
    request<T>(path, { ...opts, method: 'POST', body }),
  put: <T = unknown>(path: string, body?: unknown, opts?: { timeoutMs?: number; retries?: number }) =>
    request<T>(path, { ...opts, method: 'PUT', body }),
  del: <T = unknown>(path: string, opts?: { timeoutMs?: number; retries?: number }) =>
    request<T>(path, { ...opts, method: 'DELETE' }),
};

/** Promise 超时包装（对齐 classic withTimeout 语义） */
export function withTimeout<T>(promise: Promise<T>, ms = 15000): Promise<T> {
  let timer: ReturnType<typeof setTimeout>;
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => reject(new Error('请求超时，请重试')), ms);
  });
  return Promise.race([promise, timeout]).finally(() => clearTimeout(timer));
}

/**
 * 非 JSON 响应（frpc 配置 text/plain、WG 凭证 zip）的下载出口。
 * 不能走 request()：它无条件 res.json()，二进制/文本会被解析炸掉。
 * 错误体仍是 JSON 封套，这里手工解出来保留后端原 msg。
 */
export async function downloadRaw(
  path: string,
  opts: { timeoutMs?: number } = {},
): Promise<{ blob: Blob; filename: string }> {
  const ctrl = new AbortController();
  const timer = setTimeout(() => ctrl.abort(), opts.timeoutMs ?? 60000);
  try {
    const res = await fetch(`${API_BASE}${path}`, { credentials: 'same-origin', signal: ctrl.signal });
    if (!res.ok) {
      let msg = `下载失败（HTTP ${res.status}）`;
      try {
        const data = await res.clone().json();
        if (data?.msg) msg = data.msg;
        if (typeof data?.code === 'number' && data.code !== 0) throw envelopeError(data.code, data.msg, res.status);
      } catch (e) {
        if (e instanceof ApiError) throw e;
      }
      throw new ApiError({ code: 'HTTP_ERROR', message: msg, status: res.status });
    }
    const dispo = res.headers.get('Content-Disposition') || '';
    const m = /filename\*?=(?:UTF-8'')?"?([^";]+)"?/i.exec(dispo);
    const filename = m ? decodeURIComponent(m[1]) : '';
    return { blob: await res.blob(), filename };
  } catch (e) {
    if (e instanceof ApiError) throw e;
    if (e instanceof Error && e.name === 'AbortError') {
      throw new ApiError({ code: 'TIMEOUT', message: '下载超时，请重试' });
    }
    throw new ApiError({ code: 'NETWORK', message: e instanceof Error ? e.message : '下载失败' });
  } finally {
    clearTimeout(timer);
  }
}

/** 触发浏览器保存一个 blob（下载 = 用户主动动作，失败由调用方提示） */
export function saveBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}
