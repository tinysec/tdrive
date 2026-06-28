// Client-side findability for the file listing: an instant name filter for the
// current folder and column-header sorting (folders always first). Both work on
// the already-rendered rows, so there is no server round-trip; the sort choice
// is remembered in localStorage and both are re-applied after htmx swaps the
// list (e.g. following an upload or delete).

(function () {
	"use strict";

	var STORAGE_KEY = "tdrive.sort";
	var VIEW_KEY = "tdrive.view";

	var sortKey = "name"; // "name" | "size" | "mtime"
	var sortDir = "asc";  // "asc" | "desc"
	var view = "list";    // "list" | "grid"

	function loadSort() {

		try {
			var raw = localStorage.getItem(STORAGE_KEY);
			if (raw) {
				var saved = JSON.parse(raw);
				if (saved.key) {
					sortKey = saved.key;
				}
				if (saved.dir) {
					sortDir = saved.dir;
				}
			}
		} catch (error) {
			// localStorage may be unavailable; fall back to the defaults.
		}
	}

	function saveSort() {

		try {
			localStorage.setItem(STORAGE_KEY, JSON.stringify({ key: sortKey, dir: sortDir }));
		} catch (error) {
			// Ignore storage failures; sorting still works for this page.
		}
	}

	// dataRows returns the file/folder rows, excluding the "up one level" row and
	// any transient inline-edit rows (they carry no data-name).
	function dataRows() {

		return Array.prototype.slice.call(document.querySelectorAll(".files tbody .row[data-name]"));
	}

	function name(row) {

		return (row.getAttribute("data-name") || "").toLowerCase();
	}

	function compareRows(a, b) {

		// Directories always come before files.
		var aDir = "1" === a.getAttribute("data-dir");
		var bDir = "1" === b.getAttribute("data-dir");

		if (aDir !== bDir) {
			return aDir ? -1 : 1;
		}

		var result = 0;

		if ("size" === sortKey) {
			result = parseInt(a.getAttribute("data-size"), 10) - parseInt(b.getAttribute("data-size"), 10);
		} else if ("mtime" === sortKey) {
			result = parseInt(a.getAttribute("data-mtime"), 10) - parseInt(b.getAttribute("data-mtime"), 10);
		} else {
			result = name(a).localeCompare(name(b), undefined, { numeric: true });
		}

		// Ties break on name so the order is stable and predictable.
		if (0 === result) {
			result = name(a).localeCompare(name(b), undefined, { numeric: true });
		}

		return "asc" === sortDir ? result : -result;
	}

	function applySort() {

		var body = document.querySelector(".files tbody");

		if (null === body) {
			return;
		}

		var rows = dataRows();
		rows.sort(compareRows);

		// Re-append in order. The parent row (no data-name) is never moved, so it
		// stays at the top.
		for (var i = 0; i < rows.length; i = i + 1) {
			body.appendChild(rows[i]);
		}

		updateHeaders();
	}

	function updateHeaders() {

		var headers = document.querySelectorAll(".files thead th[data-sort]");

		for (var i = 0; i < headers.length; i = i + 1) {

			var header = headers[i];

			if (header.getAttribute("data-sort") === sortKey) {
				header.setAttribute("data-active", "");
				header.setAttribute("data-dir", sortDir);
			} else {
				header.removeAttribute("data-active");
				header.removeAttribute("data-dir");
			}
		}
	}

	function onHeaderClick(event) {

		var header = event.target.closest(".files thead th[data-sort]");

		if (null === header) {
			return;
		}

		var key = header.getAttribute("data-sort");

		if (key === sortKey) {
			sortDir = "asc" === sortDir ? "desc" : "asc";
		} else {
			sortKey = key;
			sortDir = "asc";
		}

		saveSort();
		applySort();
		applyFilter();
	}

	function filterTerm() {

		var input = document.querySelector("[data-filter]");

		if (null === input) {
			return "";
		}

		return input.value.trim().toLowerCase();
	}

	function applyFilter() {

		var term = filterTerm();
		var rows = dataRows();
		var matched = 0;

		for (var i = 0; i < rows.length; i = i + 1) {

			var show = "" === term || name(rows[i]).indexOf(term) !== -1;
			rows[i].hidden = false === show;

			if (show) {
				matched = matched + 1;
			}
		}

		updateFilterInfo(term, matched);
	}

	function updateFilterInfo(term, matched) {

		var info = document.querySelector("[data-filter-info]");

		if (null === info) {
			return;
		}

		if ("" === term) {
			info.hidden = true;
			info.textContent = "";
			return;
		}

		var prefix = info.getAttribute("data-match-prefix") || "";
		var suffix = info.getAttribute("data-match-suffix") || "";

		info.textContent = prefix + matched + suffix;
		info.hidden = false;
	}

	function onFilterInput(event) {

		if (event.target.matches && event.target.matches("[data-filter]")) {
			applyFilter();
		}
	}

	// ---- View (list / grid) ------------------------------------------------

	function loadView() {

		try {
			var saved = localStorage.getItem(VIEW_KEY);
			if (saved) {
				view = saved;
			}
		} catch (error) {
			// Default to list view.
		}
	}

	function saveView() {

		try {
			localStorage.setItem(VIEW_KEY, view);
		} catch (error) {
			// Ignore storage failures.
		}
	}

	// loadThumbnails sets the src on image thumbnails the first time grid view is
	// shown, so list view never downloads the images.
	function loadThumbnails() {

		var images = document.querySelectorAll(".entry-thumb[data-thumb]");

		for (var i = 0; i < images.length; i = i + 1) {
			if (!images[i].getAttribute("src")) {
				images[i].setAttribute("src", images[i].getAttribute("data-thumb"));
			}
		}
	}

	function applyView() {

		var table = document.querySelector(".files");

		if (null !== table) {
			if ("grid" === view) {
				table.classList.add("is-grid");
				loadThumbnails();
			} else {
				table.classList.remove("is-grid");
			}
		}

		var buttons = document.querySelectorAll("[data-view-toggle] [data-view]");

		for (var i = 0; i < buttons.length; i = i + 1) {
			if (buttons[i].getAttribute("data-view") === view) {
				buttons[i].setAttribute("data-active", "");
			} else {
				buttons[i].removeAttribute("data-active");
			}
		}
	}

	function onViewClick(event) {

		var button = event.target.closest("[data-view-toggle] [data-view]");

		if (null === button) {
			return;
		}

		view = button.getAttribute("data-view");
		saveView();
		applyView();
	}

	function reapply() {

		applySort();
		applyFilter();
		applyView();
	}

	loadSort();
	loadView();
	document.addEventListener("click", onHeaderClick);
	document.addEventListener("click", onViewClick);
	document.addEventListener("input", onFilterInput);
	document.addEventListener("htmx:afterSwap", reapply);
	reapply();
})();
