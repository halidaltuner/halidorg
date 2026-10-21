// Block editor for the short-kind forms (admin/entry_edit.html, DESIGN.md
// §38): the body textarea stays the form field and the Markdown view; the
// visual view (Tiptap) writes Markdown back into it on every change. Without
// JS the textarea is all there is.
import { mountRich } from "/static/vendor/tt.js";

for (const md of document.querySelectorAll("textarea[data-rich]")) {
	const box = md.closest(".ent-body");
	const btn = box.querySelector("[data-mode]");
	const note = box.querySelector(".ent-rich-note");
	const labels = JSON.parse(md.dataset.labels || "{}");
	const host = document.createElement("div");
	host.className = "ent-rich";
	host.lang = md.lang;
	host.dataset.upload = md.dataset.upload || "";
	host.setAttribute("aria-labelledby", "ent-md-label");
	md.after(host);
	const r = mountRich(md, host, labels);
	const say = (msg) => {
		note.textContent = msg || "";
		note.hidden = !msg;
	};
	const show = (visual) => {
		host.hidden = !visual;
		md.hidden = visual;
		btn.setAttribute("aria-pressed", String(!visual));
		btn.textContent = visual ? btn.dataset.markdown : btn.dataset.visual;
	};
	// Always the visual view first: the post editor's remembered choice
	// ("ed-mode") is for long writing, not for these short forms.
	if (!r.faithful) {
		say(labels.unfaithful);
		show(false);
	} else show(true);
	btn.hidden = false;
	btn.addEventListener("click", () => {
		if (md.hidden) return show(false);
		if (!r.load()) return say(labels.unfaithful_back);
		say("");
		show(true);
	});
	// The visual view's label points at the hidden textarea; send it here.
	box.querySelector("label").addEventListener("click", (e) => {
		if (!md.hidden) return;
		e.preventDefault();
		r.editor.commands.focus("end");
	});
}
