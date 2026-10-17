// Header menus are <details> and work without this file. It only closes an
// open menu on Esc or a click outside it, like the prototype's dropdown.
const menus = () => document.querySelectorAll(".nav-more[open], .nav-sheet[open]");
document.addEventListener("click", (e) => {
	for (const d of menus()) if (!d.contains(e.target)) d.open = false;
});
document.addEventListener("keydown", (e) => {
	if (e.key !== "Escape") return;
	for (const d of menus()) {
		d.open = false;
		d.querySelector("summary").focus();
	}
});
