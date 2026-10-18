// Read/unread toggle on /links, for the signed-in admin only. The page is
// the same cached HTML for everyone; admin_hint (auth.HintCookie) says a
// session may exist, and /admin/links/session confirms it before the
// status chips become buttons. Visitors never wake the container.
(async () => {
	if (!document.cookie.split("; ").some((c) => c === "admin_hint=1")) return;
	let res;
	try {
		res = await fetch("/admin/links/session", { credentials: "same-origin", redirect: "manual" });
	} catch {
		return;
	}
	if (res.status !== 204) return;
	const root = document.querySelector("[data-filter]");
	for (const chip of document.querySelectorAll(".lk-status[data-link-id]")) {
		const btn = document.createElement("button");
		btn.type = "button";
		btn.className = chip.className;
		btn.textContent = chip.textContent;
		btn.title = chip.dataset.toggleTitle;
		btn.setAttribute("aria-pressed", String(chip.classList.contains("is-read")));
		chip.replaceWith(btn);
		btn.addEventListener("click", async () => {
			const read = !btn.classList.contains("is-read");
			btn.disabled = true;
			try {
				const r = await fetch(`/admin/links/${chip.dataset.linkId}/read`, {
					method: "POST",
					credentials: "same-origin",
					redirect: "manual",
					body: new URLSearchParams({ status: read ? "read" : "unread" }),
				});
				if (!r.ok) return;
				btn.classList.toggle("is-read", read);
				btn.setAttribute("aria-pressed", String(read));
				btn.textContent = read ? chip.dataset.labelRead : chip.dataset.labelUnread;
				btn.closest("[data-filter-value]").dataset.filterValue = read ? "read" : "unread";
				root?.dispatchEvent(new Event("filter:refresh"));
			} finally {
				btn.disabled = false;
			}
		});
	}
})();
