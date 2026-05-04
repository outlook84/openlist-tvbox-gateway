import type { AdminConfig, Backend } from "./types";
import type { EditorProps } from "./shared";

export const emptyConfig: AdminConfig = { backends: [], subs: [], tvbox: {} };

export function normalizeConfig(config: AdminConfig): AdminConfig {
  return {
    ...emptyConfig,
    ...config,
    tvbox: config.tvbox || {},
    backends: (config.backends || []).map(normalizeBackend),
    subs: (config.subs || []).map((sub) => ({ ...sub, lives: sub.lives || [], mounts: sub.mounts || [], access_code: "", access_code_hash_action: sub.access_code_hash_action || "keep" })),
  };
}

export function normalizeBackend(backend: Backend): Backend {
  const next = { ...backend, auth_type: backend.auth_type || "anonymous", version: backend.version || "v3" };
  if (next.auth_type !== "api_key") {
    next.api_key = "";
    next.api_key_action = "clear";
  } else {
    next.api_key_action = next.api_key_action || "keep";
  }
  if (next.auth_type !== "password") {
    next.user = "";
    next.password = "";
    next.password_action = "clear";
  } else {
    next.password_action = next.password_action || "keep";
  }
  return next;
}

export function updateConfig(setConfig: EditorProps["setConfig"], patch: Partial<AdminConfig>) {
  setConfig((current) => ({ ...current, ...patch }));
}

export function updateTVBox(setConfig: EditorProps["setConfig"], patch: Partial<AdminConfig["tvbox"]>) {
  setConfig((current) => ({ ...current, tvbox: { ...(current.tvbox || {}), ...patch } }));
}
