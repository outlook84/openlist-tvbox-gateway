import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

export const jsPath = "internal/gateway/assets/openlist-tvbox.js";

export function spiderFingerprint(path = jsPath) {
  const js = readFileSync(path);
  return createHash("sha256").update(js).digest("hex").slice(0, 12);
}

if (process.argv[1] && fileURLToPath(import.meta.url) === resolve(process.argv[1])) {
  process.stdout.write(spiderFingerprint() + "\n");
}
