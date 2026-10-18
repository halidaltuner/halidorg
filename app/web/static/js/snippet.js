// Snippet viewer (design/Snippets.dc.html): without this file every file
// shows one under the other and the tabs jump to them. With it: one file at
// a time, the language badge and raw link follow the tab, # toggles line
// numbers, copy copies the visible file.
for (const fig of document.querySelectorAll("figure[data-snippet]")) {
	const tabs = [...fig.querySelectorAll(".snip-tab")];
	const panels = [...fig.querySelectorAll(".snip-panel")];
	const lang = fig.querySelector(".snip-lang");
	const raw = fig.querySelector(".snip-raw");
	let cur = 0;
	const show = (i) => {
		cur = i;
		tabs.forEach((t, j) => {
			t.setAttribute("aria-selected", String(i === j));
			t.tabIndex = i === j ? 0 : -1;
		});
		panels.forEach((p, j) => (p.hidden = i !== j));
		const t = tabs[i];
		if (lang) {
			lang.textContent = t.dataset.lang || "";
			lang.hidden = !t.dataset.lang;
		}
		if (raw) raw.href = t.dataset.raw;
	};
	const fromHash = () => Math.max(0, panels.findIndex((p) => "#" + p.id === location.hash));
	show(fromHash());
	tabs.forEach((t, i) => {
		t.addEventListener("click", (e) => {
			e.preventDefault();
			show(i);
			history.replaceState(null, "", "#" + panels[i].id);
		});
		t.addEventListener("keydown", (e) => {
			const d = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
			if (!d) return;
			e.preventDefault();
			const n = (cur + d + tabs.length) % tabs.length;
			show(n);
			tabs[n].focus();
		});
	});

	const lines = fig.querySelector("[data-snippet-lines]");
	if (lines) {
		lines.hidden = false;
		lines.addEventListener("click", () => {
			const on = fig.classList.toggle("no-ln");
			lines.setAttribute("aria-pressed", String(!on));
		});
	}

	const copy = fig.querySelector("[data-snippet-copy]");
	if (copy) {
		copy.hidden = false;
		const label = copy.textContent;
		copy.addEventListener("click", async () => {
			const pre = panels[cur];
			const clone = pre.cloneNode(true);
			for (const n of clone.querySelectorAll(".ln")) n.remove();
			try {
				await navigator.clipboard.writeText(clone.textContent);
				copy.textContent = copy.dataset.copied || label;
			} catch {
				const r = document.createRange();
				r.selectNodeContents(pre);
				getSelection()?.removeAllRanges();
				getSelection()?.addRange(r);
			}
			setTimeout(() => (copy.textContent = label), 1400);
		});
	}
}
