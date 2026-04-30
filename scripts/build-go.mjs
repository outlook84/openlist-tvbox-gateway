import { spawnSync } from "node:child_process";
import { spiderFingerprint } from "./spider-fingerprint.mjs";

const out = process.env.GO_BUILD_OUT || "openlist-tvbox.exe";
const fingerprint = spiderFingerprint();
const variable = "openlist-tvbox/internal/subscription.SpiderFingerprint";

const result = spawnSync(
  "go",
  ["build", "-ldflags", `-X ${variable}=${fingerprint}`, "-o", out, "./cmd/openlist-tvbox"],
  { stdio: "inherit" },
);

if (result.error) throw result.error;
if (result.status !== 0) process.exit(result.status ?? 1);

console.log(`Injected spider fingerprint: ${fingerprint}`);
