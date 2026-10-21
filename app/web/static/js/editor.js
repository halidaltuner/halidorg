// Editor island (design/Admin Editor.dc.html, DESIGN.md §7, §38). The page
// is a plain form that works without JS; this adds the block editor (Tiptap)
// with a Markdown view (CodeMirror), tag chips, the counters, ⌘S / ⌘⇧P and
// the typed-slug delete check. Autosave, save and preview are htmx
// attributes in the template.
import { mount } from "/static/vendor/cm.js";
import { mountRich } from "/static/vendor/tt.js";

const form = document.getElementById("editor");
const md = document.getElementById("md");
let dirty = false;

if (form && md) {
	const rich = document.getElementById("ed-rich");
	if (rich) modes(form, md, rich);
	else mount(md);
	form.addEventListener("input", () => { dirty = true; });
	// A finished save (autosave or manual) clears the unsaved flag.
	document.body.addEventListener("htmx:afterRequest", (e) => {
		if (e.detail.successful && e.detail.requestConfig && /\/admin\/posts\/[^/]+$/.test(e.detail.requestConfig.path)) dirty = false;
	});
	window.addEventListener("beforeunload", (e) => {
		if (dirty) e.preventDefault();
	});
	form.addEventListener("submit", () => { dirty = false; });

	const words = document.getElementById("ed-words");
	md.addEventListener("input", () => {
		if (words) words.textContent = md.value.split(/\s+/).filter(Boolean).length;
	});

	const seo = document.getElementById("ed-seo");
	const seoN = document.getElementById("ed-seo-n");
	if (seo && seoN) seo.addEventListener("input", () => { seoN.textContent = [...seo.value].length; });

	tags(document.getElementById("ed-tags"), document.getElementById("ed-chips"));
}

document.addEventListener("keydown", (e) => {
	if (!(e.metaKey || e.ctrlKey)) return;
	const k = e.key.toLowerCase();
	if (k === "s" && !e.shiftKey) {
		e.preventDefault();
		document.querySelector("[data-save]")?.click();
	} else if (k === "p" && e.shiftKey) {
		e.preventDefault();
		document.querySelector("[data-publish]")?.click();
	}
});

// Typed-slug delete: the button stays disabled until the slug matches.
const confirmIn = document.querySelector("[data-confirm]");
const del = document.querySelector("[data-needs-confirm]");
if (confirmIn && del) {
	const check = () => { del.disabled = confirmIn.value !== confirmIn.dataset.confirm; };
	confirmIn.addEventListener("input", (e) => { e.stopPropagation(); check(); });
	check();
}
document.querySelectorAll("[data-cancel]").forEach((b) => {
	b.hidden = false;
	b.addEventListener("click", () => {
		const d = b.closest("details");
		if (d) d.open = false;
		if (confirmIn) { confirmIn.value = ""; confirmIn.dispatchEvent(new Event("input")); }
	});
});
document.querySelectorAll('[form="ed-delete"]').forEach((el) => el.addEventListener("input", (e) => e.stopPropagation()));

// Tags: the comma-separated field becomes chips plus an "add tag ↵" input.
// The field stays the form value, so saves and the no-JS post agree.
function tags(field, chips) {
	if (!field || !chips) return;
	const list = () => field.value.split(",").map((t) => t.trim().replace(/^#/, "")).filter(Boolean);
	const set = (arr) => {
		field.value = [...new Set(arr)].join(", ");
		field.dispatchEvent(new Event("input", { bubbles: true }));
		draw();
	};
	const add = document.createElement("input");
	add.placeholder = field.dataset.add || "add tag ↵";
	add.className = "ed-tag-add";
	add.setAttribute("aria-label", add.placeholder);
	add.spellcheck = false;
	function draw() {
		chips.replaceChildren(...list().map((t) => {
			const b = document.createElement("button");
			b.type = "button";
			b.className = "ed-chip";
			b.textContent = "#" + t + " ×";
			b.setAttribute("aria-label", (field.dataset.remove || "remove") + " " + t);
			b.addEventListener("click", () => set(list().filter((x) => x !== t)));
			return b;
		}));
	}
	add.addEventListener("keydown", (e) => {
		if (e.key === "Enter") {
			e.preventDefault();
			const v = add.value.trim().replace(/^#/, "");
			if (v) set([...list(), v]);
			add.value = "";
		}
	});
	add.addEventListener("input", (e) => e.stopPropagation()); // typing a tag is not an edit yet
	field.hidden = true;
	field.after(add);
	chips.hidden = false;
	draw();
}

// modes switches between the block editor and the Markdown source. The
// textarea holds the Markdown in both; the choice is remembered per browser.
// A text the block editor would rewrite stays in Markdown, with a note.
function modes(form, md, rich) {
	const btn = form.querySelector("[data-mode]");
	const note = document.getElementById("ed-rich-note");
	const src = md.parentElement;
	const labels = JSON.parse(rich.dataset.labels || "{}");
	const r = mountRich(md, rich, labels);
	let cm = null;
	const say = (msg) => {
		note.textContent = msg || "";
		note.hidden = !msg;
	};
	const show = (visual) => {
		form.classList.toggle("is-rich", visual);
		rich.hidden = !visual;
		src.hidden = visual;
		btn.setAttribute("aria-pressed", String(!visual));
		btn.textContent = visual ? btn.dataset.markdown : btn.dataset.visual;
		try { localStorage.setItem("ed-mode", visual ? "visual" : "markdown"); } catch {}
	};
	const toMarkdown = () => {
		if (!cm) cm = mount(md);
		else cm.dispatch({ changes: { from: 0, to: cm.state.doc.length, insert: md.value } });
		show(false);
		// The preview sleeps in the visual view; bring it up to date.
		window.htmx?.trigger("#preview", "ed-refresh");
	};
	const toVisual = () => {
		if (!r.load()) {
			say(labels.unfaithful_back);
			return;
		}
		say("");
		show(true);
		r.editor.commands.focus("start");
	};
	let saved = "visual";
	try { saved = localStorage.getItem("ed-mode") || "visual"; } catch {}
	if (!r.faithful) {
		say(labels.unfaithful);
		toMarkdown();
	} else if (saved === "markdown") toMarkdown();
	else show(true);
	btn.hidden = false;
	btn.addEventListener("click", () => (form.classList.contains("is-rich") ? toMarkdown() : toVisual()));
}
