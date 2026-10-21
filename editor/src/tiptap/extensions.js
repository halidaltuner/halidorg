// The schema shared by the admin editor and the roundtrip tests: what the
// renderer understands (app/internal/render/doc.go) and nothing more, so
// every document the editor makes is one the site can show.
import StarterKit from "@tiptap/starter-kit";
import { Markdown } from "@tiptap/markdown";
import { TableKit } from "@tiptap/extension-table";
import Image from "@tiptap/extension-image";
import Link from "@tiptap/extension-link";
import { TaskItem, TaskList } from "@tiptap/extension-list";
import { Callout, Code, FileCard, Footnote, FootnoteRef, Video } from "./nodes.js";

// BareLink writes a link whose text is its own web address as the bare
// address (GFM autolinks it), the way it was typed, not as [url](url).
const BareLink = Link.extend({
	// Marks render around a placeholder; meta.markText is the linked text.
	renderMarkdown(node, h, ctx) {
		const href = node.attrs?.href ?? "";
		const title = node.attrs?.title ?? "";
		const text = h.renderChildren(node);
		if (!title && ctx?.meta?.markText === href && /^https?:\/\/\S+$/.test(href)) return text;
		return title ? `[${text}](${href} "${title}")` : `[${text}](${href})`;
	},
});

export function schema(t = () => "language file=… tabs") {
	return [
		StarterKit.configure({
			codeBlock: false, // Code below keeps the fence info
			underline: false, // Markdown has no underline
			heading: { levels: [1, 2, 3, 4] }, // the title is the h1
			link: false, // BareLink below
		}),
		BareLink.configure({ openOnClick: false, autolink: true, HTMLAttributes: { rel: null, target: null } }),
		Code.configure({ infoLabel: t("code_info") }),
		Callout,
		Footnote,
		FootnoteRef,
		TableKit.configure({ table: { resizable: false } }),
		Image.configure({ inline: false }),
		TaskList,
		TaskItem.configure({ nested: true, a11y: { checkboxLabel: (node) => t("cmd_todo") + ": " + node.textContent } }),
		Video,
		FileCard,
		Markdown.configure({ indentation: { style: "space", size: 2 } }),
	];
}

// norm makes Markdown comparable after a roundtrip: line endings, trailing
// spaces, table cell padding, runs of blank lines and the final newline do
// not count.
export function norm(md) {
	return md
		.replace(/\r\n?/g, "\n")
		.split("\n")
		.map((l) => l.replace(/[ \t]+$/, ""))
		// Table cells are padded to align columns; the padding means nothing.
		.map((l) => (/^\s*\|/.test(l) ? l.replace(/[ \t]*\|[ \t]*/g, "|") : l))
		.join("\n")
		.replace(/\n{3,}/g, "\n\n")
		.trim();
}
