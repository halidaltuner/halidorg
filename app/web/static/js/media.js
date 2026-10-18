// Media island: drag-drop and multi-file upload with a progress bar per file
// (XHR, one request each; R2 put is ~180ms p50 plus the transfer), copy
// Markdown, and a two-click delete. The form posts all files without JS.
(() => {
	const form = document.querySelector("form[data-upload]");
	const grid = document.getElementById("md-grid");
	if (form && grid) {
		const input = form.querySelector('input[type="file"]');
		const list = form.querySelector(".md-progress");
		const send = (file) => {
			const li = document.createElement("li");
			const bar = document.createElement("progress");
			bar.max = 100;
			bar.value = 0;
			const name = document.createElement("span");
			name.textContent = file.name;
			li.append(name, bar);
			list.append(li);
			const fd = new FormData();
			fd.append("file", file);
			const x = new XMLHttpRequest();
			x.open("POST", form.action);
			x.setRequestHeader("X-Upload", "1");
			x.upload.onprogress = (e) => { if (e.lengthComputable) bar.value = Math.round((e.loaded / e.total) * 100); };
			x.onload = () => {
				if (x.status === 201) {
					document.getElementById("md-empty")?.remove();
					grid.insertAdjacentHTML("afterbegin", x.responseText);
					if (window.htmx) window.htmx.process(grid.firstElementChild);
					li.remove();
				} else {
					li.classList.add("err");
					bar.replaceWith(document.createTextNode(" · " + (x.responseText || x.status)));
				}
			};
			x.onerror = () => { li.classList.add("err"); };
			x.send(fd);
		};
		const take = (files) => { [...files].forEach(send); input.value = ""; };
		form.addEventListener("submit", (e) => { e.preventDefault(); take(input.files); });
		input.addEventListener("change", () => take(input.files));
		["dragenter", "dragover"].forEach((t) => form.addEventListener(t, (e) => { e.preventDefault(); form.classList.add("over"); }));
		["dragleave", "drop"].forEach((t) => form.addEventListener(t, (e) => { e.preventDefault(); form.classList.remove("over"); }));
		form.addEventListener("drop", (e) => take(e.dataTransfer.files));
	}
	document.addEventListener("click", (e) => {
		const copy = e.target.closest("[data-copy]");
		if (copy && !copy.disabled) {
			navigator.clipboard?.writeText(copy.dataset.copy);
			const t = copy.textContent;
			copy.textContent = "✓";
			setTimeout(() => { copy.textContent = t; }, 1200);
		}
		// Delete asks twice: the first click turns the button into "sure?".
		const del = e.target.closest("[data-confirm-text]");
		if (del && !del.dataset.armed) {
			e.preventDefault();
			del.dataset.armed = "1";
			del.textContent = del.dataset.confirmText;
		}
	});
})();
