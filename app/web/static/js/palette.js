// Command palette (design/Header.dc.html): ⌘K / Ctrl+K or the header button
// opens it, Esc closes it, ↑↓ moves, ↵ opens (with nothing selected, the
// search page with every result). Results come from
// GET /x/search (FTS5 with prefixes, answered by the Worker, ADR-010) as
// JSON; kind labels and prefix hints come from the button's data-* (i18n).
(() => {
	const btn = document.querySelector("[data-palette]");
	if (!btn) return;
	btn.setAttribute("role", "button");
	const lang = btn.dataset.lang || document.documentElement.lang;
	let root, input, list, empty, sel = -1, timer, ctrl, last = "";

	function build() {
		root = document.createElement("div");
		root.className = "pal";
		root.hidden = true;
		root.innerHTML =
			'<div class="pal-panel" role="dialog" aria-modal="true">' +
			'<div class="pal-input"><span class="pal-prompt" aria-hidden="true">&gt;</span>' +
			'<input type="text" autocomplete="off" spellcheck="false" role="combobox" aria-expanded="true" aria-controls="pal-list" aria-autocomplete="list">' +
			'<kbd>esc</kbd></div>' +
			'<div class="pal-list" id="pal-list" role="listbox"></div>' +
			'<p class="pal-empty" hidden></p>' +
			'<div class="pal-hints"></div>' +
			"</div>";
		root.querySelector(".pal-panel").setAttribute("aria-label", btn.dataset.label || "command palette");
		input = root.querySelector("input");
		input.placeholder = btn.dataset.placeholder || "";
		input.setAttribute("aria-label", btn.dataset.label || "search");
		list = root.querySelector(".pal-list");
		empty = root.querySelector(".pal-empty");
		empty.textContent = btn.dataset.empty || "";
		const hints = root.querySelector(".pal-hints");
		for (const h of [...(btn.dataset.hints || "").split("|").filter(Boolean), "↑↓ ↵"]) {
			const span = document.createElement("span");
			span.textContent = h;
			hints.append(span);
		}
		document.body.append(root);
		root.addEventListener("mousedown", (e) => { if (e.target === root) close(); });
		input.addEventListener("input", () => { clearTimeout(timer); timer = setTimeout(query, 120); });
		input.addEventListener("keydown", key);
		list.addEventListener("mousemove", (e) => {
			const row = e.target.closest(".pal-row");
			if (row) select([...list.children].indexOf(row));
		});
	}

	function open() {
		if (!root) build();
		root.hidden = false;
		document.documentElement.classList.add("pal-open");
		input.focus();
		input.select();
	}

	function close() {
		if (!root || root.hidden) return;
		root.hidden = true;
		document.documentElement.classList.remove("pal-open");
		btn.focus();
	}

	async function query() {
		const q = input.value.trim();
		if (q === last) return;
		last = q;
		ctrl?.abort();
		if (!q) { list.replaceChildren(); empty.hidden = true; sel = -1; return; }
		ctrl = new AbortController();
		let rows = [];
		try {
			const res = await fetch("/x/search?lang=" + encodeURIComponent(lang) + "&q=" + encodeURIComponent(q), { signal: ctrl.signal });
			if (res.ok) rows = await res.json();
		} catch (e) {
			if (e.name === "AbortError") return;
		}
		list.replaceChildren(...rows.map(row));
		[...list.children].forEach((row, i) => { row.id = "pal-opt-" + i; row.setAttribute("aria-selected", "false"); });
		empty.hidden = list.children.length > 0;
		select(list.children.length ? 0 : -1);
	}

	// One result: kind dot, title, kind label (with the language when it is
	// not the page's). Text only, never HTML from the response.
	function row(r) {
		const a = document.createElement("a");
		a.className = "pal-row";
		a.href = r.href;
		a.setAttribute("role", "option");
		const dot = document.createElement("span");
		dot.className = "kdot";
		dot.style.setProperty("--k", "var(--kind-" + r.kind + ")");
		const t = document.createElement("span");
		t.className = "t";
		t.textContent = r.title;
		const k = document.createElement("span");
		k.className = "k";
		k.textContent = (btn.dataset["k" + r.kind[0].toUpperCase() + r.kind.slice(1)] || r.kind) + (r.lang !== lang ? " · " + r.lang : "");
		if (r.lang !== lang) a.lang = r.lang;
		a.append(dot, t, k);
		return a;
	}

	function select(i) {
		const rows = [...list.children];
		rows.forEach((r, j) => r.setAttribute("aria-selected", String(j === i)));
		sel = i;
		if (i >= 0) {
			input.setAttribute("aria-activedescendant", rows[i].id);
			rows[i].scrollIntoView({ block: "nearest" });
		} else {
			input.removeAttribute("aria-activedescendant");
		}
	}

	function key(e) {
		const n = list.children.length;
		if (e.key === "ArrowDown" && n) { e.preventDefault(); select((sel + 1) % n); }
		else if (e.key === "ArrowUp" && n) { e.preventDefault(); select((sel - 1 + n) % n); }
		else if (e.key === "Enter" && sel >= 0) { e.preventDefault(); location.href = list.children[sel].href; }
		else if (e.key === "Enter" && input.value.trim()) { e.preventDefault(); location.href = btn.href + "?q=" + encodeURIComponent(input.value.trim()); }
		else if (e.key === "Escape") { e.preventDefault(); close(); }
		else if (e.key === "Tab") { e.preventDefault(); } // focus stays in the palette
	}

	btn.addEventListener("click", (e) => {
		e.preventDefault(); // the link is the no-JS search page
		open();
	});
	document.addEventListener("keydown", (e) => {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
			e.preventDefault();
			root && !root.hidden ? close() : open();
		}
	});
})();
