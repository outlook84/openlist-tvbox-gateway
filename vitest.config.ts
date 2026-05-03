import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    environment: "node",
    include: ["tvbox/src/**/*.test.ts", "web/admin/src/**/*.test.ts", "web/admin/src/**/*.test.tsx"],
  },
});
