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

// 来源 doc/04 §2.4：登录成功后下发可读 Cookie `bt_csrf`，所有非 GET
// 管理 API 必须带 `X-CSRF-Token` 头（双提交校验）。
function getCsrfToken() {
  try {
    if (typeof document === 'undefined' || !document.cookie) return ''
    const m = document.cookie.match(/(?:^|;\s*)bt_csrf=([^;]*)/)
    return m ? decodeURIComponent(m[1]) : ''
  } catch {
    return ''
  }
}

// 来源 doc/04 §2/§5/§6：后端响应统一包装为 {code,msg,data}，code===0 成功；
// code!==0 按错误码表映射为 ApiError（HTTP 状态仍多为 200，故必须解包判断）。
function envelopeError(code, msg, status) {
  const text = typeof msg === 'string' && msg ? msg : ''
  switch (code) {
    case 1002:
      return new ApiError({ code: 'UNAUTHORIZED', message: text || '登录已过期', status, retryable: false })
    case 1003:
      return new ApiError({ code: 'FORBIDDEN', message: text || 'CSRF 校验失败', status, retryable: false })
    case 1004:
      return new ApiError({ code: 'CONFLICT', message: text || '已初始化', status, retryable: false })
    case 1005:
      return new ApiError({ code: 'UNAUTHORIZED', message: text || '用户名或密码错误', status, retryable: false })
    case 1006:
    case 429:
      return new ApiError({ code: 'RATE_LIMITED', message: text || '操作过于频繁，请稍后再试', status, retryable: true })
    case 2001:
    case 2002:
      return new ApiError({ code: 'HTTP_ERROR', message: text || `请求失败（业务码 ${code}）`, status, retryable: false })
    default:
      return new ApiError({ code: 'HTTP_ERROR', message: text || `请求失败（业务码 ${code}）`, status, retryable: status >= 500 })
  }
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
      const headers = { 'Content-Type': 'application/json' }
      // 来源 doc/04 §2.4：非 GET 自动附 X-CSRF-Token（从 document.cookie 读取 bt_csrf）。
      if (method.toUpperCase() !== 'GET') {
        const csrf = getCsrfToken()
        if (csrf) headers['X-CSRF-Token'] = csrf
      }
      const res = await fetch(url, {
        method,
        credentials: 'same-origin',
        headers,
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
          if (data?.msg) msg = data.msg
          else if (data?.message) msg = data.message
          // 来源 doc/04 §5/§6：/api 未命中返回 JSON 404（{code,...}），此处统一按 code 解包。
          if (typeof data?.code === 'number' && data.code !== 0) throw envelopeError(data.code, data.msg ?? data.message, res.status)
        } catch (e) {
          if (e instanceof ApiError) throw e
          /* 非 JSON 错误体，沿用默认文案 */
        }
        throw new ApiError({ code: 'HTTP_ERROR', message: msg, status: res.status, retryable: res.status >= 500 })
      }
      if (res.status === 204) return null
      const payload = await res.json()
      // 来源 doc/04 §2：成功响应包 {code:0,msg,data} → 返回 data；
      // 非包装 JSON（如直返对象）则原样返回，保证兼容。
      if (payload && typeof payload === 'object' && 'code' in payload) {
        if (payload.code !== 0) throw envelopeError(payload.code, payload.msg ?? payload.message, res.status)
        return 'data' in payload ? payload.data : payload
      }
      return payload
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
