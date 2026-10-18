// Queue consumer (Phase 1.3). The container cannot read Queues, so each
// message is POSTed back to it at /internal/jobs. 2xx acks; anything else
// retries with backoff until max_retries in wrangler.jsonc, then the message
// goes to the dead-letter queue. The container records every attempt in
// job_runs.

export interface JobMessage {
	id: string;
	type: string;
	payload: unknown;
}

/** Retry delay in seconds: 15, 30, 60, ... capped at 5 minutes. */
export function backoff(attempts: number): number {
	return Math.min(300, 15 * 2 ** Math.max(0, attempts - 1));
}

export async function consume(
	batch: MessageBatch<JobMessage>,
	send: (req: Request) => Promise<Response>,
	githubToken?: string,
): Promise<void> {
	// One at a time: jobs touch the same KV keys and the container is a
	// single instance (max_instances 1).
	for (const msg of batch.messages) {
		try {
			const res = await send(
				new Request("http://app/internal/jobs", {
					method: "POST",
					headers: {
						"Content-Type": "application/json",
						"X-Halid-Internal": "jobs",
						"X-Job-Attempt": String(msg.attempts),
						// The token rides each delivery, so a secret added or
						// rotated after the Durable Object started still reaches
						// the sync (its env var would lag).
						...(githubToken ? { "X-GitHub-Token": githubToken } : {}),
					},
					body: JSON.stringify(msg.body),
				}),
			);
			await res.body?.cancel();
			if (res.ok) msg.ack();
			else msg.retry({ delaySeconds: backoff(msg.attempts) });
		} catch {
			msg.retry({ delaySeconds: backoff(msg.attempts) });
		}
	}
}

/** Upserts a 404 into not_found_log for the dashboard. */
export function logNotFound(db: D1Database, path: string, now: number): Promise<unknown> {
	return db
		.prepare(
			`INSERT INTO not_found_log (path, hits, last_seen_at) VALUES (?, 1, ?)
ON CONFLICT (path) DO UPDATE SET hits = hits + 1, last_seen_at = excluded.last_seen_at`,
		)
		.bind(path.slice(0, 512), now)
		.run();
}

/** Cron schedules and the job each one enqueues (wrangler.jsonc triggers). */
export const SCHEDULES: Record<string, string> = {
	"17 */6 * * *": "github.sync",
	"7 0 * * *": "now.daily",
};

const CROCKFORD = "0123456789ABCDEFGHJKMNPQRSTVWXYZ";

/** A ULID (time-sortable id), as the container's content.NewID makes. */
export function ulid(now: number, rand: (n: number) => Uint8Array = (n) => crypto.getRandomValues(new Uint8Array(n))): string {
	let t = now;
	let time = "";
	for (let i = 0; i < 10; i++) {
		time = CROCKFORD[t % 32] + time;
		t = Math.floor(t / 32);
	}
	let tail = "";
	for (const b of rand(16)) tail += CROCKFORD[b % 32];
	return time + tail;
}

/**
 * Enqueues the job of a cron schedule like the container's jobs.Enqueue:
 * the job_runs row first, so a failed send still shows as a queued run the
 * dashboard can retry, then the message. Unknown schedules do nothing.
 */
export async function enqueueScheduled(
	cron: string,
	env: { DB: D1Database; JOBS: Queue<JobMessage> },
	now: number,
	id = ulid(now),
): Promise<string | null> {
	const type = SCHEDULES[cron];
	if (!type) return null;
	await env.DB.prepare(`INSERT INTO job_runs (id, type, payload, max_attempts, created_at) VALUES (?, ?, '{}', 4, ?)`)
		.bind(id, type, Math.floor(now / 1000))
		.run();
	try {
		await env.JOBS.send({ id, type, payload: {} });
	} catch (err) {
		await env.DB.prepare(`UPDATE job_runs SET status = 'failed', error = ? WHERE id = ?`)
			.bind("enqueue: " + String(err), id)
			.run();
		throw err;
	}
	return id;
}
