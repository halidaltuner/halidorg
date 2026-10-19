// The Notion-like layer on top of the schema: a "/" command menu, a
// formatting menu on selected text, a drag handle beside each block and a
// hint on the empty line. Plain DOM in the admin's classes, no framework.
import { Extension } from "@tiptap/core";
import Suggestion from "@tiptap/suggestion";
import BubbleMenu from "@tiptap/extension-bubble-menu";
import DragHandle from "@tiptap/extension-drag-handle";
import { Placeholder } from "@tiptap/extensions";

// commands lists the "/" menu. Labels come from the page (i18n catalog);
// keys are their fallbacks and what typing after "/" also matches.
function commands(t) {
	const ask = (key, value = "") => window.prompt(t(key), value);
	return [
		{ key: "text", glyph: "¶", run: (c) => c.setParagraph() },
		{ key: "h2", glyph: "H2", run: (c) => c.toggleHeading({ level: 2 }) },
		{ key: "h3", glyph: "H3", run: (c) => c.toggleHeading({ level: 3 }) },
		{ key: "bullets", glyph: "•", run: (c) => c.toggleBulletList() },
		{ key: "numbers", glyph: "1.", run: (c) => c.toggleOrderedList() },
		{ key: "quote", glyph: "❝", run: (c) => c.toggleBlockquote() },
		{ key: "note", glyph: "i", run: (c) => c.setCallout("NOTE") },
		{ key: "tip", glyph: "✓", run: (c) => c.setCallout("TIP") },
		{ key: "warning", glyph: "!", run: (c) => c.setCallout("WARNING") },
		{ key: "danger", glyph: "‼", run: (c) => c.setCallout("DANGER") },
		{ key: "code", glyph: "{}", run: (c) => c.setCodeBlock({ language: "" }) },
		{ key: "terminal", glyph: "$", run: (c) => c.setCodeBlock({ language: "terminal" }) },
		{ key: "mermaid", glyph: "◇", run: (c) => c.setCodeBlock({ language: "mermaid" }) },
		{ key: "table", glyph: "▦", run: (c) => c.insertTable({ rows: 3, cols: 3, withHeaderRow: true }) },
		{
			key: "image",
			glyph: "▣",
			run: (c) => {
				const src = ask("prompt_image");
				if (!src) return c;
				return c.setImage({ src: src.trim(), alt: ask("prompt_alt") || "" });
			},
		},
		{ key: "divider", glyph: "—", run: (c) => c.setHorizontalRule() },
		{
			key: "footnote",
			glyph: "¹",
			run: (c, editor) => {
				// The next free number, the reference here and its note at the end.
				const used = new Set();
				editor.state.doc.descendants((n) => {
					if (n.type.name === "footnoteRef" || n.type.name === "footnote") used.add(n.attrs.label);
				});
				let n = 1;
				while (used.has(String(n))) n++;
				const label = String(n);
				return c
					.insertContent({ type: "footnoteRef", attrs: { label } })
					.command(({ tr, state }) => {
						tr.insert(tr.doc.content.size, state.schema.nodes.footnote.create({ label }));
						return true;
					});
			},
		},
	];
}

// SlashMenu opens the command list when "/" starts a word.
const SlashMenu = Extension.create({
	name: "slashMenu",
	addOptions() {
		return { t: (k) => k };
	},
	addProseMirrorPlugins() {
		const t = this.options.t;
		const all = commands(t);
		return [
			Suggestion({
				editor: this.editor,
				char: "/",
				allowSpaces: false,
				startOfLine: false,
				items: ({ query }) => {
					const q = query.toLowerCase();
					return all.filter((c) => !q || c.key.includes(q) || t("cmd_" + c.key).toLowerCase().includes(q)).slice(0, 12);
				},
				command: ({ editor, range, props }) => {
					props.run(editor.chain().focus().deleteRange(range), editor).run();
				},
				render: () => {
					let el, items = [], sel = 0, pick, unmount;
					const draw = () => {
						el.replaceChildren(
							...(items.length
								? items.map((c, i) => {
										const b = document.createElement("button");
										b.type = "button";
										b.className = "tt-cmd" + (i === sel ? " is-on" : "");
										b.setAttribute("role", "option");
										b.setAttribute("aria-selected", String(i === sel));
										b.innerHTML = '<span class="tt-cmd-g mono"></span><span></span>';
										b.firstChild.textContent = c.glyph;
										b.lastChild.textContent = t("cmd_" + c.key);
										b.addEventListener("mousedown", (e) => {
											e.preventDefault();
											pick(c);
										});
										return b;
									})
								: [Object.assign(document.createElement("p"), { className: "tt-cmd-none", textContent: t("cmd_none") })]),
						);
						el.querySelector(".is-on")?.scrollIntoView({ block: "nearest" });
					};
					return {
						onStart: (props) => {
							el = document.createElement("div");
							el.className = "tt-menu";
							el.setAttribute("role", "listbox");
							items = props.items;
							pick = (c) => props.command(c);
							sel = 0;
							draw();
							unmount = props.mount(el);
						},
						onUpdate: (props) => {
							items = props.items;
							pick = (c) => props.command(c);
							sel = Math.min(sel, Math.max(items.length - 1, 0));
							draw();
						},
						onKeyDown: ({ event }) => {
							if (event.key === "ArrowDown") {
								sel = (sel + 1) % Math.max(items.length, 1);
								draw();
								return true;
							}
							if (event.key === "ArrowUp") {
								sel = (sel - 1 + items.length) % Math.max(items.length, 1);
								draw();
								return true;
							}
							if (event.key === "Enter" && items[sel]) {
								pick(items[sel]);
								return true;
							}
							return false;
						},
						onExit: () => {
							unmount?.();
							el?.remove();
						},
					};
				},
			}),
		];
	},
});

// bubble builds the formatting menu shown on selected text.
function bubble(t) {
	const el = document.createElement("div");
	el.className = "tt-bubble";
	const buttons = [
		["bold", "B", (c) => c.toggleBold()],
		["italic", "I", (c) => c.toggleItalic()],
		["strike", "S", (c) => c.toggleStrike()],
		["code", "</>", (c) => c.toggleCode()],
		[
			"link",
			"↗",
			(c, editor) => {
				const cur = editor.getAttributes("link").href || "";
				const href = window.prompt(t("prompt_link"), cur);
				if (href === null) return c;
				return href.trim() ? c.extendMarkRange("link").setLink({ href: href.trim() }) : c.extendMarkRange("link").unsetLink();
			},
		],
	];
	let editor;
	for (const [key, label, run] of buttons) {
		const b = document.createElement("button");
		b.type = "button";
		b.className = "tt-bb tt-bb-" + key;
		b.textContent = label;
		b.title = t("fmt_" + key);
		b.setAttribute("aria-label", t("fmt_" + key));
		b.dataset.mark = key === "code" ? "code" : key;
		b.addEventListener("mousedown", (e) => e.preventDefault());
		b.addEventListener("click", () => run(editor.chain().focus(), editor).run());
		el.append(b);
	}
	return {
		el,
		bind(ed) {
			editor = ed;
			ed.on("selectionUpdate", () => {
				for (const b of el.children) b.classList.toggle("is-on", ed.isActive(b.dataset.mark));
			});
		},
	};
}

// ui returns the extensions that make up the Notion-like layer, and a hook
// to call once the editor exists.
export function ui(t) {
	const menu = bubble(t);
	const handle = document.createElement("div");
	handle.className = "tt-handle";
	handle.textContent = "⋮⋮";
	handle.title = t("drag");
	return {
		extensions: [
			SlashMenu.configure({ t }),
			BubbleMenu.configure({
				element: menu.el,
				// On the page, not in the scrolling editor box: positions are
				// then plain viewport coordinates.
				appendTo: () => document.body,
				options: { strategy: "fixed", placement: "top", offset: 8, flip: true, shift: { padding: 8 } },
				shouldShow: ({ editor, state }) => !state.selection.empty && !editor.isActive("codeBlock") && !editor.isActive("image"),
			}),
			// Shown only once it sits beside a block, not at its initial spot.
			DragHandle.configure({ render: () => handle, onNodeChange: ({ node }) => handle.classList.toggle("is-on", !!node) }),
			Placeholder.configure({
				placeholder: ({ node, pos }) => (pos === 0 && node.type.name === "paragraph" ? t("placeholder_first") : node.type.name === "paragraph" ? t("placeholder") : ""),
				includeChildren: false,
			}),
		],
		bind: (editor) => menu.bind(editor),
	};
}
