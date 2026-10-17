// Secrets set with `wrangler secret put` (or .dev.vars locally). Kept out of the
// generated types so a missing secret is a type-level `undefined`, not a lie.
interface Env {
	GOOGLE_CLIENT_ID?: string;
	GOOGLE_CLIENT_SECRET?: string;
	SESSION_SECRET?: string;
	// Set by CI with `wrangler deploy --var DEPLOY_SHA:<sha>`; absent locally.
	DEPLOY_SHA?: string;
}
