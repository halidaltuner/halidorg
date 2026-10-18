// TOC scroll spy (design/Post.dc.html): marks the section being read.
const links = [...document.querySelectorAll(".toc a[href^='#']")];
const targets = links.map((a) => document.getElementById(decodeURIComponent(a.hash.slice(1)))).filter(Boolean);
if (targets.length) {
	let current = null;
	const update = () => {
		let id = targets[0].id;
		for (const t of targets) if (t.getBoundingClientRect().top < 160) id = t.id;
		if (id === current) return;
		current = id;
		for (const a of links) a.setAttribute("aria-current", String(a.hash === "#" + id));
	};
	addEventListener("scroll", update, { passive: true });
	update();
}
