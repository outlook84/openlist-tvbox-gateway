type Json = Record<string, unknown>;

declare const req:
  | undefined
  | ((url: string, options?: Record<string, unknown>) => { content?: string; code?: number | string });
declare const local:
  | undefined
  | {
      get?: (rule: string, key: string) => string;
      set?: (rule: string, key: string, value: string) => void;
      delete?: (rule: string, key: string) => void;
    };

let gateway = "";
let siteBase = "";
let storageRule = "openlist_tvbox";
let storageScope = "openlist_tvbox";
const sessionTokens: Record<string, StoredToken> = {};

const AUTH_ID = "__openlist_auth__";
const REFRESH_ID = "__refresh__";
const ACCESS_CODE_KEY = "access_code";
const ACCESS_TOKEN_KEY = "access_token";
const DEFAULT_ICON = "/assets/icons/file.png";

type RequestOptions = {
  method?: "get" | "post";
  headers?: Record<string, string>;
  data?: Json;
  withToken?: boolean;
};

type StoredToken = {
  gateway: string;
  token: string;
  expiresAt: number;
};

type StoredCode = {
  gateway: string;
  code: string;
};

function normalizeBaseURL(value: string): string {
  return value.replace(/\/+$/, "");
}

function siteBaseFromGateway(value: string): string {
  return value.replace(/\/s\/[^/]+$/, "");
}

function storageScopeFromGateway(value: string): string {
  const scoped = value.replace(/^https?:\/\//, "").replace(/[^A-Za-z0-9._-]+/g, "_").replace(/^_+|_+$/g, "");
  return scoped ? `openlist_tvbox_${scoped}` : "openlist_tvbox";
}

function gatewayFingerprint(): string {
  return gateway.replace(/^https?:\/\//, "").replace(/[^A-Za-z0-9._-]+/g, "_").replace(/^_+|_+$/g, "") || "gateway";
}

function queryString(params: Record<string, string> = {}): string {
  const parts: string[] = [];
  for (const key of Object.keys(params)) {
    const value = params[key];
    if (value === "") continue;
    parts.push(`${encodeURIComponent(key)}=${encodeURIComponent(value)}`);
  }
  return parts.join("&");
}

function accessTokenKey(): string {
  return `${ACCESS_TOKEN_KEY}:${storageScope}:${gatewayFingerprint()}`;
}

function accessCodeKey(): string {
  return `${ACCESS_CODE_KEY}:${storageScope}:${gatewayFingerprint()}`;
}

function parseStoredCode(value: string): StoredCode | null {
  try {
    const parsed = JSON.parse(value) as Json;
    if (parsed && parsed.gateway === gateway && typeof parsed.code === "string") {
      return { gateway: String(parsed.gateway), code: parsed.code };
    }
  } catch {
    // Legacy plaintext values are intentionally ignored because they are not gateway-bound.
  }
  return null;
}

function encodeStoredCode(code: string): string {
  return JSON.stringify({ gateway, code });
}

function getStoredCode(): string {
  try {
    if (typeof local !== "undefined" && local && typeof local.get === "function") {
      const stored = parseStoredCode(String(local.get(storageRule, accessCodeKey()) || ""));
      return stored ? stored.code : "";
    }
  } catch {
    return "";
  }
  return "";
}

function setStoredCode(code: string): void {
  try {
    if (typeof local !== "undefined" && local && typeof local.set === "function") {
      local.set(storageRule, accessCodeKey(), encodeStoredCode(code));
    }
  } catch {
    // Storage is a convenience, not a correctness dependency.
  }
}

function clearStoredCode(): void {
  try {
    if (typeof local !== "undefined" && local && typeof local.delete === "function") {
      local.delete(storageRule, accessCodeKey());
    }
  } catch {
    // Ignore storage failures.
  }
}

function getSessionToken(): string {
  const session = sessionTokens[accessTokenKey()];
  if (!session || session.gateway !== gateway) return "";
  if (session.expiresAt <= Math.floor(Date.now() / 1000)) {
    clearSessionToken();
    return "";
  }
  return session.token;
}

function setSessionToken(token: string, expiresAt: number): void {
  sessionTokens[accessTokenKey()] = { gateway, token, expiresAt };
}

function clearSessionToken(): void {
  delete sessionTokens[accessTokenKey()];
}

function authenticateWithCode(code: string): boolean {
  if (!code) return false;
  const raw = request("/api/sub/auth", {}, { method: "post", data: { code }, withToken: false });
  const auth = parseResult(raw);
  if (auth.ok === true && typeof auth.access_token === "string" && typeof auth.expires_at === "number") {
    setSessionToken(auth.access_token, auth.expires_at);
    return true;
  }
  clearSessionToken();
  clearStoredCode();
  return false;
}

function request(path: string, params: Record<string, string> = {}, options: RequestOptions = {}): string {
  if (!gateway) throw new Error("openlist-tvbox gateway is not initialized");
  const query = queryString(params);
  const url = `${gateway}${path}${query ? `?${query}` : ""}`;
  if (typeof req === "function") {
    const headers: Record<string, string> = { ...(options.headers || {}) };
    const send = (): string => {
      const response = req(url, {
        method: options.method || "get",
        timeout: 15000,
        async: false,
        headers,
        data: options.data,
        postType: "json",
      });
      if (typeof response === "string") return response;
      if (!response || !response.content) throw new Error("gateway request failed");
      return String(response.content);
    };
    if (options.withToken !== false) {
      let token = getSessionToken();
      if (!token) {
        authenticateWithCode(getStoredCode());
        token = getSessionToken();
      }
      if (token) headers["X-Access-Token"] = token;
    }
    const raw = send();
    if (options.withToken !== false && isUnauthorized(raw)) {
      clearSessionToken();
      delete headers["X-Access-Token"];
      if (authenticateWithCode(getStoredCode())) {
        const token = getSessionToken();
        if (token) {
          headers["X-Access-Token"] = token;
          return send();
        }
      }
    }
    return raw;
  }
  throw new Error("TVBox req helper is not available");
}

function parseResult(raw: string): Json {
  try {
    return JSON.parse(raw) as Json;
  } catch {
    return {};
  }
}

function resolveGatewayURL(path: string): string {
  if (!path.startsWith("/") || path.startsWith("//")) return path;
  return siteBase + path;
}

function absolutizePics(value: Json): Json {
  const list = value.list;
  if (!Array.isArray(list)) return value;
  for (const item of list) {
    if (!item || typeof item !== "object") continue;
    const vod = item as Json;
    const pic = typeof vod.vod_pic === "string" && vod.vod_pic ? vod.vod_pic : DEFAULT_ICON;
    vod.vod_pic = resolveGatewayURL(pic);
  }
  return value;
}

function normalizeResult(raw: string): string {
  return JSON.stringify(absolutizePics(parseResult(raw)));
}

function parseExtend(extend: unknown): Record<string, string> {
  if (!extend) return {};
  if (typeof extend === "string") {
    try {
      return JSON.parse(extend) as Record<string, string>;
    } catch {
      return {};
    }
  }
  return extend as Record<string, string>;
}

function parseJSON(value: string): unknown {
  try {
    return JSON.parse(value);
  } catch {
    return value;
  }
}

function resolveStringInput(value: string | Json, keys: string[], allowRawString: boolean): string {
  let current: unknown = value;
  for (let i = 0; i < 3; i += 1) {
    if (typeof current === "string") {
      const parsed = parseJSON(current);
      if (parsed === current) return allowRawString ? current : "";
      current = parsed;
      continue;
    }
    if (current && typeof current === "object") {
      const obj = current as Json;
      for (const key of keys) {
        const direct = obj[key];
        if (typeof direct === "string") return direct;
      }
      if (obj.ext !== undefined) {
        current = obj.ext;
        continue;
      }
    }
    break;
  }
  return "";
}

function resolveGatewayInput(value: string | Json): string {
  return resolveStringInput(value, ["gateway", "url"], true);
}

function resolveStorageRuleInput(value: string | Json): string {
  return resolveStringInput(value, ["skey"], false);
}

function isUnauthorized(raw: string): boolean {
  return parseResult(raw).error === "unauthorized";
}

function folderVod(id: string, name: string, remarks = ""): Json {
  return {
    vod_id: id,
    vod_name: name,
    vod_pic: resolveGatewayURL("/assets/icons/folder.png"),
    vod_remarks: remarks,
    vod_tag: "folder",
    type_flag: "1",
  };
}

function authResult(input = "", message = ""): string {
  const masked = input ? "*".repeat(input.length) : "未输入";
  const list: Json[] = [
    folderVod(`${AUTH_ID}/noop/${input}`, `访问码：${masked}`, message || "请输入访问码"),
    folderVod(`${AUTH_ID}/0/${input}`, "0"),
    folderVod(`${AUTH_ID}/1/${input}`, "1"),
    folderVod(`${AUTH_ID}/2/${input}`, "2"),
    folderVod(`${AUTH_ID}/3/${input}`, "3"),
    folderVod(`${AUTH_ID}/4/${input}`, "4"),
    folderVod(`${AUTH_ID}/5/${input}`, "5"),
    folderVod(`${AUTH_ID}/6/${input}`, "6"),
    folderVod(`${AUTH_ID}/7/${input}`, "7"),
    folderVod(`${AUTH_ID}/8/${input}`, "8"),
    folderVod(`${AUTH_ID}/9/${input}`, "9"),
    folderVod(`${AUTH_ID}/backspace/${input}`, "退格"),
    folderVod(`${AUTH_ID}/clear/${input}`, "清空"),
    folderVod(`${AUTH_ID}/submit/${input}`, "确认", input ? "验证访问码" : "请先输入"),
  ];
  return JSON.stringify({
    class: [{ type_id: AUTH_ID, type_name: "访问码", type_flag: "1" }],
    list,
    page: 1,
    pagecount: 1,
    limit: list.length,
    total: list.length,
  });
}

function authCategory(tid: string): string {
  const parts = tid.split("/");
  const action = parts[1] || "";
  const input = parts.slice(2).join("/");
  if (/^\d$/.test(action)) return authResult((input + action).slice(0, 12));
  if (action === "backspace") return authResult(input.slice(0, -1));
  if (action === "clear") return authResult("");
  if (action === "noop") return authResult(input);
  if (action !== "submit") return authResult(input);
  if (!input) return authResult("", "请先输入访问码");
  if (authenticateWithCode(input)) {
    setStoredCode(input);
    return authSuccessResult();
  }
  clearSessionToken();
  clearStoredCode();
  return authResult("", "访问码错误");
}

function refreshCategory(tid: string): string {
  const target = tid.slice(`${REFRESH_ID}/`.length);
  if (!target) return normalizeResult(JSON.stringify({ error: "invalid refresh id" }));
  const raw = request("/api/tvbox/refresh", {}, { method: "post", data: { id: target } });
  if (isUnauthorized(raw)) return authResult("");
  if (parseResult(raw).error) return normalizeResult(raw);
  return categoryResponse(target);
}

function authSuccessResult(): string {
  const list = [folderVod(`${AUTH_ID}/noop/`, "验证成功", "请重启 App 后使用")];
  return JSON.stringify({
    list,
    page: 1,
    pagecount: 1,
    limit: list.length,
    total: list.length,
  });
}

function homeResponse(): string {
  const raw = request("/api/tvbox/home");
  if (isUnauthorized(raw)) return authResult("");
  const home = parseResult(raw);
  const classes = Array.isArray(home.class) ? (home.class as Array<{ type_id?: string }>) : [];
  const first = classes.length > 0 ? classes[0].type_id || "" : "";
  if (first) {
    const categoryRaw = request("/api/tvbox/category", { tid: first });
    if (isUnauthorized(categoryRaw)) return authResult("");
    const category = parseResult(categoryRaw);
    if (Array.isArray(category.list)) home.list = category.list;
  }
  return JSON.stringify(absolutizePics(home));
}

function categoryResponse(tid: string, extend?: unknown): string {
  if (tid === AUTH_ID || tid.startsWith(`${AUTH_ID}/`)) return authCategory(tid);
  if (tid.startsWith(`${REFRESH_ID}/`)) return refreshCategory(tid);
  const parsed = parseExtend(extend);
  const raw = request("/api/tvbox/category", {
    tid,
    type: String(parsed.type || ""),
    order: String(parsed.order || ""),
  });
  if (isUnauthorized(raw)) return authResult("");
  return normalizeResult(raw);
}

function detailResponse(id: string | string[]): string {
  const value = Array.isArray(id) ? id[0] : id;
  const raw = request("/api/tvbox/detail", { id: value });
  if (isUnauthorized(raw)) return authResult("");
  return normalizeResult(raw);
}

function requestOrAuth(path: string, params: Record<string, string>): string {
  const raw = request(path, params);
  if (isUnauthorized(raw)) return authResult("");
  return normalizeResult(raw);
}

const spider = {
  init(ext: string | Json) {
    gateway = normalizeBaseURL(resolveGatewayInput(ext));
    siteBase = siteBaseFromGateway(gateway);
    storageScope = resolveStorageRuleInput(ext) || storageScopeFromGateway(gateway);
    storageRule = storageScope;
    clearSessionToken();
  },

  home(_filter?: boolean) {
    return homeResponse();
  },

  homeContent(_filter?: boolean) {
    return homeResponse();
  },

  homeVod() {
    const home = parseResult(homeResponse());
    return JSON.stringify({ list: Array.isArray(home.list) ? home.list : [] });
  },

  category(tid: string, _pg?: string, _filter?: boolean, extend?: unknown) {
    return categoryResponse(tid, extend);
  },

  categoryContent(tid: string, _pg?: string, _filter?: boolean, extend?: unknown) {
    return categoryResponse(tid, extend);
  },

  detail(id: string | string[]) {
    return detailResponse(id);
  },

  detailContent(ids: string[]) {
    return detailResponse(ids);
  },

  search(key: string, _quick?: boolean, _pg?: string) {
    return requestOrAuth("/api/tvbox/search", { key });
  },

  searchContent(key: string, _quick?: boolean) {
    return requestOrAuth("/api/tvbox/search", { key });
  },

  play(_flag: string, id: string, _vipFlags?: string[]) {
    return requestOrAuth("/api/tvbox/play", { id });
  },

  playerContent(_flag: string, id: string, _vipFlags?: string[]) {
    return requestOrAuth("/api/tvbox/play", { id });
  },

  proxy() {
    return "";
  },

  destroy() {},
};

export default spider;

export function __jsEvalReturn() {
  return spider;
}
