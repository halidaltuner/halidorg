// Cover picker (DESIGN.md §39): turns every select[data-cover] into a
// preview with Upload / Choose from library / Remove. Uploading and choosing
// happen on the page, in one dialog: the library grid, or the alt text step
// for an image that has none yet (a cover needs both alt texts). The select
// stays the form field; without JS it is the whole picker.
(() => {
	const selects = [...document.querySelectorAll("select[data-cover]")];
	if (!selects.length) return;
	const src = selects.find((s) => s.dataset.labels) || document.querySelector("[data-cover-labels]");
	let L = {};
	try { L = JSON.parse((src && (src.dataset.labels || src.dataset.coverLabels)) || "{}"); } catch {}
	const t = (k) => L[k] || k;
	const el = (tag, props = {}, ...kids) => {
		const n = Object.assign(document.createElement(tag), props);
		n.append(...kids.filter((k) => k != null));
		return n;
	};

	// The library, loaded once; uploads and alt edits update it in place.
	const byId = new Map();
	let items = [];
	const library = fetch("/admin/media/picker", { credentials: "same-origin", headers: { Accept: "application/json" } })
		.then((r) => (r.ok ? r.json() : []))
		.catch(() => [])
		.then((list) => {
			items = list;
			list.forEach((i) => byId.set(i.id, i));
			return list;
		});
	const keep = (it) => {
		if (!byId.has(it.id)) items.unshift(it);
		else items = items.map((x) => (x.id === it.id ? it : x));
		byId.set(it.id, it);
		return it;
	};

	async function upload(file) {
		const body = new FormData();
		body.append("file", file);
		const r = await fetch("/admin/media", { method: "POST", body, credentials: "same-origin", headers: { "X-Upload": "1", Accept: "application/json" } });
		if (!r.ok) throw new Error((await r.text()).trim() || String(r.status));
		const m = await r.json();
		if (!m.image) throw new Error(m.name);
		return keep({ id: m.id, url: m.url, name: m.name, alt_tr: "", alt_en: "", ready: false });
	}

	async function saveAlt(id, tr, en) {
		const r = await fetch("/admin/media/" + encodeURIComponent(id) + "/alt", {
			method: "POST",
			credentials: "same-origin",
			headers: { Accept: "application/json", "Content-Type": "application/x-www-form-urlencoded" },
			body: new URLSearchParams({ alt_tr: tr, alt_en: en }),
		});
		if (!r.ok) throw new Error((await r.text()).trim() || String(r.status));
		return keep(await r.json());
	}

	// --- The dialog: library grid, or the alt step ---------------------------
	const search = el("input", { type: "search", className: "cv-search", placeholder: t("search") });
	search.setAttribute("aria-label", t("search"));
	const grid = el("div", { className: "cv-grid" });
	const file = el("input", { type: "file", accept: "image/png,image/jpeg,image/gif,image/webp", hidden: true });
	const upBtn = el("button", { type: "button", className: "adm-btn", textContent: t("upload") });
	const closeBtn = el("button", { type: "button", className: "cv-x", textContent: "×", title: t("close") });
	closeBtn.setAttribute("aria-label", t("close"));
	const status = el("p", { className: "cv-status mono", hidden: true });
	status.setAttribute("role", "status");
	const libPane = el("div", { className: "cv-lib" }, el("div", { className: "cv-head" }, el("h2", { textContent: t("title") }), search, upBtn, closeBtn), status, grid);
	const altImg = el("img", { className: "cv-alt-img", alt: "" });
	const altTR = el("input", { lang: "tr", required: true });
	const altEN = el("input", { lang: "en", required: true });
	const altSave = el("button", { type: "submit", className: "adm-btn solid", textContent: t("save") });
	const altCancel = el("button", { type: "button", className: "adm-btn", textContent: t("cancel") });
	const altErr = el("p", { className: "adm-error", hidden: true });
	altErr.setAttribute("role", "alert");
	const altPane = el(
		"form",
		{ className: "cv-alt", hidden: true, method: "dialog" },
		el("h2", { textContent: t("alt_title") }),
		altImg,
		el("p", { className: "cv-hint", textContent: t("alt_hint") }),
		el("label", {}, t("alt_tr"), altTR),
		el("label", {}, t("alt_en"), altEN),
		altErr,
		el("div", { className: "cv-alt-acts" }, altSave, altCancel),
	);
	const dlg = el("dialog", { className: "cv-dlg" }, libPane, altPane, file);
	document.body.append(dlg);

	let onPick = null; // set while the dialog serves one picker
	const say = (msg) => {
		status.textContent = msg || "";
		status.hidden = !msg;
	};
	const close = () => {
		onPick = null;
		if (dlg.open) dlg.close();
	};
	closeBtn.addEventListener("click", close);
	dlg.addEventListener("close", () => { onPick = null; });
	dlg.addEventListener("click", (e) => { if (e.target === dlg) close(); });

	function drawGrid() {
		const q = search.value.trim().toLocaleLowerCase();
		const list = items.filter((i) => !q || [i.name, i.alt_tr, i.alt_en].join(" ").toLocaleLowerCase().includes(q));
		grid.replaceChildren(
			...(list.length
				? list.map((i) => {
						const b = el("button", { type: "button", className: "cv-cell" + (i.ready ? "" : " no-alt") }, el("img", { src: i.url, alt: "", loading: "lazy" }), el("span", { className: "cv-name mono", textContent: i.name || i.id }), i.ready ? null : el("span", { className: "cv-badge mono", textContent: t("missing") }));
						b.title = i.alt_en || i.name || "";
						b.addEventListener("click", () => pick(i));
						return b;
					})
				: [el("p", { className: "cv-empty", textContent: t("empty") })]),
		);
	}
	search.addEventListener("input", drawGrid);

	function showLib() {
		altPane.hidden = true;
		libPane.hidden = false;
		say("");
		drawGrid();
		if (!dlg.open) dlg.showModal();
		search.focus();
	}

	// askAlt shows the alt step for an image and resolves to the saved item,
	// or null when cancelled.
	function askAlt(item) {
		return new Promise((resolve) => {
			libPane.hidden = true;
			altPane.hidden = false;
			altErr.hidden = true;
			altImg.src = item.url;
			altTR.value = item.alt_tr || "";
			altEN.value = item.alt_en || "";
			if (!dlg.open) dlg.showModal();
			altTR.focus();
			const done = (v) => {
				altPane.onsubmit = altCancel.onclick = null;
				resolve(v);
			};
			altCancel.onclick = () => {
				showLib();
				done(null);
			};
			altPane.onsubmit = async (e) => {
				e.preventDefault();
				if (!altTR.value.trim() || !altEN.value.trim()) return;
				altSave.disabled = true;
				try {
					done(await saveAlt(item.id, altTR.value, altEN.value));
				} catch (err) {
					altErr.textContent = t("failed") + err.message;
					altErr.hidden = false;
				} finally {
					altSave.disabled = false;
				}
			};
		});
	}

	async function pick(item) {
		const ready = item.ready ? item : await askAlt(item);
		if (!ready) return;
		const fn = onPick;
		close();
		fn && fn(ready);
	}

	async function fromFile(f) {
		if (!f) return;
		if (!dlg.open) showLib();
		say(t("uploading"));
		try {
			const item = await upload(f);
			say("");
			await pick(item);
		} catch (err) {
			showLib();
			say(t("failed") + err.message);
		}
	}
	upBtn.addEventListener("click", () => file.click());
	file.addEventListener("change", () => {
		fromFile(file.files[0]);
		file.value = "";
	});

	// --- One picker per select ------------------------------------------------
	function enhance(sel) {
		const small = sel.dataset.cover === "small";
		const host = sel.closest(".ed-sec, label, div") || sel.parentElement;
		host.querySelectorAll("[data-cover-nojs]").forEach((n) => n.remove());
		const box = el("button", { type: "button", className: "cv-box" });
		box.style.aspectRatio = sel.dataset.aspect || (small ? "1" : "16/9");
		box.setAttribute("aria-label", t("library"));
		const remove = el("button", { type: "button", className: small ? "cv-x cv-rm" : "adm-btn cv-rm", textContent: small ? "×" : t("remove") });
		remove.setAttribute("aria-label", t("remove"));
		const acts = small
			? null
			: el(
					"div",
					{ className: "cv-acts" },
					el("button", { type: "button", className: "adm-btn", textContent: t("upload"), onclick: () => open(true) }),
					el("button", { type: "button", className: "adm-btn", textContent: t("library"), onclick: () => open(false) }),
					remove,
				);
		const wrap = el("div", { className: "cv" + (small ? " small" : "") }, box, small ? remove : acts);
		sel.hidden = true;
		sel.after(wrap);

		const draw = () => {
			const it = byId.get(sel.value);
			box.replaceChildren(it ? el("img", { src: it.url, alt: it.alt_en || "" }) : small ? el("span", { textContent: "+" }) : el("span", { textContent: sel.dataset.empty || t("drop") }));
			box.classList.toggle("is-empty", !it);
			remove.hidden = !sel.value;
		};
		const choose = (it) => {
			for (const s of selects) {
				if (![...s.options].some((o) => o.value === it.id)) s.append(new Option(it.name || it.id, it.id));
			}
			sel.value = it.id;
			sel.dispatchEvent(new Event("input", { bubbles: true }));
			sel.dispatchEvent(new Event("change", { bubbles: true }));
			draw();
		};
		const open = (uploadFirst) => {
			onPick = choose;
			library.then(() => {
				showLib();
				if (uploadFirst) file.click();
			});
		};
		box.addEventListener("click", () => open(false));
		remove.addEventListener("click", () => {
			sel.value = "";
			sel.dispatchEvent(new Event("input", { bubbles: true }));
			sel.dispatchEvent(new Event("change", { bubbles: true }));
			draw();
		});
		["dragenter", "dragover"].forEach((ev) => box.addEventListener(ev, (e) => { e.preventDefault(); box.classList.add("over"); }));
		["dragleave", "drop"].forEach((ev) => box.addEventListener(ev, (e) => { e.preventDefault(); box.classList.remove("over"); }));
		box.addEventListener("drop", (e) => {
			const f = [...(e.dataTransfer?.files || [])].find((x) => x.type.startsWith("image/"));
			if (!f) return;
			onPick = choose;
			library.then(() => fromFile(f));
		});
		draw();
		library.then(draw);
	}
	selects.forEach(enhance);
})();
