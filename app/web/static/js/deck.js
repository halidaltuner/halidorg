// Deck controls on a presentation page (design/Presentation Detail, ADR-007).
// The deck runs in a sandboxed iframe on the decks origin; the page talks to
// it with postMessage: {deck: "go", slide}, "next", "prev" and "hello", and
// the deck's bridge script answers {deck: "state", slide, total}. Only
// messages from that iframe and origin count. A deck without the bridge
// still shows; the buttons stay disabled and outline rows open the deck.
(() => {
	const box = document.querySelector("[data-deck]");
	const frame = box?.querySelector("iframe");
	if (!box || !frame) return;
	const origin = box.dataset.deckOrigin;
	const prev = box.querySelector("[data-deck-prev]");
	const next = box.querySelector("[data-deck-next]");
	const count = box.querySelector("[data-deck-count]");
	const bar = box.querySelector("[data-deck-progress]");
	let total = Number(box.dataset.total) || 0;
	let slide = 1;
	let live = false;

	const send = (msg) => frame.contentWindow?.postMessage(msg, origin);
	const show = () => {
		if (total > 0) count.textContent = `${slide} / ${total}`;
		bar.style.width = total > 0 ? `${(slide / total) * 100}%` : "0";
		prev.disabled = !live || slide <= 1;
		next.disabled = !live || (total > 0 && slide >= total);
	};

	window.addEventListener("message", (e) => {
		if (e.origin !== origin || e.source !== frame.contentWindow) return;
		const m = e.data;
		if (!m || m.deck !== "state") return;
		const n = Number(m.slide);
		const t = Number(m.total);
		if (!Number.isInteger(n) || n < 1) return;
		if (Number.isInteger(t) && t > 0) total = t;
		slide = Math.min(n, total || n);
		live = true;
		show();
	});
	frame.addEventListener("load", () => send({ deck: "hello" }));
	send({ deck: "hello" });

	prev.addEventListener("click", () => send({ deck: "prev" }));
	next.addEventListener("click", () => send({ deck: "next" }));

	// Outline rows jump the deck to their slide and bring it into view.
	document.addEventListener("click", (e) => {
		const row = e.target.closest("[data-deck-go]");
		if (!row) return;
		const n = Number(row.dataset.deckGo);
		if (!live) {
			window.open(frame.src, "_blank", "noopener");
			return;
		}
		send({ deck: "go", slide: n });
		box.scrollIntoView({ behavior: matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth", block: "center" });
	});

	// Fullscreen takes the frame and its bar; without the API the link
	// opens the deck on its own origin.
	const full = box.querySelector("[data-deck-full]");
	if (box.requestFullscreen) {
		full.addEventListener("click", (e) => {
			e.preventDefault();
			if (document.fullscreenElement) document.exitFullscreen();
			else box.requestFullscreen().then(() => frame.focus()).catch(() => window.open(frame.src, "_blank", "noopener"));
		});
	}
	show();
})();
