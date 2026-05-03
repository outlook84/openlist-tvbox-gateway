import { afterEach, describe, expect, it, vi } from "vitest";

type ReqCall = {
  url: string;
  options?: Record<string, unknown>;
};

type Spider = {
  init(ext: string | Record<string, unknown>): void;
  home(filter?: boolean): string;
  category(tid: string, pg?: string, filter?: boolean, extend?: unknown): string;
  detail(id: string | string[]): string;
  play(flag: string, id: string, vipFlags?: string[]): string;
};

async function loadSpider(): Promise<Spider> {
  vi.resetModules();
  const mod = await import("./index");
  return mod.default as Spider;
}

function stubReq(handler: (call: ReqCall) => unknown) {
  const req = vi.fn((url: string, options?: Record<string, unknown>) => handler({ url, options }));
  vi.stubGlobal("req", req);
  return req;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("tvbox spider", () => {
  it("loads home content and absolutizes gateway asset paths", async () => {
    const spider = await loadSpider();
    const req = stubReq(({ url }) => {
      if (url === "https://example.test/s/site/api/tvbox/home") {
        return {
          content: JSON.stringify({
            class: [{ type_id: "movies", type_name: "Movies" }],
          }),
        };
      }
      if (url === "https://example.test/s/site/api/tvbox/category?tid=movies") {
        return {
          content: JSON.stringify({
            list: [{ vod_id: "movies/video.mp4", vod_name: "Video", vod_pic: "/assets/icons/video.png" }],
          }),
        };
      }
      throw new Error(`unexpected request: ${url}`);
    });

    spider.init("https://example.test/s/site/");
    const result = JSON.parse(spider.home()) as {
      list: Array<{ vod_pic: string }>;
    };

    expect(result.list[0].vod_pic).toBe("https://example.test/assets/icons/video.png");
    expect(req).toHaveBeenCalledTimes(2);
  });

  it("passes category sorting filters to the gateway", async () => {
    const spider = await loadSpider();
    const req = stubReq(({ url }) => {
      expect(url).toBe("https://gateway.test/api/tvbox/category?tid=mount%2Fpath&type=size&order=desc");
      return { content: JSON.stringify({ list: [] }) };
    });

    spider.init({ gateway: "https://gateway.test" });
    const result = JSON.parse(spider.category("mount/path", "1", true, { type: "size", order: "desc" })) as {
      list: unknown[];
    };

    expect(result.list).toEqual([]);
    expect(req).toHaveBeenCalledTimes(1);
  });

  it("uses stored access code to authenticate once and retry authorized requests with the token", async () => {
    const spider = await loadSpider();
    const stored: Record<string, string> = {};
    const local = {
      get: vi.fn((_rule: string, key: string) => stored[key] || ""),
      set: vi.fn((_: string, key: string, value: string) => {
        stored[key] = value;
      }),
      delete: vi.fn((_: string, key: string) => {
        delete stored[key];
      }),
    };
    vi.stubGlobal("local", local);

    const req = stubReq(({ url, options }) => {
      if (url === "https://gateway.test/api/sub/auth") {
        expect(options?.data).toEqual({ code: "1234" });
        return { content: JSON.stringify({ ok: true, access_token: "token-1", expires_at: 4_102_444_800 }) };
      }
      if (url === "https://gateway.test/api/tvbox/play?id=mount%2Fvideo.mp4") {
        expect((options?.headers as Record<string, string>)["X-Access-Token"]).toBe("token-1");
        return { content: JSON.stringify({ parse: 0, url: "https://media.test/video.mp4" }) };
      }
      throw new Error(`unexpected request: ${url}`);
    });

    spider.init({ gateway: "https://gateway.test" });
    spider.category("__openlist_auth__/submit/1234");
    const result = JSON.parse(spider.play("", "mount/video.mp4")) as { url: string };

    expect(result.url).toBe("https://media.test/video.mp4");
    expect(req).toHaveBeenCalledTimes(2);
  });

  it("returns the access-code UI when the gateway rejects a request as unauthorized", async () => {
    const spider = await loadSpider();
    stubReq(() => ({ content: JSON.stringify({ error: "unauthorized" }) }));

    spider.init("https://gateway.test");
    const result = JSON.parse(spider.detail("mount/video.mp4")) as {
      class: Array<{ type_id: string }>;
      list: Array<{ vod_name: string }>;
    };

    expect(result.class[0].type_id).toBe("__openlist_auth__");
    expect(result.list.map((item) => item.vod_name)).toContain("确认");
  });
});
