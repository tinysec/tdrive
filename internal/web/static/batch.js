// Multi-select for the file list: tracks checked rows, keeps a "select all"
// box in sync, and enables the batch-delete button with a live count. Event
// delegation is used so it keeps working after htmx swaps the list.

(function () {
	"use strict";

	// formatSize renders a byte count the same way the server does (binary units,
	// one decimal), so the selection summary matches the listing.
	function formatSize(bytes) {

		if (bytes < 1024) {
			return String(bytes) + " B";
		}

		var labels = ["KB", "MB", "GB", "TB", "PB"];
		var value = bytes;
		var exp = -1;

		while (value >= 1024 && exp < labels.length - 1) {
			value = value / 1024;
			exp = exp + 1;
		}

		return value.toFixed(1) + " " + labels[exp];
	}

	// updateBar refreshes the selection count, shows the contextual action bar
	// only while something is selected, enables the delete button, and fills the
	// status-bar selection summary (count + combined size).
	function updateBar() {

		var checked = document.querySelectorAll(".row-select:checked");
		var selected = checked.length;

		var bar = document.querySelector("[data-batch-bar]");
		var button = document.querySelector("[data-batch-delete]");
		var count = document.querySelector("[data-batch-count]");

		if (null !== count) {
			count.textContent = String(selected);
		}

		if (null !== button) {
			button.disabled = 0 === selected;
		}

		if (null !== bar) {
			bar.hidden = 0 === selected;
		}

		markSelectedRows();
		updateStatusSelection(checked, selected);
	}

	// markSelectedRows mirrors each checkbox's state onto its row so the selection
	// is visible (select.js drives the checkboxes; the class is the highlight).
	function markSelectedRows() {

		var boxes = document.querySelectorAll(".row-select");

		for (var i = 0; i < boxes.length; i = i + 1) {

			var row = boxes[i].closest(".row");

			if (null !== row) {
				if (boxes[i].checked) {
					row.classList.add("selected");
				} else {
					row.classList.remove("selected");
				}
			}
		}
	}

	function updateStatusSelection(checked, selected) {

		var summary = document.querySelector("[data-status-selection]");

		if (null === summary) {
			return;
		}

		if (0 === selected) {
			summary.hidden = true;
			summary.textContent = "";
			return;
		}

		var bytes = 0;

		for (var i = 0; i < checked.length; i = i + 1) {

			var row = checked[i].closest(".row");

			if (null !== row) {
				bytes = bytes + parseInt(row.getAttribute("data-size") || "0", 10);
			}
		}

		var prefix = summary.getAttribute("data-sel-prefix") || "";
		var suffix = summary.getAttribute("data-sel-suffix") || "";

		summary.textContent = prefix + selected + suffix + " · " + formatSize(bytes);
		summary.hidden = false;
	}

	// setAll checks or unchecks every row checkbox.
	function setAll(checked) {

		var boxes = document.querySelectorAll(".row-select");

		for (var i = 0; i < boxes.length; i = i + 1) {
			boxes[i].checked = checked;
		}
	}

	function onChange(event) {

		var target = event.target;

		if (target.matches("[data-select-all]")) {
			setAll(target.checked);
			updateBar();
			return;
		}

		if (target.matches(".row-select")) {
			updateBar();
		}
	}

	document.addEventListener("change", onChange);

	// After htmx replaces the list, the selection is gone — reset the bar.
	document.addEventListener("htmx:afterSwap", updateBar);

	// select.js drives the checkboxes directly (click/shift/ctrl, keyboard); it
	// calls this after changing them so the bar, highlight, and status stay in sync.
	window.tdRefreshSelection = updateBar;
})();
