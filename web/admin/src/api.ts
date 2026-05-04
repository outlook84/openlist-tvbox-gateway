import { APIError, type AdminConfig, type Backend, type BackendTestResult, type ConfigMeta, type ErrorParams, type LogResponse, type SessionState } from "./types";

const authExpiredListeners = new Set<() => void>();

export function onAuthExpired(listener: () => void) {
  authExpiredListeners.add(listener);
  return () => {
    authExpiredListeners.delete(listener);
  };
}

function notifyAuthExpired() {
  authExpiredListeners.forEach((listener) => listener());
}

function getRequestPathname(path: string) {
  return new URL(path, "http://localhost").pathname;
}

export function shouldNotifyAuthExpired(path: string, status: number, code?: string) {
  const pathname = getRequestPathname(path);
  if (status !== 401) return false;
  if (["/admin/session", "/admin/login", "/admin/setup"].includes(pathname)) return false;
  if (pathname === "/admin/access-code" && code === "admin.access_code.current_invalid") return false;
  return true;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    credentials: "same-origin",
    headers: init?.body ? { "Content-Type": "application/json", ...init.headers } : init?.headers,
    ...init,
  });
  const text = await response.text();
  const data = text ? JSON.parse(text) : {};
  if (!response.ok) {
    if (shouldNotifyAuthExpired(path, response.status, data.error_code)) {
      notifyAuthExpired();
    }
    throw new APIError(data.error || data.message || `HTTP ${response.status}`, data.error_code, data.error_params as ErrorParams | undefined);
  }
  return data as T;
}

export function getSession() {
  return request<SessionState>("/admin/session");
}

export function setupAdmin(setupCode: string, accessCode: string) {
  return request<{ ok: boolean }>("/admin/setup", {
    method: "POST",
    body: JSON.stringify({ setup_code: setupCode, access_code: accessCode }),
  });
}

export function login(accessCode: string) {
  return request<{ ok: boolean }>("/admin/login", {
    method: "POST",
    body: JSON.stringify({ access_code: accessCode }),
  });
}

export function logout() {
  return request<{ ok: boolean }>("/admin/logout", { method: "POST" });
}

export function updateAdminAccessCode(currentAccessCode: string, newAccessCode: string) {
  return request<{ ok: boolean }>("/admin/access-code", {
    method: "POST",
    body: JSON.stringify({ current_access_code: currentAccessCode, new_access_code: newAccessCode }),
  });
}

export function getMeta() {
  return request<ConfigMeta>("/admin/config/meta");
}

export function getConfig() {
  return request<AdminConfig>("/admin/config");
}

export function validateConfig(config: AdminConfig) {
  return request<{ valid: boolean; error?: string; error_code?: string; error_params?: ErrorParams }>("/admin/config/validate", {
    method: "POST",
    body: JSON.stringify(config),
  });
}

export function testBackend(backend: Backend) {
  return request<BackendTestResult>("/admin/backend/test", {
    method: "POST",
    body: JSON.stringify(backend),
  });
}

export function saveConfig(config: AdminConfig) {
  return request<{ ok: boolean; message?: string }>("/admin/config", {
    method: "PUT",
    body: JSON.stringify(config),
  });
}

export function getLogs(limit = 300, level = "debug") {
  const params = new URLSearchParams({ limit: String(limit), level });
  return request<LogResponse>(`/admin/logs?${params.toString()}`);
}
