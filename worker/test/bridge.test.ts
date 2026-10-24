import { describe, expect, it } from "vitest";
import { type BridgeEnv, handleBridge } from "../src/bridge";

// Minimal in-memory stand-ins: only what handleBridge touches.
function fakeEnv() {
	const kv = new Map<string, { value: ArrayBuffer; metadata: unknown }>();
	const sql: { sql: string; params: unknown[] }[] = [];
	const sent: unknown[] = [];
	const stmt = (q: string, params: unknown[] = []) => ({
		bind: (...p: unknown[]) => stmt(q, p),
		all: async () => (sql.push({ sql: q, params }), { results: [{ ok: 1 }], meta: { changes: 0, last_row_id: 0 } }),
	});
	const env = {
		DB: { prepare: (q: string) => stmt(q), batch: async (s: { all: () => Promise<unknown> }[]) => Promise.all(s.map((x) => x.all())) },
		PAGES: {
			getWithMetadata: async (k: string) => kv.get(k) ?? { value: null, metadata: null },
			put: async (k: string, value: ArrayBuffer, o?: { metadata?: unknown }) => void kv.set(k, { value, metadata: o?.metadata ?? null }),
			delete: async (k: string) => void kv.delete(k),
		},
		MEDIA: {},
		SEARCH: { prepare: (q: string) => stmt("search:" + q), batch: async () => [] },
		JOBS: { send: async (m: unknown) => void sent.push(m) },
	} as unknown as BridgeEnv;
	return { env, kv, sql, sent };
}

const post = (body: unknown) => new Request("http://bridge.internal/x", { method: "POST", body: JSON.stringify(body) });

describe("handleBridge", () => {
	it("binds D1 params and runs batches", async () => {
		const { env, sql } = fakeEnv();
		const res = await handleBridge(post({ sql: "SELECT * FROM t WHERE id = ?", params: ["a"] }), "/d1/query", env);
		expect(await res.json()).toEqual({ results: [{ ok: 1 }], changes: 0, last_row_id: 0 });
		await handleBridge(post({ statements: [{ sql: "DELETE FROM t" }, { sql: "SELECT 1" }] }), "/d1/batch", env);
		expect(sql.map((s) => s.sql)).toEqual(["SELECT * FROM t WHERE id = ?", "DELETE FROM t", "SELECT 1"]);
		expect(sql[0]?.params).toEqual(["a"]);
	});

	it("decodes an escaped key and round-trips KV metadata", async () => {
		const { env, kv } = fakeEnv();
		const path = "/kv/" + encodeURIComponent("page:en:/en/");
		const put = new Request("http://bridge.internal" + path, { method: "PUT", body: "<html>", headers: { "X-KV-Metadata": '{"etag":"x"}' } });
		expect((await handleBridge(put, path, env)).status).toBe(204);
		expect(kv.has("page:en:/en/")).toBe(true);
		const got = await handleBridge(new Request("http://bridge.internal" + path), path, env);
		expect(await got.text()).toBe("<html>");
		expect(got.headers.get("X-KV-Metadata")).toBe('{"etag":"x"}');
	});

	it("routes search to the SEARCH database and sends queue messages", async () => {
		const { env, sql, sent } = fakeEnv();
		await handleBridge(post({ sql: "SELECT 1" }), "/search/query", env);
		expect(sql.at(-1)?.sql).toBe("search:SELECT 1");
		const res = await handleBridge(post({ id: "j1", type: "entry.changed" }), "/queue/send", env);
		expect(res.status).toBe(204);
		expect(sent).toEqual([{ id: "j1", type: "entry.changed" }]);
		expect((await handleBridge(post({}), "/queue/other", env)).status).toBe(404);
	});

	it("writes backups to their own bucket, never to media", async () => {
		const { env } = fakeEnv();
		const put: string[] = [];
		const bucket = (name: string) => ({ put: async (k: string) => void put.push(name + ":" + k) });
		Object.assign(env, { MEDIA: bucket("media"), BACKUPS: bucket("backups") });
		const path = "/backups/" + encodeURIComponent("exports/2026-10-25.zip");
		const res = await handleBridge(new Request("http://bridge.internal" + path, { method: "PUT", body: "zip" }), path, env);
		expect(res.status).toBe(204);
		expect(put).toEqual(["backups:exports/2026-10-25.zip"]);
	});
});
