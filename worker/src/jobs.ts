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
