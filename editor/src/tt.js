// The admin's block editor (Tiptap, Phase 7.6): Notion-like editing over
// Markdown. The textarea stays the form field and the Markdown stays the
// stored text: the editor parses it, and every change writes Markdown back
// and fires "input", so autosave, the word count and the no-JS post agree.
//
// A document the editor cannot write back unchanged (Markdown it does not
// model, say raw HTML) opens in Markdown instead, with a note: switching
// views never rewrites text behind the author's back.
import { Editor } from "@tiptap/core";
import { norm, schema } from "./tiptap/extensions.js";
import { ui } from "./tiptap/ui.js";

export { norm };

export function mountRich(textarea, host, labels = {}) {
	const t = (k) => labels[k] || k;
	const layer = ui(t);
	const original = textarea.value;
	const editor = new Editor({
		element: host,
		extensions: [...schema(t), ...layer.extensions],
		content: original,
		contentType: "markdown",
		editorProps: { attributes: { class: "prose tt-prose", "aria-label": textarea.getAttribute("aria-label") || "text", spellcheck: "true" } },
	});
	layer.bind(editor);
	const faithful = norm(editor.getMarkdown()) === norm(original);
	let syncing = false;
	editor.on("update", () => {
		if (syncing) return;
		textarea.value = editor.getMarkdown();
		textarea.dispatchEvent(new Event("input", { bubbles: true }));
	});
	return {
		editor,
		faithful,
		// load replaces the document with the textarea's Markdown (after
		// editing the source) without counting as an edit.
		load() {
			syncing = true;
			editor.commands.setContent(textarea.value, { contentType: "markdown" });
			syncing = false;
			return norm(editor.getMarkdown()) === norm(textarea.value);
		},
	};
}
