// Secrets set with `wrangler secret put` (or .dev.vars locally). Kept out of the
// generated types so a missing secret is a type-level `undefined`, not a lie.
interface Env {
	BRIDGE_SECRET?: string;
	CF_ACCOUNT_ID?: string;
	CF_API_TOKEN?: string;
	CF_D1_ID?: string;
	CF_KV_NAMESPACE_ID?: string;
	GOOGLE_CLIENT_ID?: string;
	GOOGLE_CLIENT_SECRET?: string;
	SESSION_SECRET?: string;
	// Set by CI with `wrangler deploy --var DEPLOY_SHA:<sha>`; absent locally.
	DEPLOY_SHA?: string;
}
