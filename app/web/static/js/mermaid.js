// Mermaid diagrams (Phase 7.2). Pages whose body has ```mermaid blocks load
// this island; it imports the vendored Mermaid build (app/web/static/mermaid,
// one chunk per diagram type) only when a diagram comes near the viewport,
// and draws it with the site's tokens. Without JS, or if a diagram does
// not parse, the block stays as its source.
const blocks = [...document.querySelectorAll("pre.mermaid")];
const src = document.currentScript?.dataset.mermaid;
if (blocks.length && src && "IntersectionObserver" in window) {
	let ready;
	const load = () =>
		(ready ??= import(src).then(({ default: mermaid }) => {
			const css = getComputedStyle(document.documentElement);
			const v = (name) => css.getPropertyValue(name).trim();
			mermaid.initialize({
				startOnLoad: false,
				securityLevel: "strict",
				theme: "base",
				fontFamily: v("--font-sans"),
				themeVariables: {
					fontFamily: v("--font-sans"),
					fontSize: "14px",
					background: v("--surface"),
					primaryColor: v("--tone-cyan-tint"),
					primaryBorderColor: v("--tone-cyan-ink"),
					primaryTextColor: v("--text"),
					secondaryColor: v("--tone-indigo-tint"),
					secondaryBorderColor: v("--tone-indigo-ink"),
					tertiaryColor: v("--surface-2"),
					tertiaryBorderColor: v("--border"),
					lineColor: v("--text-muted"),
					textColor: v("--text"),
					noteBkgColor: v("--tone-amber-tint"),
					noteBorderColor: v("--tone-amber-ink"),
					noteTextColor: v("--text"),
				},
			});
			return mermaid;
		}));
	let n = 0;
	const draw = async (pre) => {
		try {
			const mermaid = await load();
			const { svg } = await mermaid.render("mmd-" + ++n, pre.textContent);
			const fig = document.createElement("figure");
			fig.className = "mermaid-figure";
			fig.innerHTML = svg;
			fig.querySelector("svg")?.setAttribute("role", "img");
			pre.replaceWith(fig);
		} catch {
			pre.classList.add("mermaid-error"); // the source stays readable
		}
	};
	const io = new IntersectionObserver(
		(entries) => {
			for (const e of entries) {
				if (!e.isIntersecting) continue;
				io.unobserve(e.target);
				draw(e.target);
			}
		},
		{ rootMargin: "600px 0px" },
	);
	for (const b of blocks) io.observe(b);
}
