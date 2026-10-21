/*
Authoring syntax on top of CommonMark + GFM:

Headings h2 and h3 get stable ids (Turkish letters folded to ASCII) and
appear in the TOC.

Callouts use GitHub alert syntax. The kind line may carry a custom label:

	> [!TIP]
	> Explicit saves always create a revision.

	> [!WARNING] careful
	> OOB targets must exist on every page.

Kinds: NOTE, TIP, WARNING, DANGER (CAUTION is DANGER, IMPORTANT is NOTE).

Footnotes ([^1]) referenced from a top-level paragraph render as side notes
next to that paragraph; any other footnote stays in a list at the end.

Code fences take the language first, then key=value pairs and flags:

	```go file=internal/admin/revisions.go     light block with a filename bar
	```html file=editor.html tabs              consecutive "tabs" fences form one
	```go file=handler.go tabs                 dark block with file tabs
	```go dark                                 dark block on its own
	```terminal cwd=~/halid                    terminal; "$ " lines are commands
	```mermaid                                 kept as source for the Phase 7 island

Dark blocks show line numbers; add "nolines" to drop them.

A paragraph holding only a YouTube URL becomes a player (youtube-nocookie,
lazy), and one holding only a link to a PDF under /media/ becomes a
download card.
*/
package render
