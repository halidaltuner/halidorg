// Admin island (DESIGN.md §7): ⌘K focuses search, Esc clears its results.
// Everything else is plain forms upgraded by htmx.
(() => {
	const box = document.querySelector('.adm-search input[name="q"]');
	const results = document.getElementById("adm-search-results");
	document.addEventListener("keydown", (e) => {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k" && box) {
			e.preventDefault();
			box.focus();
			box.select();
		} else if (e.key === "Escape" && results && document.activeElement === box) {
			results.innerHTML = "";
		}
	});
	document.addEventListener("click", (e) => {
		if (results && !e.target.closest(".adm-search")) results.innerHTML = "";
	});
})();

// The Quick Capture bookmarklet is meant to be dragged to the bookmarks bar.
// Clicked here it would capture this admin page, so it explains instead.
for (const a of document.querySelectorAll("[data-bookmarklet]")) {
	a.addEventListener("click", (e) => {
		e.preventDefault();
		const hint = a.parentElement.querySelector(".bm-hint");
		if (hint) {
			hint.textContent = a.dataset.hint;
			hint.classList.add("nudge");
		}
	});
}
