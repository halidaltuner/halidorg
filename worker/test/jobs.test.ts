import { describe, expect, it } from "vitest";
import { backoff, consume, enqueueScheduled, type JobMessage, ulid } from "../src/jobs";
import { isCacheableResponse } from "../src/cache";

function batch(...bodies: JobMessage[]) {
	const log: string[] = [];
	const messages = bodies.map((body, i) => ({
		id: String(i),
		body,
		attempts: i + 1,
		timestamp: new Date(),
		ack: () => void log.push(`ack ${body.id}`),
		retry: (o?: { delaySeconds?: number }) => void log.push(`retry ${body.id} ${o?.delaySeconds}`),
	}));
	return { batch: { queue: "q", messages } as unknown as MessageBatch<JobMessage>, log };
}

describe("consume", () => {
	it("acks 2xx, retries errors and thrown sends with backoff", async () => {
		const { batch: b, log } = batch(
			{ id: "ok", type: "t", payload: {} },
			{ id: "fail", type: "t", payload: {} },
			{ id: "throw", type: "t", payload: {} },
		);
		const seen: Request[] = [];
		await consume(b, async (req) => {
			seen.push(req);
			const body = (await req.clone().json()) as JobMessage;
			if (body.id === "throw") throw new Error("container down");
			return new Response(null, { status: body.id === "ok" ? 200 : 500 });
		}, "tok");
		expect(log).toEqual(["ack ok", "retry fail 30", "retry throw 60"]);
		expect(seen[0]?.headers.get("X-GitHub-Token")).toBe("tok");
		expect(seen[0]?.headers.get("X-Halid-Internal")).toBe("jobs");
		expect(seen[1]?.headers.get("X-Job-Attempt")).toBe("2");
		expect(new URL(seen[0]!.url).pathname).toBe("/internal/jobs");
	});

	it("sends no token header without the secret", async () => {
		const { batch: b } = batch({ id: "ok", type: "t", payload: {} });
		let got: string | null = "unset";
		await consume(b, async (req) => {
			got = req.headers.get("X-GitHub-Token");
			return new Response(null);
		});
		expect(got).toBeNull();
	});

	it("caps backoff at five minutes", () => {
		expect([1, 2, 3, 10].map(backoff)).toEqual([15, 30, 60, 300]);
	});
});

describe("feed caching", () => {
	it("stores public feeds, sitemap and robots, not other types", () => {
		const r = (ct: string) => new Response("x", { headers: { "Content-Type": ct, "Cache-Control": "public, max-age=0" } });
		for (const ct of ["application/rss+xml; charset=utf-8", "application/atom+xml", "application/feed+json", "application/xml", "text/plain; charset=utf-8"]) {
			expect(isCacheableResponse(r(ct)), ct).toBe(true);
		}
		expect(isCacheableResponse(r("text/css"))).toBe(false);
		expect(isCacheableResponse(r("application/json"))).toBe(false);
	});
});

describe("enqueueScheduled", () => {
	function fakeEnv(sendFails = false) {
		const sql: { q: string; args: unknown[] }[] = [];
		const sent: JobMessage[] = [];
		const DB = {
			prepare: (q: string) => ({
				bind: (...args: unknown[]) => ({ run: async () => void sql.push({ q, args }) }),
			}),
		} as unknown as D1Database;
		const JOBS = {
			send: async (m: JobMessage) => {
				if (sendFails) throw new Error("queue down");
				sent.push(m);
			},
		} as unknown as Queue<JobMessage>;
		return { env: { DB, JOBS }, sql, sent };
	}

	it("records the run, then sends the job of the schedule", async () => {
		const { env, sql, sent } = fakeEnv();
		const id = await enqueueScheduled("17 */6 * * *", env, 1_760_000_000_000, "01TESTID");
		expect(id).toBe("01TESTID");
		expect(sql).toHaveLength(1);
		expect(sql[0]?.q).toContain("INSERT INTO job_runs");
		expect(sql[0]?.args).toEqual(["01TESTID", "github.sync", 1_760_000_000]);
		expect(sent).toEqual([{ id: "01TESTID", type: "github.sync", payload: {} }]);
	});

	it("marks the run failed when the send fails, and ignores unknown schedules", async () => {
		const { env, sql } = fakeEnv(true);
		await expect(enqueueScheduled("17 */6 * * *", env, 0, "X")).rejects.toThrow("queue down");
		expect(sql[1]?.q).toContain("status = 'failed'");
		expect(await enqueueScheduled("0 0 * * *", fakeEnv().env, 0)).toBeNull();
	});
});

describe("ulid", () => {
	it("is 26 Crockford characters, sorted by time", () => {
		const a = ulid(1_760_000_000_000, () => new Uint8Array(16));
		const b = ulid(1_760_000_000_001, () => new Uint8Array(16));
		expect(a).toMatch(/^[0-9A-HJKMNP-TV-Z]{26}$/);
		expect(a < b).toBe(true);
	});
});
