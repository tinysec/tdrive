// Lightweight toast notifications.
//
// Two roles: (1) surface backend errors that htmx would otherwise drop — the
// handlers reply to failures with a localized message and a 4xx/5xx status, and
// htmx:responseError carries that text here; (2) confirm the discrete mutations
// the user triggers (delete / rename / move / new folder). Other code can also
// call window.tdToast(message, kind) directly (e.g. "link copied").

(function () {
	"use strict";

	var container = null;

	function ensureContainer() {

		if (null !== container && document.body.contains(container)) {
			return container;
		}

		container = document.createElement("div");
		container.className = "toast-container";
		document.body.appendChild(container);

		return container;
	}

	// show displays a toast of the given kind ("success" | "error" | "info") and
	// removes it after a delay (errors linger longer).
	function show(message, kind) {

		if (!message) {
			return;
		}

		var host = ensureContainer();

		var toast = document.createElement("div");
		toast.className = "toast toast-" + (kind || "info");
		toast.textContent = message;
		host.appendChild(toast);

		// Trigger the enter transition on the next frame.
		requestAnimationFrame(function () {
			toast.classList.add("toast-in");
		});

		var ttl = "error" === kind ? 5000 : 2500;

		setTimeout(function () {

			toast.classList.remove("toast-in");

			setTimeout(function () {
				if (null !== toast.parentNode) {
					toast.parentNode.removeChild(toast);
				}
			}, 200);
		}, ttl);
	}

	window.tdToast = show;

	function labels() {

		return document.querySelector("[data-toasts]");
	}

	// requestPath extracts the request path from a finished htmx request.
	function requestPath(detail) {

		var xhr = detail && detail.xhr;

		if (xhr && xhr.responseURL) {
			try {
				return new URL(xhr.responseURL).pathname;
			} catch (error) {
				// Fall through to the request config below.
			}
		}

		var config = detail && detail.requestConfig;

		return config && config.path ? config.path : "";
	}

	// successMessage maps a mutation endpoint to its confirmation text. Read-only
	// endpoints (list refresh, rename form) return "" so they stay silent.
	function successMessage(path) {

		var element = labels();

		if (null === element) {
			return "";
		}

		if (0 === path.indexOf("/api/batch-delete") || 0 === path.indexOf("/api/delete")) {
			return element.getAttribute("data-deleted");
		}

		if (0 === path.indexOf("/api/rename") && 0 !== path.indexOf("/api/rename-form")) {
			return element.getAttribute("data-renamed");
		}

		if (0 === path.indexOf("/api/move")) {
			return element.getAttribute("data-moved");
		}

		if (0 === path.indexOf("/api/mkdir")) {
			return element.getAttribute("data-created");
		}

		return "";
	}

	function onAfterRequest(event) {

		var detail = event.detail;

		if (!detail || !detail.successful) {
			return;
		}

		var message = successMessage(requestPath(detail));

		if (message) {
			show(message, "success");
		}
	}

	function onResponseError(event) {

		var xhr = event.detail && event.detail.xhr;
		var text = xhr && xhr.responseText ? xhr.responseText.trim() : "";

		if (text.length > 200) {
			text = text.slice(0, 200);
		}

		var fallback = labels() ? labels().getAttribute("data-error") : "";

		show(text || fallback || "Error", "error");
	}

	document.addEventListener("htmx:afterRequest", onAfterRequest);
	document.addEventListener("htmx:responseError", onResponseError);
})();
