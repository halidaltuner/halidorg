// The Notion-like layer on top of the schema: a "/" command menu, a
// formatting menu on selected text, a drag handle beside each block and a
// hint on the empty line. Plain DOM in the admin's classes, no framework.
import { Extension } from "@tiptap/core";
import Suggestion from "@tiptap/suggestion";
import BubbleMenu from "@tiptap/extension-bubble-menu";
import DragHandle from "@tiptap/extension-drag-handle";
import { Placeholder } from "@tiptap/extensions";
import { youtubeID } from "./nodes.js";

// pickFile opens the file chooser and resolves to the chosen file, if any.
function pickFile(accept) {
	return new Promise((resolve) => {
		const input = Object.assign(document.createElement("input"), { type: "file", accept, hidden: true });
		input.addEventListener("change", () => {
			resolve(input.files[0] || null);
			input.remove();
		});
		document.body.append(input);
		input.click();
	});
}

// upload stores a file in the media library (POST /admin/media) and
// resolves to { url, name, mime, image }.
export async function upload(url, file) {
	const body = new FormData();
	body.append("file", file);
	const res = await fetch(url, { method: "POST", body, credentials: "same-origin", headers: { "X-Upload": "1", Accept: "application/json" } });
	if (!res.ok) throw new Error((await res.text()).trim() || String(res.status));
	return res.json();
}

// insertUpload uploads file and puts it at the cursor: an image (asking for
// its alt text) or a download card.
export async function insertUpload(editor, file, t, url, pos) {
	editor.view.dom.classList.add("is-busy");
	try {
		const m = await upload(url, file);
		const chain = editor.chain().focus();
		if (typeof pos === "number") chain.setTextSelection(pos);
		if (m.image) chain.setImage({ src: m.url, alt: window.prompt(t("prompt_alt"), "") || "" });
		else chain.insertContent({ type: "fileCard", attrs: { href: m.url, name: m.name.replace(/\.pdf$/i, "") } });
		chain.run();
	} catch (err) {
		window.alert(t("upload_failed") + err.message);
	} finally {
		editor.view.dom.classList.remove("is-busy");
	}
}

// commands lists the "/" menu. Labels come from the page (i18n catalog);
// keys are their fallbacks and what typing after "/" also matches.
function commands(t, uploadURL) {
	const ask = (key, value = "") => window.prompt(t(key), value);
	// Upload commands pick the file after the menu closes; the chain only
	// removes the typed "/command".
	const later = (accept) => (c, editor) => {
		pickFile(accept).then((file) => file && insertUpload(editor, file, t, uploadURL));
		return c;
	};
	return [
		{ key: "text", glyph: "¶", run: (c) => c.setParagraph() },
		{ key: "h1", glyph: "H1", run: (c) => c.toggleHeading({ level: 1 }) },
		{ key: "h2", glyph: "H2", run: (c) => c.toggleHeading({ level: 2 }) },
		{ key: "h3", glyph: "H3", run: (c) => c.toggleHeading({ level: 3 }) },
		{ key: "bullets", glyph: "•", run: (c) => c.toggleBulletList() },
		{ key: "numbers", glyph: "1.", run: (c) => c.toggleOrderedList() },
		{ key: "todo", glyph: "☐", run: (c) => c.toggleTaskList() },
		{ key: "quote", glyph: "❝", run: (c) => c.toggleBlockquote() },
		{ key: "note", glyph: "i", run: (c) => c.setCallout("NOTE") },
		{ key: "tip", glyph: "✓", run: (c) => c.setCallout("TIP") },
		{ key: "warning", glyph: "!", run: (c) => c.setCallout("WARNING") },
		{ key: "danger", glyph: "‼", run: (c) => c.setCallout("DANGER") },
		{ key: "code", glyph: "{}", run: (c) => c.setCodeBlock({ language: "" }) },
		{ key: "terminal", glyph: "$", run: (c) => c.setCodeBlock({ language: "terminal" }) },
		{ key: "mermaid", glyph: "◇", run: (c) => c.setCodeBlock({ language: "mermaid" }) },
		{ key: "table", glyph: "▦", run: (c) => c.insertTable({ rows: 3, cols: 3, withHeaderRow: true }) },
		{ key: "image", glyph: "▣", run: later("image/png,image/jpeg,image/gif,image/webp") },
		{
			key: "image_url",
			glyph: "▢",
			run: (c) => {
				const src = ask("prompt_image");
				if (!src) return c;
				return c.setImage({ src: src.trim(), alt: ask("prompt_alt") || "" });
			},
		},
		{
			key: "youtube",
			glyph: "▶",
			run: (c) => {
				const src = ask("prompt_youtube");
				if (!src) return c;
				if (!youtubeID(src)) {
					window.alert(t("bad_youtube"));
					return c;
				}
				return c.setVideo(src.trim());
			},
		},
		{ key: "file", glyph: "⎙", run: later("application/pdf") },
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
		return { t: (k) => k, uploadURL: "/admin/media" };
	},
	addProseMirrorPlugins() {
		const t = this.options.t;
		const all = commands(t, this.options.uploadURL);
		return [
			Suggestion({
				editor: this.editor,
				char: "/",
				allowSpaces: false,
				startOfLine: false,
				items: ({ query }) => {
					const q = query.toLowerCase();
					return all.filter((c) => !q || c.key.includes(q) || t("cmd_" + c.key).toLowerCase().includes(q)).slice(0, 30);
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
export function ui(t, uploadURL) {
	const menu = bubble(t);
	const handle = document.createElement("div");
	handle.className = "tt-handle";
	handle.textContent = "⋮⋮";
	handle.title = t("drag");
	return {
		extensions: [
			SlashMenu.configure({ t, uploadURL }),
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
