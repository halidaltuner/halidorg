// Tiptap nodes for the authoring syntax on top of CommonMark + GFM
// (app/internal/render/doc.go). Each one parses its Markdown and writes it
// back unchanged, so the stored Markdown stays the source of truth.
import { Node, mergeAttributes } from "@tiptap/core";
import CodeBlock from "@tiptap/extension-code-block";
import { Plugin, PluginKey } from "@tiptap/pm/state";
import { Decoration, DecorationSet } from "@tiptap/pm/view";
import { common, createLowlight } from "lowlight";
import dockerfile from "highlight.js/lib/languages/dockerfile";
import nginx from "highlight.js/lib/languages/nginx";
import powershell from "highlight.js/lib/languages/powershell";

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
	addProseMirrorPlugins() {
		return [...(this.parent?.() || []), highlighter(this.name)];
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
			info.setAttribute("list", langList());
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

// langList adds, once, the datalist of fence languages the code bar offers.
function langList() {
	const id = "tt-langs";
	if (!document.getElementById(id)) {
		const dl = Object.assign(document.createElement("datalist"), { id });
		for (const l of ["go", "terminal", "bash", "javascript", "typescript", "python", "json", "yaml", "html", "css", "sql", "dockerfile", "nginx", "toml", "rust", "powershell", "diff", "mermaid"]) dl.append(Object.assign(document.createElement("option"), { value: l }));
		document.body.append(dl);
	}
	return id;
}

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

// --- Syntax colors -----------------------------------------------------------
// highlight.js (via lowlight) marks tokens as decorations, so the document
// itself stays plain text. The hljs-* classes share their colors with the
// rendered pages' chroma classes (app.css).

const lowlight = createLowlight(common);
lowlight.register({ dockerfile, nginx, powershell });
const aliases = { terminal: "bash", sh: "bash", zsh: "bash", console: "bash", html: "xml", svg: "xml", yml: "yaml", js: "javascript", ts: "typescript", py: "python", golang: "go", docker: "dockerfile", ps1: "powershell", tf: "ini", toml: "ini" };

// guessable are the languages tried for a block without one.
const guessable = ["go", "bash", "javascript", "typescript", "python", "json", "yaml", "xml", "css", "sql", "dockerfile", "nginx", "ini", "rust", "powershell", "diff"];

// fenceLang is the highlight.js language of a fence info string, or "".
export function fenceLang(info) {
	const first = (info || "").trim().split(/\s+/)[0].toLowerCase();
	const lang = aliases[first] || first;
	return lang && lowlight.registered(lang) ? lang : "";
}

function flatten(nodes, classes = []) {
	return nodes.flatMap((n) => {
		const cls = n.properties?.className ? [...classes, ...n.properties.className] : classes;
		return n.children ? flatten(n.children, cls) : [{ text: n.value || "", cls }];
	});
}

function decorate(doc, name) {
	const decos = [];
	doc.descendants((node, pos) => {
		if (node.type.name !== name) return true;
		const info = (node.attrs.language || "").trim();
		const lang = fenceLang(info);
		// No language: guess, like the renderer (render.highlight) does.
		if (!lang && (info || !node.textContent.trim())) return false;
		const tree = lang ? lowlight.highlight(lang, node.textContent) : lowlight.highlightAuto(node.textContent, { subset: guessable });
		let from = pos + 1;
		for (const part of flatten(tree.children)) {
			const to = from + part.text.length;
			if (part.cls.length) decos.push(Decoration.inline(from, to, { class: part.cls.join(" ") }));
			from = to;
		}
		return false;
	});
	return DecorationSet.create(doc, decos);
}

function highlighter(name) {
	const key = new PluginKey("highlight");
	return new Plugin({
		key,
		state: {
			init: (_, { doc }) => decorate(doc, name),
			apply: (tr, set) => (tr.docChanged ? decorate(tr.doc, name) : set),
		},
		props: { decorations: (state) => key.getState(state) },
	});
}

// --- Embeds --------------------------------------------------------------------
// Mirrors app/internal/render/embeds.go: a paragraph holding only a YouTube
// URL is a player, one holding only a link to a PDF under /media/ is a
// download card. Both are a line of their own in the Markdown.

// youtubeID returns the video id of a YouTube watch, short, shorts or embed
// URL, or "".
export function youtubeID(raw) {
	let u;
	try {
		u = new URL(String(raw).trim());
	} catch {
		return "";
	}
	if (u.protocol !== "https:" && u.protocol !== "http:") return "";
	const host = u.hostname.toLowerCase().replace(/^www\./, "").replace(/^m\./, "");
	let id = "";
	if (host === "youtu.be") id = u.pathname.replace(/^\/+|\/+$/g, "");
	else if (host === "youtube.com" || host === "youtube-nocookie.com") {
		if (u.pathname === "/watch") id = u.searchParams.get("v") || "";
		else if (/^\/(shorts|embed|live)\//.test(u.pathname)) id = u.pathname.split("/").pop();
	}
	return /^[A-Za-z0-9_-]{11}$/.test(id) ? id : "";
}

// A line that is a block of its own: followed by a blank line or the end.
const ownLine = (body) => new RegExp("^" + body + "[ \\t]*(?:\\n|$)(?=[ \\t]*(?:\\n|$))", "i");
const videoLine = ownLine("(https?:\\/\\/\\S+)");
const fileLine = ownLine("\\[([^\\]\\n]+)\\]\\((\\/media\\/[^)\\s]+\\.pdf)\\)");

export const Video = Node.create({
	name: "video",
	group: "block",
	atom: true,
	draggable: true,
	addAttributes() {
		return { src: { default: "", parseHTML: (el) => el.getAttribute("data-src") } };
	},
	parseHTML() {
		return [{ tag: "div[data-video]" }];
	},
	renderHTML({ node }) {
		const id = youtubeID(node.attrs.src);
		return [
			"div",
			{ "data-video": "", "data-src": node.attrs.src, class: "tt-embed tt-video" },
			["iframe", { src: "https://www.youtube-nocookie.com/embed/" + id, title: "YouTube", loading: "lazy", allow: "encrypted-media; picture-in-picture; fullscreen", referrerpolicy: "strict-origin-when-cross-origin", tabindex: "-1" }],
		];
	},
	markdownTokenizer: {
		name: "video",
		level: "block",
		start(src) {
			// marked asks with the first character cut off, so a line start
			// is a newline here; the tokenizer itself runs at block starts.
			const m = src.match(/\nhttps?:\/\/\S*youtu/i);
			return m ? m.index + 1 : -1;
		},
		tokenize(src) {
			const m = src.match(videoLine);
			if (!m || !youtubeID(m[1])) return;
			return { type: "video", raw: m[0], src: m[1] };
		},
	},
	parseMarkdown(token) {
		return { type: "video", attrs: { src: token.src } };
	},
	renderMarkdown(node) {
		return node.attrs.src;
	},
	addCommands() {
		return {
			setVideo:
				(src) =>
				({ commands }) =>
					commands.insertContent({ type: this.name, attrs: { src } }),
		};
	},
});

export const FileCard = Node.create({
	name: "fileCard",
	group: "block",
	atom: true,
	draggable: true,
	addAttributes() {
		return {
			href: { default: "", parseHTML: (el) => el.getAttribute("data-href") },
			name: { default: "", parseHTML: (el) => el.getAttribute("data-name") },
		};
	},
	parseHTML() {
		return [{ tag: "div[data-file]" }];
	},
	renderHTML({ node }) {
		return [
			"div",
			{ "data-file": "", "data-href": node.attrs.href, "data-name": node.attrs.name, class: "tt-embed tt-file" },
			["span", { class: "file-kind mono" }, "PDF"],
			["span", { class: "file-name" }, node.attrs.name || node.attrs.href],
			["span", { class: "tt-file-href mono" }, node.attrs.href],
		];
	},
	markdownTokenizer: {
		name: "fileCard",
		level: "block",
		start(src) {
			const m = src.match(/\n\[[^\]\n]+\]\(\/media\//);
			return m ? m.index + 1 : -1;
		},
		tokenize(src) {
			const m = src.match(fileLine);
			if (!m) return;
			return { type: "fileCard", raw: m[0], name: m[1], href: m[2] };
		},
	},
	parseMarkdown(token) {
		return { type: "fileCard", attrs: { href: token.href, name: token.name } };
	},
	renderMarkdown(node) {
		return `[${(node.attrs.name || "file").replace(/[\[\]]/g, "")}](${node.attrs.href})`;
	},
});
