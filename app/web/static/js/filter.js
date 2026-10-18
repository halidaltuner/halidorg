// List filters (links ?status=, favs ?type=). The chips are links, so
// without JS the server filters. The page in KV holds every item, so with
// JS the chips filter in place and the URL follows with replaceState.
// A page the server already filtered (data-filtered) keeps plain links:
// it does not hold the other items.
for (const root of document.querySelectorAll("[data-filter]")) {
	if (root.dataset.filtered) continue;
	const chips = [...root.querySelectorAll("[data-filter-key]")];
	const param = root.dataset.filter;
	const apply = (key) => {
		chips.forEach((c) => (c.dataset.filterKey === key ? c.setAttribute("aria-current", "true") : c.removeAttribute("aria-current")));
		let shown = 0;
		for (const el of root.querySelectorAll("[data-filter-value]")) {
			el.hidden = key !== "" && el.dataset.filterValue !== key;
			if (!el.hidden) shown++;
		}
		for (const g of root.querySelectorAll("[data-filter-group]")) {
			const n = g.querySelectorAll("[data-filter-value]:not([hidden])").length;
			g.hidden = n === 0;
			const count = g.querySelector("[data-filter-count]");
			if (count) count.textContent = String(n);
		}
		const empty = root.querySelector("[data-filter-empty]");
		if (empty) empty.hidden = shown > 0 || root.querySelectorAll("[data-filter-value]").length === 0;
	};
	let current = new URLSearchParams(location.search).get(param) || "";
	if (!chips.some((c) => c.dataset.filterKey === current)) current = "";
	chips.forEach((c) =>
		c.addEventListener("click", (e) => {
			if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
			e.preventDefault();
			current = c.dataset.filterKey;
			apply(current);
			const u = new URL(location.href);
			current ? u.searchParams.set(param, current) : u.searchParams.delete(param);
			history.replaceState(null, "", u);
		}),
	);
	// links.js re-applies after a read toggle moves a row out of the filter.
	root.addEventListener("filter:refresh", () => apply(current));
}
