// Loading feedback (DESIGN.md §39). A thin bar runs at the top while the
// next page or a clicked htmx request is on its way (CSS shows it only after
// 150ms, so fast pages never flash it). The clicked submit button spins
// until the answer comes; a save htmx answers shows ✓ for a moment.
(() => {
	const bar = document.createElement("div");
	bar.className = "ld-bar";
	bar.setAttribute("aria-hidden", "true");
	document.body.prepend(bar);
	let running = 0;
	let endTimer;
	const start = () => {
		running++;
		clearTimeout(endTimer);
		bar.className = "ld-bar";
		void bar.offsetWidth; // restart the transition
		bar.className = "ld-bar on";
	};
	const done = () => {
		running = Math.max(0, running - 1);
		if (running) return;
		bar.className = "ld-bar end";
		endTimer = setTimeout(() => { bar.className = "ld-bar"; }, 450);
	};

	let busy = null;
	const clear = () => {
		if (!busy) return;
		busy.classList.remove("btn-busy");
		busy.removeAttribute("aria-busy");
		busy = null;
	};
	const mark = (b) => {
		if (!b || !b.matches(".adm-btn, .ed-btn")) return;
		clear();
		busy = b;
		b.classList.add("btn-busy");
		b.setAttribute("aria-busy", "true");
	};

	// Links to another page of this site.
	document.addEventListener("click", (e) => {
		if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
		const a = e.target.closest("a[href]");
		if (!a || (a.target && a.target !== "_self") || a.hasAttribute("download") || a.hasAttribute("hx-get")) return;
		const u = new URL(a.href, location.href);
		if (u.origin !== location.origin || u.pathname.startsWith("/media/")) return;
		if (u.pathname === location.pathname && u.search === location.search) return; // same page, maybe an anchor
		start();
	});

	// Plain form posts (htmx forms prevent the default and report below).
	document.addEventListener("submit", (e) => {
		if (e.defaultPrevented || e.target.target === "_blank") return;
		mark(e.submitter);
		start();
	});

	// htmx: only requests a click or a submit started; autosave and live
	// search run quietly.
	const clicked = (e) => /^(click|submit)$/.test(e.detail.requestConfig?.triggeringEvent?.type || "");
	document.addEventListener("htmx:beforeRequest", (e) => {
		if (!clicked(e)) return;
		e.detail.elt.dataset.ldRun = "1";
		start();
		mark(e.detail.requestConfig.triggeringEvent.submitter || e.detail.elt);
	});
	document.addEventListener("htmx:afterRequest", (e) => {
		if (!e.detail.elt.dataset.ldRun) return;
		delete e.detail.elt.dataset.ldRun;
		done();
		const b = busy;
		clear();
		if (b && e.detail.successful && b.isConnected) {
			b.classList.add("btn-done");
			setTimeout(() => b.classList.remove("btn-done"), 1200);
		}
	});

	// The back/forward cache brings a page back as it was left: bar running.
	addEventListener("pageshow", (e) => {
		if (!e.persisted) return;
		running = 0;
		bar.className = "ld-bar";
		clear();
	});
})();
