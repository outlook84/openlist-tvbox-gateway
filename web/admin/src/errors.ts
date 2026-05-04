import { APIError, type ErrorParams } from "./types";
import type { T } from "./shared";

export function localizeError(err: unknown, t: T, context: "request" | "config" = "request"): string {
  const message = typeof err === "string" ? err : err instanceof Error ? err.message : "";
  const code = err instanceof APIError ? err.code : undefined;
  const params = err instanceof APIError ? err.params : undefined;
  if (code) {
    return localizeErrorCode(code, params, message, t, context);
  }
  if (!message.trim()) {
    return t("requestFailed");
  }
  if (message.toLowerCase().startsWith("http ")) {
    return `${t("requestFailed")} (${message})`;
  }
  if (context === "config") {
    return `${t("configInvalid")} ${message}`;
  }
  return message;
}

export function localizeErrorCode(code: string, params: ErrorParams | undefined, message: string, t: T, context: "request" | "config") {
  const p = params || {};
  const backend = stringParam(p, "backend_id");
  const sub = stringParam(p, "sub_id");
  const mount = stringParam(p, "mount_id");
  const index = stringParam(p, "index");
  const path = stringParam(p, "path");
  const siteKey = stringParam(p, "site_key");
  const env = stringParam(p, "env");
  const reserved = stringParam(p, "reserved");
  const secret = stringParam(p, "secret");
  switch (code) {
    case "auth.unauthorized":
      return t("errorUnauthorized");
    case "admin.setup_required":
      return t("errorAdminSetupRequired");
    case "admin.already_initialized":
      return t("errorAdminAlreadyInitialized");
    case "auth.too_many_setup_attempts":
      return t("errorTooManySetupAttempts");
    case "auth.too_many_login_attempts":
      return t("errorTooManyLoginAttempts");
    case "request.invalid_json":
      return t("errorInvalidRequest");
    case "admin.setup_failed":
      return t("errorAdminSetupFailed");
    case "admin.session_failed":
      return t("errorAdminSessionFailed");
    case "admin.access_code.update_failed":
      return t("errorAdminAccessCodeUpdateFailed");
    case "admin.access_code.current_invalid":
      return t("errorAdminCurrentAccessCodeInvalid");
    case "config.load_failed":
      return t("errorConfigLoadFailed");
    case "config.save_failed":
      return t("errorConfigSaveFailed");
    case "admin.access_code.invalid":
      return adminAccessCodeReason(message, t);
    case "subscription.access_code.invalid":
      return withScope(t("subscription"), sub, t("errorNumericAccessCode"));
    case "subscription.access_code_hash.invalid":
      return withScope(t("subscription"), sub, t("errorAccessCodeHashInvalid"));
    case "subscription.access_code.plaintext_unsupported":
      return withScope(t("subscription"), sub, t("errorPlaintextAccessCodeUnsupported"));
    case "secret.keep_missing":
      return scopedSecretError(p, t("errorSecretKeepMissing"), t);
    case "secret.invalid_action":
      return scopedSecretError(p, t("errorSecretInvalidAction"), t);
    case "config.public_base_url.invalid":
      return t("errorPublicBaseURLInvalid");
    case "tvbox.site_key.invalid":
      return t("errorSiteKeyInvalid");
    case "backend.required":
      return t("errorBackendRequired");
    case "backend.id.invalid":
      return withScope(t("backend"), index, t("errorIDInvalid"));
    case "backend.id.duplicate":
      return withScope(t("backend"), backend, t("errorBackendIDDuplicate"));
    case "backend.server.invalid":
      return withScope(t("backend"), backend, t("errorBackendServerInvalid"));
    case "backend.version.invalid":
      return withScope(t("backend"), backend, t("errorBackendVersionInvalid"));
    case "backend.env_secret.unsupported":
      return withScope(t("backend"), backend, t("errorEnvSecretUnsupported"));
    case "backend.auth.credentials_for_anonymous":
      return withScope(t("backend"), backend, t("errorAnonymousCredentials"));
    case "backend.auth.api_key_password_conflict":
      return withScope(t("backend"), backend, t("errorAPIKeyPasswordConflict"));
    case "backend.auth.password_api_key_conflict":
      return withScope(t("backend"), backend, t("errorPasswordAPIKeyConflict"));
    case "backend.secret.multiple_sources":
      return withScope(t("backend"), backend, `${secretLabel(secret, t)}: ${t("errorSecretMultipleSources")}`);
    case "backend.env_secret.missing":
      return withScope(t("backend"), backend, `${env}: ${t("errorEnvSecretMissing")}`);
    case "backend.env_secret.empty":
      return withScope(t("backend"), backend, `${env}: ${t("errorEnvSecretEmpty")}`);
    case "backend.auth.api_key_required":
      return withScope(t("backend"), backend, t("errorAPIKeyRequired"));
    case "backend.auth.user_required":
      return withScope(t("backend"), backend, t("errorBackendUserRequired"));
    case "backend.auth.password_required":
      return withScope(t("backend"), backend, t("errorBackendPasswordRequired"));
    case "backend.auth_type.invalid":
      return withScope(t("backend"), backend, t("errorBackendAuthInvalid"));
    case "backend.test_failed":
      return withScope(t("backend"), backend, t("errorBackendTestFailed"));
    case "subscription.required":
      return t("errorSubscriptionRequired");
    case "subscription.id.invalid":
      return withScope(t("subscription"), index, t("errorIDInvalid"));
    case "subscription.id.duplicate":
      return withScope(t("subscription"), sub, t("errorSubscriptionIDDuplicate"));
    case "subscription.path.invalid":
      return withScope(t("subscription"), sub, t("errorPathInvalid"));
    case "subscription.path.reserved":
      return withScope(t("subscription"), sub, `${path}: ${t("errorReservedPath")} ${reserved}`);
    case "subscription.path.duplicate":
      return `${t("path")} ${path}: ${t("errorPathDuplicate")}`;
    case "subscription.site_key.invalid":
      return withScope(t("subscription"), sub, t("errorSiteKeyInvalid"));
    case "subscription.site_key.duplicate":
      return `${t("siteKey")} ${siteKey}: ${t("errorSiteKeyDuplicate")}`;
    case "subscription.mount.required":
      return withScope(t("subscription"), sub, t("errorMountRequired"));
    case "mount.id.invalid":
      return withMountScope(sub, index, t("errorIDInvalid"), t);
    case "mount.id.duplicate":
      return withMountScope(sub, mount, t("errorMountIDDuplicate"), t);
    case "mount.backend.unknown":
      return withMountScope(sub, mount, `${t("backend")} ${backend}: ${t("errorBackendUnknown")}`, t);
    case "mount.path.invalid":
      return withMountScope(sub, mount, t("errorPathInvalid"), t);
    case "mount.params.invalid":
      return withMountScope(sub, mount, t("errorDirectoryPasswordsInvalid"), t);
    case "mount.play_headers.invalid":
      return withMountScope(sub, mount, t("errorPlayHeadersInvalid"), t);
    case "subscription.live.url_required":
    case "subscription.live.url_invalid":
    case "subscription.live.epg_invalid":
    case "subscription.live.logo_invalid":
    case "subscription.live.type_invalid":
      return withScope(t("subscription"), sub, `${t("errorLiveInvalid")} ${index}`);
    case "request.cross_origin":
      return t("errorCrossOrigin");
    default:
      return context === "config" ? `${t("configInvalid")} ${message}` : message || t("requestFailed");
  }
}

export function stringParam(params: ErrorParams, key: string) {
  const value = params[key];
  return value === undefined ? "" : String(value);
}

export function withScope(label: string, id: string, message: string) {
  return id ? `${label} ${id}: ${message}` : message;
}

export function withMountScope(sub: string, mount: string, message: string, t: T) {
  const prefix = sub ? `${t("subscription")} ${sub}` : t("subscription");
  return mount ? `${prefix} / ${t("mount")} ${mount}: ${message}` : `${prefix}: ${message}`;
}

export function scopedSecretError(params: ErrorParams | undefined, message: string, t: T) {
  const p = params || {};
  const backend = stringParam(p, "backend_id");
  const sub = stringParam(p, "sub_id");
  const secret = secretLabel(stringParam(p, "secret"), t);
  if (backend) return `${t("backend")} ${backend} ${secret}: ${message}`;
  if (sub) return `${t("subscription")} ${sub} ${secret}: ${message}`;
  return message;
}

export function secretLabel(secret: string, t: T) {
  if (secret === "api_key") return t("apiKey");
  if (secret === "password") return t("password");
  if (secret === "access_code") return t("accessCode");
  return secret;
}

export function adminAccessCodeReason(message: string, t: T) {
  const normalized = message.toLowerCase();
  if (normalized.includes("8 to 64")) return t("errorAdminAccessCodeLength");
  if (normalized.includes("whitespace") || normalized.includes("control")) return t("errorAdminAccessCodeChars");
  return t("errorAdminAccessCodeInvalid");
}
