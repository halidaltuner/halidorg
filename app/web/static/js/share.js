// POSSE share links (Phase 8.3): the text typed for X goes into its share
// link. The text is not saved and does not autosave the post. Without JS
// the link carries the title.
(() => {
	const text = document.querySelector("[data-share-text]");
	const x = document.querySelector("[data-share-x]");
	const box = document.querySelector("[data-share-url]");
	if (!text || !x || !box) return;
	text.addEventListener("input", (e) => {
		e.stopPropagation(); // not a post edit: keep it from the autosave
		x.href = "https://x.com/intent/tweet?text=" + encodeURIComponent(text.value) + "&url=" + encodeURIComponent(box.dataset.shareUrl);
	});
})();
