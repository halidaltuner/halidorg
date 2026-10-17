// Loading state for the sign-in button: the Google redirect can take a moment.
document.querySelector(".login-form")?.addEventListener("submit", (e) => {
	const btn = e.currentTarget.querySelector(".google-btn");
	btn.setAttribute("aria-busy", "true");
	btn.querySelector(".google-label").textContent = btn.dataset.loading;
});
