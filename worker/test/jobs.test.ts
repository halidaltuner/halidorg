import { describe, expect, it } from "vitest";
import { backoff, consume, type JobMessage } from "../src/jobs";
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
		});
		expect(log).toEqual(["ack ok", "retry fail 30", "retry throw 60"]);
		expect(seen[0]?.headers.get("X-Halid-Internal")).toBe("jobs");
		expect(seen[1]?.headers.get("X-Job-Attempt")).toBe("2");
		expect(new URL(seen[0]!.url).pathname).toBe("/internal/jobs");
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
