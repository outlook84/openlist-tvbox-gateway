export type SecretAction = "keep" | "replace" | "clear";

export interface TVBoxSettings {
  site_key?: string;
  site_name?: string;
  timeout?: number;
  searchable?: number;
  quick_search?: number;
  changeable?: number;
}

export interface Backend {
  id: string;
  server: string;
  auth_type: "anonymous" | "api_key" | "password";
  api_key?: string;
  api_key_action?: SecretAction;
  user?: string;
  password?: string;
  password_action?: SecretAction;
  version?: string;
  api_key_set?: boolean;
  password_set?: boolean;
}

export interface Mount {
  id: string;
  name?: string;
  backend: string;
  path: string;
  params?: Record<string, string>;
  play_headers?: Record<string, string>;
  search?: boolean;
  refresh?: boolean;
  hidden?: boolean;
}

export interface Live {
  name: string;
  type?: number;
  url: string;
  playerType?: number;
  epg?: string;
  logo?: string;
  ua?: string;
}

export interface Subscription {
  id: string;
  path?: string;
  access_code_hash?: string;
  access_code_hash_action?: SecretAction;
  access_code_hash_set?: boolean;
  access_code?: string;
  site_key?: string;
  site_name?: string;
  tvbox?: TVBoxSettings;
  lives?: Live[];
  mounts: Mount[];
}

export interface AdminConfig {
  public_base_url?: string;
  trust_forwarded_headers?: boolean;
  trust_x_forwarded_for?: boolean;
  tvbox?: TVBoxSettings;
  backends: Backend[];
  subs: Subscription[];
}

export interface SessionState {
  authenticated: boolean;
  setup_required: boolean;
}

export interface ConfigMeta {
  mode: string;
  format: string;
  editable: boolean;
  path: string;
  message?: string;
}

export type ErrorParams = Record<string, string | number | boolean | undefined>;

export interface BackendTestResult {
  ok: boolean;
  message?: string;
}

export interface LogEntry {
  time: string;
  level: "DEBUG" | "INFO" | "WARN" | "ERROR" | string;
  message: string;
  attrs?: Record<string, unknown>;
}

export interface LogResponse {
  logs: LogEntry[];
}

export class APIError extends Error {
  code?: string;
  params?: ErrorParams;

  constructor(message: string, code?: string, params?: ErrorParams) {
    super(message);
    this.name = "APIError";
    this.code = code;
    this.params = params;
  }
}
