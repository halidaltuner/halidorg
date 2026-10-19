// Tech radar: hovering or focusing a row lights its blip and the other way
// round; the active row shows its note. Rows are links to the item pages,
// so the list works without JS (every note shown) and from the keyboard;
// ↑ and ↓ move between rows. A blip click opens its item.
const svg = document.querySelector("[data-radar]");
const list = document.querySelector("[data-radar-list]");
if (svg && list) {
	list.classList.add("js");
	const rows = [...list.querySelectorAll(".rdr-row")];
	const blips = [...svg.querySelectorAll(".rdr-blip")];
	const on = (n) => {
		for (const el of [...rows, ...blips]) el.classList.toggle("is-on", n !== "" && el.dataset.n === n);
	};
	for (const el of [...rows, ...blips]) {
		el.addEventListener("mouseenter", () => on(el.dataset.n));
		el.addEventListener("mouseleave", () => on(document.activeElement?.closest?.(".rdr-row")?.dataset.n ?? ""));
	}
	for (const [i, row] of rows.entries()) {
		row.addEventListener("focus", () => on(row.dataset.n));
		row.addEventListener("blur", () => on(""));
		row.addEventListener("keydown", (e) => {
			const next = e.key === "ArrowDown" ? rows[i + 1] : e.key === "ArrowUp" ? rows[i - 1] : null;
			if (next) {
				e.preventDefault();
				next.focus();
			}
		});
	}
	for (const b of blips) b.addEventListener("click", () => (location.href = b.dataset.href));
}
