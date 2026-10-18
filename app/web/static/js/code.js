// Code blocks (internal/render) and the snippet viewer: without this file
// every tab panel shows and the copy/wrap/line buttons stay hidden. With it:
// tabs switch (and move the raw link), copy, wrap and line numbers work.
for (const fig of document.querySelectorAll("figure.code")) {
	for (const b of fig.querySelectorAll("[data-code-copy], [data-code-wrap], [data-code-lines]")) b.hidden = false;
	const tabs = [...fig.querySelectorAll(".code-tab")];
	const panels = [...fig.querySelectorAll(".code-panel")];
	const lang = fig.querySelector(".code-lang");
	const raw = fig.querySelector("[data-code-raw]");
	const show = (i) => {
		tabs.forEach((t, j) => t.setAttribute("aria-selected", String(i === j)));
		panels.forEach((p, j) => (p.hidden = i !== j));
		if (lang && tabs[i]) lang.textContent = tabs[i].dataset.lang || "";
		if (raw && tabs[i]?.dataset.raw) raw.href = tabs[i].dataset.raw;
	};
	if (tabs.length) {
		show(0);
		tabs.forEach((t, i) => t.addEventListener("click", () => show(i)));
	}
	const active = () => fig.querySelector("pre:not([hidden])");
	const copy = fig.querySelector("[data-code-copy]");
	copy?.addEventListener("click", async () => {
		const pre = active();
		if (!pre) return;
		const clone = pre.cloneNode(true);
		for (const n of clone.querySelectorAll(".ln")) n.remove();
		const label = copy.textContent;
		try {
			await navigator.clipboard.writeText(clone.textContent);
			copy.textContent = copy.dataset.copied || "✓";
		} catch {
			const r = document.createRange();
			r.selectNodeContents(pre);
			getSelection()?.removeAllRanges();
			getSelection()?.addRange(r);
		}
		setTimeout(() => (copy.textContent = label), 1400);
	});
	const lines = fig.querySelector("[data-code-lines]");
	lines?.addEventListener("click", () => {
		const on = fig.classList.toggle("nolines");
		lines.setAttribute("aria-pressed", String(!on));
	});
	fig.querySelector("[data-code-wrap]")?.addEventListener("click", () => {
		for (const pre of fig.querySelectorAll("pre")) pre.classList.toggle("wrap");
	});
}
