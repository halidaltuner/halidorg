import { describe, expect, it } from "vitest";
import { type BridgeEnv, handleBridge, sign, verifySignature } from "../src/bridge";

const body = new TextEncoder().encode("hello").buffer as ArrayBuffer;

describe("bridge signature", () => {
	// Same vector as TestSignVector in app/internal/spike/spike_test.go.
	it("matches the Go signer", async () => {
		expect(await sign("test-secret", "1700000000", "PUT", "/kv/spike/x", body)).toBe(
			"851cfb134e76d62584811f22cc34d0c64518d7d1e291d3e41f1fc0d2e535c9d7",
		);
	});

	it("rejects stale timestamps and tampered paths", async () => {
		const sig = await sign("s", "1000", "PUT", "/kv/spike/x", body);
		const r = new Request("https://x/__bridge/kv/spike/x", { method: "PUT", headers: { "X-Bridge-Ts": "1000", "X-Bridge-Sig": sig } });
		expect(await verifySignature(r, "/kv/spike/x", body, "s", 1030)).toBe(true);
		expect(await verifySignature(r, "/kv/spike/x", body, "s", 1100)).toBe(false);
		expect(await verifySignature(r, "/kv/spike/y", body, "s", 1030)).toBe(false);
		expect(await verifySignature(r, "/kv/spike/x", body, "other", 1030)).toBe(false);
	});
});

// Minimal in-memory stand-ins: only what handleBridge touches.
function fakeEnv() {
	const kv = new Map<string, { value: ArrayBuffer; metadata: unknown }>();
	const sql: { sql: string; params: unknown[] }[] = [];
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
	} as unknown as BridgeEnv;
	return { env, kv, sql };
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

	it("keeps the public route limited to spike probes", async () => {
		const { env } = fakeEnv();
		const spikeOnly = { spikeOnly: true };
		expect((await handleBridge(post({ sql: "DELETE FROM admin_sessions" }), "/d1/query", env, spikeOnly)).status).toBe(403);
		expect((await handleBridge(post({ statements: [] }), "/d1/batch", env, spikeOnly)).status).toBe(404);
		expect((await handleBridge(new Request("http://x/kv/page"), "/kv/page", env, spikeOnly)).status).toBe(403);
		expect((await handleBridge(post({ sql: "SELECT 1" }), "/d1/query", env, spikeOnly)).status).toBe(200);
	});
});
