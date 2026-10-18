// Photos lightbox (design/Photos.dc.html). Each photo's panel opens with
// its #p-{id} anchor (CSS :target, no JS needed). With JS, closing clears
// the hash without a jump to the top: :target does not follow
// history.replaceState, so a closed panel is marked .is-closed. Esc closes;
// focus goes to the panel and back to its tile.
(() => {
	let opener = null;
	const current = () => document.querySelector(".ph-box:target:not(.is-closed)");
	const sync = () => {
		const box = current();
		document.documentElement.classList.toggle("ph-locked", !!box);
		if (box) box.querySelector(".ph-close")?.focus({ preventScroll: true });
		else if (opener) {
			opener.focus({ preventScroll: true });
			opener = null;
		}
	};
	const close = () => {
		const box = current();
		if (!box) return;
		box.classList.add("is-closed");
		history.replaceState(null, "", location.pathname + location.search);
		sync();
	};
	document.addEventListener("click", (e) => {
		const a = e.target.closest("a");
		if (!a) return;
		if (a.classList.contains("ph-open")) {
			opener = a;
			document.getElementById(a.hash.slice(1))?.classList.remove("is-closed");
			if (a.hash === location.hash) {
				e.preventDefault();
				sync();
			}
		}
		if (a.classList.contains("ph-close") || a.classList.contains("ph-backdrop")) {
			e.preventDefault();
			close();
		}
	});
	document.addEventListener("keydown", (e) => {
		if (e.key === "Escape") close();
	});
	window.addEventListener("hashchange", sync);
	sync();
})();
