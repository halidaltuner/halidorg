// Store bridge for the container (ADR-001): answers the container's calls to
// http://bridge.internal with the Worker's bindings. Only reachable through
// outbound interception, never from the internet, so it trusts every request.

export interface BridgeEnv {
	DB: D1Database;
	PAGES: KVNamespace;
	MEDIA: R2Bucket;
}

interface Statement {
	sql: string;
	params?: unknown[];
}

/** Serves a bridge request whose path is already stripped to `/d1/...`, `/kv/...` or `/r2/...`. */
export async function handleBridge(req: Request, path: string, env: BridgeEnv): Promise<Response> {
	const [, store, ...rest] = path.split("/");
	const key = decodeURIComponent(rest.join("/"));

	try {
		if (store === "d1") return await d1(req, rest[0], env);
		if (store !== "kv" && store !== "r2") return text(404, "unknown store");
		if (key === "") return text(400, "missing key");
		return store === "kv" ? await kv(req, key, env) : await r2(req, key, env);
	} catch (err) {
		// D1 reports constraint and syntax errors as exceptions; the Go side needs the message.
		return text(500, err instanceof Error ? err.message : String(err));
	}
}

async function d1(req: Request, op: string | undefined, env: BridgeEnv): Promise<Response> {
	if (req.method !== "POST") return text(405, "method not allowed");
	if (op === "query") {
		const st = await req.json<Statement>();
		return Response.json(result(await prepare(env, st).all()));
	}
	if (op === "batch") {
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

function text(status: number, msg: string): Response {
	return new Response(msg, { status });
}
