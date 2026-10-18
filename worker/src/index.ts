// Edge Worker (ADR-001): serves public pages from KV, forwards everything else
// to the Go container, and bridges the container to D1/KV/R2.
import { Container, getContainer } from "@cloudflare/containers";
import { handleBridge } from "./bridge";
import { cacheKey, etagMatches, isCacheableRequest, isCacheableResponse, isStale, type PageMeta } from "./cache";
import { consume, type JobMessage, logNotFound } from "./jobs";
import { serveR2Public } from "./r2public";

// Required by outboundByHost: without it the container fails to start.
export { ContainerProxy } from "@cloudflare/containers";

export class App extends Container<Env> {
	defaultPort = 8080;
	sleepAfter = "5m";

	// Store access (ADR-001, method B): the container calls http://bridge.internal/...
	// and this handler answers it with the Worker's own bindings.
	// Assigned in a static block, not declared as `static outboundByHost = ...`:
	// with ES2022+ class fields that defines an own property and skips
	// Container's setter, so the handler is never registered and the call
	// falls through to the internet as "internal error".
	static {
		this.outboundByHost = {
			"bridge.internal": (req: Request, env: Env) => handleBridge(req, new URL(req.url).pathname, env),
		};
	}

	constructor(ctx: DurableObjectState<{}>, env: Env) {
		super(ctx, env);
		this.envVars = {
			APP_ENV: env.APP_ENV,
			PUBLIC_ORIGIN: env.PUBLIC_ORIGIN,
			DEPLOY_SHA: env.DEPLOY_SHA ?? "",
			GOOGLE_CLIENT_ID: env.GOOGLE_CLIENT_ID ?? "",
			GOOGLE_CLIENT_SECRET: env.GOOGLE_CLIENT_SECRET ?? "",
			SESSION_SECRET: env.SESSION_SECRET ?? "",
		};
	}
}

export default {
	async fetch(req, env, ctx): Promise<Response> {
		const url = new URL(req.url);
		// Container-internal routes (job delivery) never come from the internet.
		if (url.pathname === "/internal" || url.pathname.startsWith("/internal/")) {
			return new Response("Not Found", { status: 404 });
		}
		const obj = await serveR2Public(req, url, env);
		if (obj) return obj;
		if (!isCacheableRequest(req)) return origin(req, env);

		const key = cacheKey(url);
		const t0 = Date.now();
		const hit = await env.PAGES.getWithMetadata<PageMeta>(key, "arrayBuffer");
		const kvMs = Date.now() - t0;

		if (hit.value !== null && hit.metadata) {
			if (isStale(hit.metadata, env.DEPLOY_SHA)) {
				// Stale-while-revalidate: the visitor gets the stored page now,
				// the current templates render it in the background.
				ctx.waitUntil(refresh(req, key, env).catch(() => {}));
				return page(req, hit.value, hit.metadata, "STALE", `kv;dur=${kvMs}`);
			}
			return page(req, hit.value, hit.metadata, "HIT", `kv;dur=${kvMs}`);
		}

		const t1 = Date.now();
		const res = await origin(forStore(req), env);
		const timing = `kv;dur=${kvMs}, origin;dur=${Date.now() - t1}`;

		if (res.status === 404) {
			ctx.waitUntil(logNotFound(env.DB, url.pathname, Math.floor(Date.now() / 1000)).catch(() => {}));
		}

		if (!isCacheableResponse(res)) {
			const out = new Response(res.body, res);
			out.headers.set("X-Cache", "BYPASS");
			out.headers.set("Server-Timing", timing);
			return out;
		}

		const body = await res.arrayBuffer();
		const meta = metaOf(res, env);
		ctx.waitUntil(env.PAGES.put(key, body, { metadata: meta }));
		return page(req, body, meta, "MISS", timing);
	},

	async queue(batch, env): Promise<void> {
		await consume(batch, (req) => origin(req, env));
	},
} satisfies ExportedHandler<Env, JobMessage>;

// The container learns the current deploy from every request: its own
// DEPLOY_SHA env var is fixed when its Durable Object starts and lags behind
// deploys that do not restart it. Any client-sent value is overwritten.
function origin(req: Request, env: Env): Promise<Response> {
	const fwd = new Request(req);
	if (env.DEPLOY_SHA) fwd.headers.set("X-Deploy-Sha", env.DEPLOY_SHA);
	else fwd.headers.delete("X-Deploy-Sha");
	return getContainer(env.APP).fetch(fwd);
}

// Strip validators so the container returns a full 200 we can store.
function forStore(req: Request): Request {
	const fwd = new Request(req);
	fwd.headers.delete("If-None-Match");
	fwd.headers.delete("If-Modified-Since");
	return fwd;
}

function metaOf(res: Response, env: Env): PageMeta {
	return {
		etag: res.headers.get("ETag") ?? "",
		contentType: res.headers.get("Content-Type") ?? "text/html; charset=utf-8",
		storedAt: Math.floor(Date.now() / 1000),
		deploy: env.DEPLOY_SHA,
	};
}

/**
 * Re-renders a stale page with the current deploy. A page that is gone
 * (404, 410, or a redirect now) leaves KV; a failing container leaves the
 * stale copy in place for the next try.
 */
export async function refresh(req: Request, key: string, env: Env): Promise<void> {
	const res = await origin(forStore(req), env);
	if (isCacheableResponse(res)) {
		await env.PAGES.put(key, await res.arrayBuffer(), { metadata: metaOf(res, env) });
	} else if (res.status === 404 || res.status === 410 || (res.status >= 300 && res.status < 400)) {
		await env.PAGES.delete(key);
	}
}

function page(req: Request, body: ArrayBuffer, meta: PageMeta, cache: "HIT" | "MISS" | "STALE", timing: string): Response {
	const headers = new Headers({
		"Content-Type": meta.contentType,
		"Cache-Control": "public, max-age=0, must-revalidate",
		"X-Cache": cache,
		"Server-Timing": timing,
	});
	if (meta.etag) headers.set("ETag", meta.etag);
	if (meta.etag && etagMatches(req.headers.get("If-None-Match"), meta.etag)) {
		return new Response(null, { status: 304, headers });
	}
	return new Response(req.method === "HEAD" ? null : body, { headers });
}
