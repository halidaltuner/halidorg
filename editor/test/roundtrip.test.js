// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";
import fs from "node:fs";
import { Editor } from "@tiptap/core";
import { norm, schema } from "../src/tiptap/extensions.js";

const roundtrip = (md) => {
	const ed = new Editor({ extensions: schema(), content: md, contentType: "markdown" });
	const out = ed.getMarkdown();
	ed.destroy();
	return out;
};

describe("markdown roundtrip", () => {
	for (const file of ["../../app/internal/render/testdata/post.md", "../../app/internal/styleguide/sample.md"]) {
		it(file, () => {
			const md = fs.readFileSync(new URL(file, import.meta.url), "utf8");
			expect(norm(roundtrip(md))).toBe(norm(md));
		});
	}
	const cases = {
		callout: "> [!TIP]\n> Explicit saves always create a revision.",
		"callout with label and two paragraphs": "> [!WARNING] dikkat\n> First.\n>\n> Second with **bold**.",
		"plain quote": "> Just a quote.",
		footnotes: "Text with a note[^1] and another[^long-one].\n\n[^1]: The note.\n\n[^long-one]: A second note with `code`.",
		"code fence info": "```go file=main.go\nfunc main() {}\n```",
		"tabs and terminal": "```html file=a.html tabs\n<p>a</p>\n```\n\n```terminal cwd=~/x\n$ ls\n```",
		mermaid: "```mermaid\nflowchart LR\n  A --> B\n```",
		headings: "## Two\n\n### Three\n\nText.",
		lists: "- one\n- two\n  - nested\n\n1. first\n2. second",
		inline: "Some **bold**, *italic*, `code`, ~~gone~~ and a [link](https://example.com).",
		image: "![A cat on a mat](/media/abc/cat.jpg)",
		table: "| a | b |\n| --- | --- |\n| 1 | 2 |",
		rule: "Above.\n\n---\n\nBelow.",
		turkish: "Işık, ğüşöç ve İstanbul.",
		h1: "# One\n\nText.",
		todo: "- [ ] write the post\n- [x] pick a title",
		youtube: "Before.\n\nhttps://www.youtube.com/watch?v=dQw4w9WgXcQ\n\nAfter.",
		"youtube short link": "https://youtu.be/dQw4w9WgXcQ",
		"url in a sentence stays a link": "See https://youtu.be/dQw4w9WgXcQ here.",
		file: "[My CV](/media/2026/10/cv.pdf)\n\nText.",
		"file after text": "Intro.\n\n[My CV](/media/2026/10/cv.pdf)",
		"file link in a sentence": "Read [the CV](/media/2026/10/cv.pdf) first.",
		"image beside text": "Text.\n\n![A cat](/media/abc/cat.jpg)",
	};
	for (const [name, md] of Object.entries(cases)) {
		it(name, () => expect(norm(roundtrip(md))).toBe(norm(md)));
	}
});
