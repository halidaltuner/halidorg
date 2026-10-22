import { describe, expect, it } from "vitest";
import type { JobMessage } from "../src/jobs";
import { check, LIMIT, serveWebmention } from "../src/webmention";

const ORIGIN = "https://halid.org";

function fakeEnv(recent = 0) {
	const sql: { q: string; args: unknown[] }[] = [];
	const sent: JobMessage[] = [];
	const DB = {
		prepare: (q: string) => ({
			bind: (...args: unknown[]) => ({
				run: async () => void sql.push({ q, args }),
				first: async () => (sql.push({ q, args }), { n: recent }),
			}),
		}),
	} as unknown as D1Database;
	const JOBS = { send: async (m: JobMessage) => void sent.push(m) } as unknown as Queue<JobMessage>;
	return { env: { DB, JOBS, PUBLIC_ORIGIN: ORIGIN }, sql, sent };
}

const post = (body: string, ct = "application/x-www-form-urlencoded") =>
	new Request(ORIGIN + "/webmention", { method: "POST", headers: { "Content-Type": ct }, body });

describe("check", () => {
	const f = (o: Record<string, string>) => new URLSearchParams(o);
	it("accepts an outside source and a target on the site", () => {
		expect(check(f({ source: "https://blog.example/p/1", target: ORIGIN + "/en/posts/x" }), ORIGIN)).toEqual({
			source: "https://blog.example/p/1",
			target: ORIGIN + "/en/posts/x",
		});
	});
	it("refuses missing, non-http, foreign targets and self-mentions", () => {
		expect(check(f({ target: ORIGIN + "/en/posts/x" }), ORIGIN)).toMatch(/http/);
		expect(check(f({ source: "javascript:alert(1)", target: ORIGIN + "/en" }), ORIGIN)).toMatch(/http/);
		expect(check(f({ source: "https://a.example/", target: "https://other.example/en" }), ORIGIN)).toMatch(/not on this site/);
		expect(check(f({ source: ORIGIN + "/en/notes/y", target: ORIGIN + "/en/posts/x" }), ORIGIN)).toMatch(/source is on this site/);
	});
});

describe("serveWebmention", () => {
	it("queues a receive job and answers 202", async () => {
		const { env, sql, sent } = fakeEnv();
		const body = new URLSearchParams({ source: "https://blog.example/p/1", target: ORIGIN + "/en/posts/x" }).toString();
		const res = await serveWebmention(post(body), env, 1_760_000_000_000);
		expect(res.status).toBe(202);
		expect(sent).toHaveLength(1);
		expect(sent[0]?.type).toBe("webmention.receive");
		expect(sent[0]?.payload).toEqual({ source: "https://blog.example/p/1", target: ORIGIN + "/en/posts/x" });
		expect(sql.at(-1)?.q).toContain("INSERT INTO job_runs");
	});
	it("refuses GET, other bodies and bad URLs without queueing", async () => {
		const { env, sent } = fakeEnv();
		expect((await serveWebmention(new Request(ORIGIN + "/webmention"), env)).status).toBe(405);
		expect((await serveWebmention(post("{}", "application/json"), env)).status).toBe(415);
		expect((await serveWebmention(post("source=x&target=y"), env)).status).toBe(400);
		expect(sent).toHaveLength(0);
	});
	it("answers 429 once the window is full", async () => {
		const { env, sent } = fakeEnv(LIMIT);
		const body = new URLSearchParams({ source: "https://blog.example/p/1", target: ORIGIN + "/en/posts/x" }).toString();
		const res = await serveWebmention(post(body), env);
		expect(res.status).toBe(429);
		expect(res.headers.get("Retry-After")).toBe("600");
		expect(sent).toHaveLength(0);
	});
});
