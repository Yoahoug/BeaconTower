// ============================================================
// BeaconTower · HTTP 客户端（唯一出口）
// 约束：
// - 所有网络请求必须走此处的 request()，禁止在组件/store 里直接 fetch。
// - 支持超时取消（AbortController）与统一错误归一化。
// - 后端就绪后：把 MOCK_MODE 设为 false，本文件其余逻辑（重试/取消/
//   错误归一化）全部复用，零改动上层调用方。
// ============================================================

export const API_BASE = '/api'
export const REQUEST_TIMEOUT_MS = 8000

/** 归一化后的请求错误：{ code, message, status, retryable } */
export class ApiError extends Error {
  constructor({ code = 'UNKNOWN', message = '请求失败', status = 0, retryable = false } = {}) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
    this.retryable = retryable
  }
}

function toApiError(e, url) {
  if (e instanceof ApiError) return e
  if (e?.name === 'AbortError') {
    return new ApiError({ code: 'TIMEOUT', message: `请求超时（${url}）`, retryable: true })
  }
  return new ApiError({ code: 'NETWORK', message: e?.message || `网络异常（${url}）`, retryable: true })
}

/**
 * 发起 JSON 请求。
 * @param {string} path 以 / 开头的路径，如 /public/summary
 * @param {object} options { method, body, signal, timeoutMs, retries }
 */
export async function request(path, options = {}) {
  const { method = 'GET', body, signal, timeoutMs = REQUEST_TIMEOUT_MS, retries = 1 } = options
  const url = `${API_BASE}${path}`
  let lastError = null

  for (let attempt = 0; attempt <= retries; attempt++) {
    const ctrl = new AbortController()
    const timer = setTimeout(() => ctrl.abort(), timeoutMs)
    // 外部 signal（如组件卸载）联动取消内部 controller
    const onExternalAbort = () => ctrl.abort()
    if (signal) {
      if (signal.aborted) {
        clearTimeout(timer)
        throw new ApiError({ code: 'ABORTED', message: '请求已取消' })
      }
      signal.addEventListener('abort', onExternalAbort, { once: true })
    }

    try {
      const res = await fetch(url, {
        method,
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: body === undefined ? undefined : JSON.stringify(body),
        signal: ctrl.signal,
      })
      clearTimeout(timer)
      if (signal) signal.removeEventListener('abort', onExternalAbort)

      if (res.status === 401) {
        throw new ApiError({ code: 'UNAUTHORIZED', message: '登录已过期，请重新登录', status: 401 })
      }
      if (res.status === 403) {
        throw new ApiError({ code: 'FORBIDDEN', message: '无权限执行该操作', status: 403 })
      }
      if (res.status === 429) {
        throw new ApiError({ code: 'RATE_LIMITED', message: '操作过于频繁，请稍后再试', status: 429, retryable: true })
      }
      if (!res.ok) {
        let msg = `请求失败（HTTP ${res.status}）`
        try {
          const data = await res.json()
          if (data?.message) msg = data.message
        } catch {
          /* 非 JSON 错误体，沿用默认文案 */
        }
        throw new ApiError({ code: 'HTTP_ERROR', message: msg, status: res.status, retryable: res.status >= 500 })
      }
      if (res.status === 204) return null
      return await res.json()
    } catch (e) {
      clearTimeout(timer)
      if (signal) signal.removeEventListener('abort', onExternalAbort)
      lastError = toApiError(e, url)
      // 不可重试的错误（401/403/取消）直接抛出
      if (!lastError.retryable || attempt === retries) throw lastError
      // 指数退避后重试：300ms / 600ms
      await new Promise((r) => setTimeout(r, 300 * (attempt + 1)))
    }
  }

  throw lastError
}

export const http = {
  get: (path, options) => request(path, { ...options, method: 'GET' }),
  post: (path, body, options) => request(path, { ...options, method: 'POST', body }),
  put: (path, body, options) => request(path, { ...options, method: 'PUT', body }),
  patch: (path, body, options) => request(path, { ...options, method: 'PATCH', body }),
  del: (path, options) => request(path, { ...options, method: 'DELETE' }),
}
