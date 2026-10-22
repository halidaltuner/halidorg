// Webmention receiving endpoint (Phase 8.1, W3C Webmention §3.2). The edge
// checks the request and queues a webmention.receive job; the container
// fetches and verifies the source later (internal/webmention). Receiving
// never needs a warm container, and a flood stops here.

import { enqueue, type JobMessage } from "./jobs";

export const PATH = "/webmention";
export const TYPE = "webmention.receive";

/** At most this many receive jobs are queued per window, for all senders. */
export const LIMIT = 60;
export const WINDOW_S = 600;

export interface WebmentionEnv {
	DB: D1Database;
	JOBS: Queue<JobMessage>;
	PUBLIC_ORIGIN: string;
}

const text = (body: string, status: number, headers: Record<string, string> = {}) =>
	new Response(body + "\n", { status, headers: { "Content-Type": "text/plain; charset=utf-8", "Cache-Control": "no-store", ...headers } });

function httpURL(raw: string | null): URL | null {
	if (!raw || raw.length > 2048) return null;
	try {
		const u = new URL(raw);
		return u.protocol === "http:" || u.protocol === "https:" ? u : null;
	} catch {
		return null;
	}
}

/** Checks a webmention request; returns the error message, or the source and target. */
export function check(form: { get(name: string): unknown }, origin: string): string | { source: string; target: string } {
	const s = form.get("source");
	const t = form.get("target");
	const source = httpURL(typeof s === "string" ? s : null);
	const target = httpURL(typeof t === "string" ? t : null);
	if (!source || !target) return "source and target must be http(s) URLs";
	const site = new URL(origin);
	if (target.host !== site.host) return "target is not on this site";
	if (source.host === site.host) return "source is on this site";
	if (source.href === target.href) return "source and target are the same";
	return { source: source.href, target: target.href };
}

/** Answers /webmention: 202 once queued, 400 for a bad request, 429 when busy. */
export async function serveWebmention(req: Request, env: WebmentionEnv, now = Date.now()): Promise<Response> {
	if (req.method !== "POST") return text("Method Not Allowed", 405, { Allow: "POST" });
	const ct = req.headers.get("Content-Type") ?? "";
	if (!ct.toLowerCase().startsWith("application/x-www-form-urlencoded")) return text("expected application/x-www-form-urlencoded", 415);
	const raw = await req.text();
	if (raw.length > 8192) return text("request too large", 413);
	const got = check(new URLSearchParams(raw), env.PUBLIC_ORIGIN);
	if (typeof got === "string") return text(got, 400);
	const recent = await env.DB.prepare(`SELECT COUNT(*) AS n FROM job_runs WHERE type = ? AND created_at >= ?`)
		.bind(TYPE, Math.floor(now / 1000) - WINDOW_S)
		.first<{ n: number }>();
	if ((recent?.n ?? 0) >= LIMIT) return text("too many webmentions, try again later", 429, { "Retry-After": String(WINDOW_S) });
	await enqueue(env, TYPE, got, now);
	return text("accepted: the source will be verified shortly", 202);
}
