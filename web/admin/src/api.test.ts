import { afterEach, describe, expect, it, vi } from "vitest";

async function loadAPI() {
  vi.resetModules();
  return import("./api");
}

function stubFetch(status: number, body: unknown) {
  const fetch = vi.fn(async () => new Response(JSON.stringify(body), { status }));
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("admin api auth expiry handling", () => {
  it("does not treat a wrong current access code as an expired session", async () => {
    const api = await loadAPI();
    const fetch = stubFetch(401, { error: "current access code is incorrect", error_code: "admin.access_code.current_invalid" });
    const listener = vi.fn();
    api.onAuthExpired(listener);

    await expect(api.updateAdminAccessCode("wrong-code", "new-code")).rejects.toMatchObject({
      code: "admin.access_code.current_invalid",
    });

    expect(fetch).toHaveBeenCalledWith(
      "/admin/access-code",
      expect.objectContaining({
        method: "POST",
        credentials: "same-origin",
      }),
    );
    expect(listener).not.toHaveBeenCalled();
  });

  it("matches auth expiry exclusions before query strings", async () => {
    const api = await loadAPI();

    expect(api.shouldNotifyAuthExpired("/admin/access-code?next=/admin/config", 401, "admin.access_code.current_invalid")).toBe(false);
  });

  it("treats access-code 401 responses from expired sessions as expired sessions", async () => {
    const api = await loadAPI();
    stubFetch(401, { error: "unauthorized", error_code: "auth.unauthorized" });
    const listener = vi.fn();
    api.onAuthExpired(listener);

    await expect(api.updateAdminAccessCode("current-code", "new-code")).rejects.toMatchObject({
      code: "auth.unauthorized",
    });

    expect(listener).toHaveBeenCalledTimes(1);
  });

  it("treats protected endpoint 401 responses as expired sessions", async () => {
    const api = await loadAPI();
    stubFetch(401, { error: "unauthorized", error_code: "auth.unauthorized" });
    const listener = vi.fn();
    api.onAuthExpired(listener);

    await expect(api.getConfig()).rejects.toMatchObject({
      name: "APIError",
      code: "auth.unauthorized",
    });

    expect(listener).toHaveBeenCalledTimes(1);
  });
});
