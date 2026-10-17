// Store bridge for the container (ADR-001). The chosen path is method B:
// outbound interception of http://bridge.internal, trusted because only the
// container can produce it, so it gets the full surface. The public, HMAC-signed
// /__bridge route (spike method C) stays limited to the spike probes.

export interface BridgeEnv {
	DB: D1Database;
	PAGES: KVNamespace;
	MEDIA: R2Bucket;
	BRIDGE_SECRET?: string;
}

export interface BridgeOptions {
	/** Restricts the surface to the spike probes. Set for anything reachable from outside. */
	spikeOnly?: boolean;
}

interface Statement {
	sql: string;
	params?: unknown[];
}

const SPIKE_SQL = new Set(["SELECT 1", "SELECT sqlite_version()"]);
const SPIKE_PREFIX = "spike/";
const MAX_SKEW_S = 60;

/** Serves a bridge request whose path is already stripped to `/d1/...`, `/kv/...` or `/r2/...`. */
export async function handleBridge(req: Request, path: string, env: BridgeEnv, opts: BridgeOptions = {}): Promise<Response> {
	const [, store, ...rest] = path.split("/");
	const key = decodeURIComponent(rest.join("/"));

	try {
		if (store === "d1") return await d1(req, rest[0], env, opts);
		if (store !== "kv" && store !== "r2") return text(404, "unknown store");
		if (key === "") return text(400, "missing key");
		if (opts.spikeOnly && !key.startsWith(SPIKE_PREFIX)) return text(403, `key must start with ${SPIKE_PREFIX}`);
		return store === "kv" ? await kv(req, key, env) : await r2(req, key, env);
	} catch (err) {
		// D1 reports constraint and syntax errors as exceptions; the Go side needs the message.
		return text(500, err instanceof Error ? err.message : String(err));
	}
}

async function d1(req: Request, op: string | undefined, env: BridgeEnv, opts: BridgeOptions): Promise<Response> {
	if (req.method !== "POST") return text(405, "method not allowed");
	if (op === "query") {
		const st = await req.json<Statement>();
		if (opts.spikeOnly && !SPIKE_SQL.has(st.sql)) return text(403, "sql not allowed");
		return Response.json(result(await prepare(env, st).all()));
	}
	if (op === "batch" && !opts.spikeOnly) {
		const { statements } = await req.json<{ statements: Statement[] }>();
		const out = await env.DB.batch(statements.map((st) => prepare(env, st)));
		return Response.json(out.map(result));
	}
	return text(404, "unknown d1 op");
}

function prepare(env: BridgeEnv, st: Statement): D1PreparedStatement {
	const stmt = env.DB.prepare(st.sql);
	return st.params?.length ? stmt.bind(...st.params) : stmt;
}

function result(r: D1Result): { results: unknown[]; changes: number; last_row_id: number } {
	return { results: r.results ?? [], changes: r.meta?.changes ?? 0, last_row_id: r.meta?.last_row_id ?? 0 };
}

async function kv(req: Request, key: string, env: BridgeEnv): Promise<Response> {
	switch (req.method) {
		case "GET": {
			const { value, metadata } = await env.PAGES.getWithMetadata(key, "arrayBuffer");
			if (value === null) return text(404, "not found");
			const headers = new Headers();
			if (metadata != null) headers.set("X-KV-Metadata", JSON.stringify(metadata));
			return new Response(value, { headers });
		}
		case "PUT": {
			const meta = req.headers.get("X-KV-Metadata");
			const ttl = Number(req.headers.get("X-KV-TTL") ?? 0);
			await env.PAGES.put(key, await req.arrayBuffer(), {
				metadata: meta ? JSON.parse(meta) : undefined,
				expirationTtl: ttl > 0 ? ttl : undefined,
			});
			return new Response(null, { status: 204 });
		}
		case "DELETE":
			await env.PAGES.delete(key);
			return new Response(null, { status: 204 });
	}
	return text(405, "method not allowed");
}

async function r2(req: Request, key: string, env: BridgeEnv): Promise<Response> {
	switch (req.method) {
		case "GET": {
			const obj = await env.MEDIA.get(key);
			if (obj === null) return text(404, "not found");
			const headers = new Headers({ ETag: obj.httpEtag });
			if (obj.httpMetadata?.contentType) headers.set("Content-Type", obj.httpMetadata.contentType);
			return new Response(obj.body, { headers });
		}
		case "PUT": {
			const contentType = req.headers.get("Content-Type") ?? undefined;
			await env.MEDIA.put(key, await req.arrayBuffer(), { httpMetadata: { contentType } });
			return new Response(null, { status: 204 });
		}
		case "DELETE":
			await env.MEDIA.delete(key);
			return new Response(null, { status: 204 });
	}
	return text(405, "method not allowed");
}

/** Checks the HMAC on a public bridge request. Must match spike.Sign in the Go app. */
export async function verifySignature(req: Request, path: string, body: ArrayBuffer, secret: string, nowS = Date.now() / 1000): Promise<boolean> {
	const ts = req.headers.get("X-Bridge-Ts");
	const sig = req.headers.get("X-Bridge-Sig");
	if (!ts || !sig || Math.abs(nowS - Number(ts)) > MAX_SKEW_S) return false;
	const expected = await sign(secret, ts, req.method, path, body);
	return timingSafeEqual(expected, sig);
}

export async function sign(secret: string, ts: string, method: string, path: string, body: ArrayBuffer): Promise<string> {
	const bodySum = hex(await crypto.subtle.digest("SHA-256", body));
	const key = await crypto.subtle.importKey("raw", new TextEncoder().encode(secret), { name: "HMAC", hash: "SHA-256" }, false, ["sign"]);
	const mac = await crypto.subtle.sign("HMAC", key, new TextEncoder().encode(`${ts}\n${method}\n${path}\n${bodySum}`));
	return hex(mac);
}

function hex(buf: ArrayBuffer): string {
	return [...new Uint8Array(buf)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

function timingSafeEqual(a: string, b: string): boolean {
	if (a.length !== b.length) return false;
	let diff = 0;
	for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
	return diff === 0;
}

function text(status: number, msg: string): Response {
	return new Response(msg, { status });
}
