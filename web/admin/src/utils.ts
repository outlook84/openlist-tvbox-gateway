export function uniqueID(prefix: string, existing: string[]) {
  let index = existing.length + 1;
  let id = `${prefix}${index}`;
  while (existing.includes(id)) {
    index += 1;
    id = `${prefix}${index}`;
  }
  return id;
}

export function formatStringMap(values?: Record<string, string>): string {
  if (!values || Object.keys(values).length === 0) return "";
  if (Object.keys(values).length === 1 && Object.prototype.hasOwnProperty.call(values, "")) return values[""] || "";
  return JSON.stringify(values, null, 2);
}

export function parseStringMapDraft(value: string): Record<string, string> | undefined {
  const trimmed = value.trim();
  if (!trimmed) return undefined;
  try {
    const parsed = JSON.parse(trimmed);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return { "": value };
    }
    const values: Record<string, string> = {};
    for (const [key, mapValue] of Object.entries(parsed)) {
      values[key] = typeof mapValue === "string" ? mapValue : String(mapValue);
    }
    return values;
  } catch {
    return { "": value };
  }
}

export function parseOptionalInt(value: string) {
  if (value.trim() === "") {
    return undefined;
  }
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) ? parsed : undefined;
}

export function publicURL(baseURL: string, path: string) {
  if (!path) {
    return "";
  }
  const suffix = path.startsWith("/") ? path : `/${path}`;
  const trimmedBase = baseURL.trim();
  if (!trimmedBase) {
    return `${window.location.origin}${suffix}`;
  }
  if (!isAbsoluteHTTPURL(trimmedBase)) {
    return path;
  }
  const base = trimmedBase.replace(/\/+$/, "");
  return `${base}${suffix}`;
}

export function isAbsoluteHTTPURL(value: string) {
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
}

export async function copyText(text: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  const input = document.createElement("textarea");
  input.value = text;
  input.setAttribute("readonly", "");
  input.style.position = "fixed";
  input.style.left = "-9999px";
  document.body.append(input);
  input.select();
  document.execCommand("copy");
  input.remove();
}
