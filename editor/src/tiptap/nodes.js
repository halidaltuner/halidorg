// Tiptap nodes for the authoring syntax on top of CommonMark + GFM
// (app/internal/render/doc.go). Each one parses its Markdown and writes it
// back unchanged, so the stored Markdown stays the source of truth.
import { Node, mergeAttributes } from "@tiptap/core";
import CodeBlock from "@tiptap/extension-code-block";

// Callouts: GitHub alert syntax, "> [!TIP] optional label" then the body.
const calloutStart = /^ {0,3}> ?\[!(NOTE|TIP|WARNING|DANGER|CAUTION|IMPORTANT)\]/im;

export const Callout = Node.create({
	name: "callout",
	group: "block",
	content: "block+",
	defining: true,
	addAttributes() {
		return {
			kind: { default: "NOTE", parseHTML: (el) => el.getAttribute("data-kind") || "NOTE" },
			label: { default: "", parseHTML: (el) => el.getAttribute("data-label") || "" },
		};
	},
	parseHTML() {
		return [{ tag: "div[data-callout]" }];
	},
	renderHTML({ node, HTMLAttributes }) {
		const kind = node.attrs.kind.toLowerCase();
		return ["div", mergeAttributes(HTMLAttributes, { "data-callout": "", "data-kind": node.attrs.kind, "data-label": node.attrs.label || kind, class: "tt-callout k-" + kind }), 0];
	},
	markdownTokenizer: {
		name: "callout",
		level: "block",
		start(src) {
			const m = src.match(calloutStart);
			return m ? m.index : -1;
		},
		tokenize(src, _tokens, lexer) {
			const head = src.match(/^ {0,3}> ?\[!(NOTE|TIP|WARNING|DANGER|CAUTION|IMPORTANT)\][ \t]*(.*)\n?/i);
			if (!head) return;
			let pos = head[0].length;
			const lines = [];
			for (;;) {
				const rest = src.slice(pos);
				const line = rest.match(/^ {0,3}>(?: ?)(.*)(?:\n|$)/);
				if (!line || rest === "") break;
				lines.push(line[1]);
				pos += line[0].length;
			}
			const body = lines.join("\n");
			return { type: "callout", raw: src.slice(0, pos), kind: head[1], label: head[2].trim(), tokens: lexer.blockTokens(body) };
		},
	},
	parseMarkdown(token, h) {
		const content = h.parseChildren(token.tokens || []);
		return { type: "callout", attrs: { kind: token.kind, label: token.label }, content: content.length ? content : [{ type: "paragraph" }] };
	},
	renderMarkdown(node, h) {
		const head = `> [!${node.attrs.kind}]` + (node.attrs.label ? " " + node.attrs.label : "");
		const body = h.renderChildren(node.content || [], "\n\n");
		return head + "\n" + body.split("\n").map((l) => (l ? "> " + l : ">")).join("\n");
	},
	addCommands() {
		return {
			// setCallout wraps the current block, so the cursor stays in it.
			setCallout:
				(kind) =>
				({ commands }) =>
					commands.wrapIn(this.name, { kind }),
		};
	},
});

// Footnote references ([^1]): an inline atom showing its label.
export const FootnoteRef = Node.create({
	name: "footnoteRef",
	group: "inline",
	inline: true,
	atom: true,
	selectable: true,
	addAttributes() {
		return { label: { default: "1", parseHTML: (el) => el.getAttribute("data-label") } };
	},
	parseHTML() {
		return [{ tag: "sup[data-fnref]" }];
	},
	renderHTML({ node, HTMLAttributes }) {
		return ["sup", mergeAttributes(HTMLAttributes, { "data-fnref": "", "data-label": node.attrs.label, class: "tt-fnref" }), node.attrs.label];
	},
	renderText({ node }) {
		return `[^${node.attrs.label}]`;
	},
	markdownTokenizer: {
		name: "footnoteRef",
		level: "inline",
		start(src) {
			return src.indexOf("[^");
		},
		tokenize(src) {
			const m = src.match(/^\[\^([^\]\s]+)\](?!:)/);
			if (!m) return;
			return { type: "footnoteRef", raw: m[0], label: m[1] };
		},
	},
	parseMarkdown(token) {
		return { type: "footnoteRef", attrs: { label: token.label } };
	},
	renderMarkdown(node) {
		return `[^${node.attrs.label}]`;
	},
});

// Footnote definitions ("[^1]: text", continuation lines indented): a block
// with its label and inline text. The renderer turns them into side notes.
export const Footnote = Node.create({
	name: "footnote",
	group: "block",
	content: "inline*",
	defining: true,
	addAttributes() {
		return { label: { default: "1", parseHTML: (el) => el.getAttribute("data-label") } };
	},
	parseHTML() {
		return [{ tag: "div[data-footnote]" }];
	},
	renderHTML({ node, HTMLAttributes }) {
		return ["div", mergeAttributes(HTMLAttributes, { "data-footnote": "", "data-label": node.attrs.label, class: "tt-footnote" }), 0];
	},
	markdownTokenizer: {
		name: "footnote",
		level: "block",
		start(src) {
			const m = src.match(/^\[\^[^\]\s]+\]:/m);
			return m ? m.index : -1;
		},
		tokenize(src, _tokens, lexer) {
			const m = src.match(/^\[\^([^\]\s]+)\]:[ \t]*(.*(?:\n(?: {2,}|\t)\S.*)*)(?:\n|$)/);
			if (!m) return;
			const text = m[2].split("\n").map((l) => l.trim()).join("\n");
			return { type: "footnote", raw: m[0], label: m[1], tokens: lexer.inlineTokens(text) };
		},
	},
	parseMarkdown(token, h) {
		return { type: "footnote", attrs: { label: token.label }, content: h.parseInline(token.tokens || []) };
	},
	renderMarkdown(node, h) {
		return `[^${node.attrs.label}]: ` + h.renderChildren(node.content || []);
	},
});

// Code blocks keep the whole fence info ("go file=main.go tabs",
// "terminal cwd=~/x", "mermaid") in `language`; a small field above the code
// edits it, and a mermaid block previews its diagram.
export const Code = CodeBlock.extend({
	addOptions() {
		return { ...this.parent?.(), infoLabel: "language file=… tabs" };
	},
	addNodeView() {
		const infoLabel = this.options.infoLabel;
		return ({ node, editor, getPos }) => {
			const dom = document.createElement("div");
			dom.className = "tt-code";
			const bar = document.createElement("div");
			bar.className = "tt-code-bar";
			bar.contentEditable = "false";
			const info = document.createElement("input");
			info.className = "tt-code-info";
			info.value = node.attrs.language || "";
			info.placeholder = infoLabel;
			info.spellcheck = false;
			info.setAttribute("aria-label", info.placeholder);
			info.addEventListener("change", () => {
				const pos = getPos();
				if (typeof pos !== "number") return;
				editor.chain().command(({ tr }) => {
					tr.setNodeMarkup(pos, undefined, { ...node.attrs, language: info.value.trim() || null });
					return true;
				}).run();
			});
			info.addEventListener("keydown", (e) => {
				if (e.key === "Enter") {
					e.preventDefault();
					info.blur();
					editor.commands.focus();
				}
			});
			bar.append(info);
			const pre = document.createElement("pre");
			const code = document.createElement("code");
			pre.append(code);
			const preview = document.createElement("div");
			preview.className = "tt-mermaid";
			preview.contentEditable = "false";
			dom.append(bar, pre, preview);
			let timer;
			const draw = (n) => {
				const isMermaid = /^mermaid\b/.test(n.attrs.language || "");
				dom.classList.toggle("is-mermaid", isMermaid);
				dom.classList.toggle("is-dark", /\b(dark|tabs)\b/.test(n.attrs.language || "") || /^terminal\b/.test(n.attrs.language || ""));
				preview.hidden = !isMermaid;
				if (!isMermaid) return;
				clearTimeout(timer);
				timer = setTimeout(() => renderMermaid(preview, n.textContent), 400);
			};
			draw(node);
			return {
				dom,
				contentDOM: code,
				update(n) {
					if (n.type !== node.type) return false;
					node = n;
					if (document.activeElement !== info) info.value = n.attrs.language || "";
					draw(n);
					return true;
				},
				stopEvent: (e) => e.target === info,
				ignoreMutation: (m) => !code.contains(m.target),
			};
		};
	},
});

let mermaidLib;
let mermaidN = 0;
async function renderMermaid(el, src) {
	if (!src.trim()) {
		el.replaceChildren();
		return;
	}
	try {
		mermaidLib ??= import(document.querySelector("[data-mermaid-src]")?.dataset.mermaidSrc || "/static/mermaid/mermaid.esm.min.mjs").then(({ default: m }) => {
			m.initialize({ startOnLoad: false, securityLevel: "strict", theme: "base" });
			return m;
		});
		const m = await mermaidLib;
		const { svg } = await m.render("tt-mmd-" + ++mermaidN, src);
		el.innerHTML = svg;
		el.classList.remove("is-error");
	} catch {
		el.classList.add("is-error");
	}
}
