// CodeMirror 6 for the admin editor (DESIGN.md §7). The textarea stays the
// form field: the editor writes every change back to it and fires "input",
// so htmx triggers and the no-JS form post see the same value.
import { EditorView, keymap, drawSelection, highlightActiveLine, placeholder } from "@codemirror/view";
import { EditorState } from "@codemirror/state";
import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { markdown } from "@codemirror/lang-markdown";
import { syntaxHighlighting, HighlightStyle } from "@codemirror/language";
import { tags as t } from "@lezer/highlight";

// Colors come from tokens.css custom properties, so the editor follows the
// design tokens instead of hard-coding them.
const style = HighlightStyle.define([
	{ tag: t.heading, fontWeight: "600", color: "var(--text)" },
	{ tag: t.emphasis, fontStyle: "italic" },
	{ tag: t.strong, fontWeight: "600" },
	{ tag: [t.link, t.url], color: "var(--link)" },
	{ tag: t.monospace, color: "var(--warning)" },
	{ tag: [t.processingInstruction, t.meta, t.contentSeparator], color: "var(--text-muted)" },
	{ tag: t.quote, color: "var(--text-muted)" },
	{ tag: t.list, color: "var(--text-muted)" },
]);

export function mount(textarea) {
	const view = new EditorView({
		parent: textarea.parentElement,
		state: EditorState.create({
			doc: textarea.value,
			extensions: [
				history(),
				drawSelection(),
				highlightActiveLine(),
				EditorView.lineWrapping,
				markdown(),
				syntaxHighlighting(style),
				placeholder(textarea.getAttribute("placeholder") || ""),
				keymap.of([indentWithTab, ...defaultKeymap, ...historyKeymap]),
				EditorView.contentAttributes.of({ "aria-label": textarea.getAttribute("aria-label") || "markdown", spellcheck: "false" }),
				EditorView.updateListener.of((u) => {
					if (!u.docChanged) return;
					textarea.value = u.state.doc.toString();
					textarea.dispatchEvent(new Event("input", { bubbles: true }));
				}),
			],
		}),
	});
	textarea.hidden = true;
	textarea.parentElement.classList.add("has-cm");
	return view;
}
