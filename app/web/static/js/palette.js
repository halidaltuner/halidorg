// Command palette (design/Header.dc.html): ⌘K / Ctrl+K or the header button
// opens it, Esc closes it, ↑↓ moves, ↵ opens. Results come from
// GET /x/search (FTS5 with prefixes, ADR-010) as HTML rows.
(() => {
	const btn = document.querySelector("[data-palette]");
	if (!btn) return;
	btn.hidden = false;
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
			'<div class="pal-hints"><span>p: posts</span><span>n: notes</span><span>s: snippets</span><span>l: links</span><span>pr: projects</span><span>h: homelab</span><span>↑↓ ↵</span></div>' +
			"</div>";
		root.querySelector(".pal-panel").setAttribute("aria-label", btn.dataset.label || "command palette");
		input = root.querySelector("input");
		input.placeholder = btn.dataset.placeholder || "";
		input.setAttribute("aria-label", btn.dataset.label || "search");
		list = root.querySelector(".pal-list");
		empty = root.querySelector(".pal-empty");
		empty.textContent = btn.dataset.empty || "";
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
		if (!q) { list.innerHTML = ""; empty.hidden = true; sel = -1; return; }
		ctrl = new AbortController();
		try {
			const res = await fetch("/x/search?lang=" + encodeURIComponent(lang) + "&q=" + encodeURIComponent(q), { signal: ctrl.signal });
			list.innerHTML = res.ok ? await res.text() : "";
		} catch (e) {
			if (e.name === "AbortError") return;
			list.innerHTML = "";
		}
		[...list.children].forEach((row, i) => { row.id = "pal-opt-" + i; row.setAttribute("aria-selected", "false"); });
		empty.hidden = list.children.length > 0;
		select(list.children.length ? 0 : -1);
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
		else if (e.key === "Escape") { e.preventDefault(); close(); }
		else if (e.key === "Tab") { e.preventDefault(); } // focus stays in the palette
	}

	btn.addEventListener("click", open);
	document.addEventListener("keydown", (e) => {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
			e.preventDefault();
			root && !root.hidden ? close() : open();
		}
	});
})();
