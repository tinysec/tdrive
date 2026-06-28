// Desktop-style interaction for the file list: click to select, Shift for a
// range, Ctrl/Cmd to toggle, a right-click context menu, and keyboard
// shortcuts. Selection is stored in the row checkboxes (the same state the batch
// bar and batch-delete already use), so this layer only translates gestures into
// checkbox changes and then asks batch.js to refresh the UI.
//
// Selection gestures and the keyboard run only on writable pages (which have the
// checkboxes). The context menu also works read-only, acting on the single
// right-clicked row.

(function () {
	"use strict";

	var anchor = null; // Row that Shift-range selection extends from.

	function writable() {
		return null !== document.querySelector(".row-select");
	}

	// visibleRows returns the file/folder rows currently shown, in DOM order.
	function visibleRows() {
		return Array.prototype.slice.call(document.querySelectorAll(".files tbody .row[data-name]:not([hidden])"));
	}

	function checkboxOf(row) {
		return row.querySelector(".row-select");
	}

	function isSelected(row) {
		var box = checkboxOf(row);
		return null !== box && box.checked;
	}

	function setSelected(row, on) {
		var box = checkboxOf(row);
		if (null !== box) {
			box.checked = on;
		}
	}

	function clearAll() {
		var boxes = document.querySelectorAll(".row-select");
		for (var i = 0; i < boxes.length; i = i + 1) {
			boxes[i].checked = false;
		}
	}

	function refresh() {
		if (window.tdRefreshSelection) {
			window.tdRefreshSelection();
		}
	}

	function selectOnly(row) {
		clearAll();
		setSelected(row, true);
	}

	function selectRange(row) {

		if (null === anchor) {
			selectOnly(row);
			return;
		}

		var rows = visibleRows();
		var from = rows.indexOf(anchor);
		var to = rows.indexOf(row);

		if (from === -1 || to === -1) {
			selectOnly(row);
			return;
		}

		if (from > to) {
			var swap = from;
			from = to;
			to = swap;
		}

		clearAll();

		for (var i = from; i <= to; i = i + 1) {
			setSelected(rows[i], true);
		}
	}

	// onClick turns a click on a row into a selection change. Clicks on the name
	// link, action buttons, or the checkbox keep their own behaviour unless a
	// modifier key is held (then they become selection gestures).
	function onClick(event) {

		if (false === writable()) {
			return;
		}

		var row = event.target.closest(".files tbody .row[data-name]");

		if (null === row) {
			return;
		}

		var onControl = null !== event.target.closest("a, button, input, label");

		if (event.shiftKey || event.ctrlKey || event.metaKey) {

			event.preventDefault();

			if (event.shiftKey) {
				selectRange(row); // Anchor stays put so the range can be adjusted.
			} else {
				setSelected(row, false === isSelected(row));
				anchor = row;
			}

			refresh();
			return;
		}

		// A plain click on a link/button/checkbox keeps its default action.
		if (onControl) {
			return;
		}

		selectOnly(row);
		anchor = row;
		refresh();
	}

	// ---- Context menu ------------------------------------------------------

	var menu = null;

	function selectedRows() {

		var boxes = document.querySelectorAll(".row-select:checked");
		var rows = [];

		for (var i = 0; i < boxes.length; i = i + 1) {
			var row = boxes[i].closest(".row");
			if (null !== row) {
				rows.push(row);
			}
		}

		return rows;
	}

	// encodePath URL-encodes each path segment so a name with spaces or unicode
	// produces a valid URL, mirroring how the server builds links.
	function encodePath(canonical) {

		var parts = canonical.split("/");

		for (var i = 0; i < parts.length; i = i + 1) {
			parts[i] = encodeURIComponent(parts[i]);
		}

		return parts.join("/");
	}

	function showItem(key, on) {
		var item = menu.querySelector('[data-ctx="' + key + '"]');
		if (null !== item) {
			item.hidden = false === on;
		}
	}

	// configureMenu shows the items relevant to the current target and returns
	// false when there is nothing to show.
	function configureMenu(targetRow) {

		var selected = selectedRows();
		var multiple = selected.length > 1;
		var canWrite = writable();
		var isDir = "1" === targetRow.getAttribute("data-dir");

		// With several rows selected only the bulk action (delete) makes sense.
		showItem("open", false === multiple);
		showItem("download", false === multiple && false === isDir);
		showItem("copy", false === multiple);
		showItem("rename", false === multiple && canWrite);
		showItem("delete", canWrite);

		var sep = menu.querySelector("[data-ctx-sep]");
		if (null !== sep) {
			sep.hidden = false === canWrite || multiple;
		}

		return canWrite || false === multiple;
	}

	function openMenuAt(x, y) {

		menu.hidden = false;

		// Clamp to the viewport so the menu never runs off-screen.
		var width = menu.offsetWidth;
		var height = menu.offsetHeight;
		var left = Math.min(x, window.innerWidth - width - 8);
		var top = Math.min(y, window.innerHeight - height - 8);

		menu.style.left = Math.max(8, left) + "px";
		menu.style.top = Math.max(8, top) + "px";
	}

	function closeMenu() {
		if (null !== menu) {
			menu.hidden = true;
		}
		menuTarget = null;
	}

	var menuTarget = null;

	function onContextMenu(event) {

		var row = event.target.closest(".files tbody .row[data-name]");

		if (null === row || null === menu) {
			return;
		}

		event.preventDefault();

		// Right-clicking outside the current multi-selection re-selects just this
		// row (writable pages only; read-only has no selection state).
		if (writable() && false === isSelected(row)) {
			selectOnly(row);
			anchor = row;
			refresh();
		}

		menuTarget = row;

		if (false === configureMenu(row)) {
			closeMenu();
			return;
		}

		openMenuAt(event.clientX, event.clientY);
	}

	function onMenuClick(event) {

		var item = event.target.closest("[data-ctx]");

		if (null === item || null === menuTarget) {
			return;
		}

		var action = item.getAttribute("data-ctx");
		var row = menuTarget;
		var path = row.getAttribute("data-path");
		var isDir = "1" === row.getAttribute("data-dir");

		closeMenu();

		if ("open" === action) {
			window.location.href = (isDir ? "/drive" : "/view") + encodePath(path);
			return;
		}

		if ("download" === action) {
			window.location.href = "/download" + encodePath(path);
			return;
		}

		if ("copy" === action) {
			copyLink(window.location.origin + "/raw" + encodePath(path));
			return;
		}

		if ("rename" === action) {
			clickRowAction(row, "rename");
			return;
		}

		if ("zip" === action) {
			zipFromMenu(path);
			return;
		}

		if ("delete" === action) {
			triggerDelete(row);
			return;
		}
	}

	function copyLink(url) {

		var labels = document.querySelector("[data-toasts]");
		var done = labels ? labels.getAttribute("data-copied") : "";

		copyText(url).then(function (ok) {
			if (window.tdToast) {
				window.tdToast(ok ? (done || url) : url, ok ? "success" : "info");
			}
		});
	}

	// copyText copies to the clipboard. navigator.clipboard only exists in a
	// secure context (https or localhost); tdrive is normally reached over plain
	// http on a LAN, so it falls back to a hidden textarea + execCommand.
	function copyText(text) {

		if (navigator.clipboard && navigator.clipboard.writeText && window.isSecureContext) {
			return navigator.clipboard.writeText(text).then(function () {
				return true;
			}).catch(function () {
				return fallbackCopy(text);
			});
		}

		return Promise.resolve(fallbackCopy(text));
	}

	function fallbackCopy(text) {

		try {
			var area = document.createElement("textarea");
			area.value = text;
			area.style.position = "fixed";
			area.style.top = "-1000px";
			area.style.opacity = "0";
			document.body.appendChild(area);
			area.focus();
			area.select();

			var ok = document.execCommand("copy");
			document.body.removeChild(area);

			return ok;
		} catch (error) {
			return false;
		}
	}

	function clickRowAction(row, action) {
		var button = row.querySelector('[data-action="' + action + '"]');
		if (null !== button) {
			button.click();
		}
	}

	// triggerDelete removes the selection: the batch-delete button when several
	// rows are selected, otherwise the row's own delete button.
	function triggerDelete(row) {

		if (selectedRows().length > 1) {
			var batch = document.querySelector("[data-batch-delete]");
			if (null !== batch) {
				batch.click();
				return;
			}
		}

		clickRowAction(row, "delete");
	}

	// ---- Keyboard ----------------------------------------------------------

	function inField(target) {
		return target.matches("input, textarea, select") || target.isContentEditable;
	}

	function moveSelection(delta) {

		var rows = visibleRows();

		if (0 === rows.length) {
			return;
		}

		var current = selectedRows();
		var index;

		if (1 === current.length) {
			index = rows.indexOf(current[0]) + delta;
		} else {
			index = delta > 0 ? 0 : rows.length - 1;
		}

		if (index < 0) {
			index = 0;
		}

		if (index >= rows.length) {
			index = rows.length - 1;
		}

		selectOnly(rows[index]);
		anchor = rows[index];
		refresh();
		rows[index].scrollIntoView({ block: "nearest" });
	}

	function onKeyDown(event) {

		// "/" focuses the filter from anywhere outside a field.
		if ("/" === event.key && false === inField(event.target)) {
			var filter = document.querySelector("[data-filter]");
			if (null !== filter) {
				event.preventDefault();
				filter.focus();
			}
			return;
		}

		if ("Escape" === event.key) {
			closeMenu();
			if (inField(event.target)) {
				event.target.blur();
			} else if (writable()) {
				clearAll();
				refresh();
			}
			return;
		}

		// The remaining shortcuts are list navigation; ignore them inside fields
		// and on read-only pages.
		if (inField(event.target) || false === writable()) {
			return;
		}

		if ("ArrowDown" === event.key) {
			event.preventDefault();
			moveSelection(1);
			return;
		}

		if ("ArrowUp" === event.key) {
			event.preventDefault();
			moveSelection(-1);
			return;
		}

		var current = selectedRows();

		if ("Enter" === event.key && 1 === current.length) {
			event.preventDefault();
			var isDir = "1" === current[0].getAttribute("data-dir");
			window.location.href = (isDir ? "/drive" : "/view") + encodePath(current[0].getAttribute("data-path"));
			return;
		}

		if (("Delete" === event.key || "Backspace" === event.key) && current.length > 0) {
			event.preventDefault();
			triggerDelete(current[0]);
			return;
		}

		if ("F2" === event.key && 1 === current.length) {
			event.preventDefault();
			clickRowAction(current[0], "rename");
			return;
		}
	}

	function init() {

		menu = document.querySelector("[data-ctxmenu]");

		document.addEventListener("click", onClick);
		document.addEventListener("contextmenu", onContextMenu);
		document.addEventListener("keydown", onKeyDown);

		if (null !== menu) {
			menu.addEventListener("click", onMenuClick);
		}

		// Any click elsewhere, scroll, or list swap closes the menu.
		document.addEventListener("click", function (event) {
			if (null !== menu && false === menu.contains(event.target)) {
				closeMenu();
			}
		});
		document.addEventListener("scroll", closeMenu, true);
		document.addEventListener("htmx:afterSwap", closeMenu);

		// The per-row copy-link button (data-action="copy-link") copies the file's
		// static raw URL. Bound in the capture phase and stopped so it does not
		// flow into the row's selection handler.
		document.addEventListener("click", function (event) {
			var btn = event.target.closest('[data-action="copy-link"]');
			if (null === btn) {
				return;
			}
			event.preventDefault();
			event.stopPropagation();
			var row = btn.closest('tr[data-path]');
			if (null !== row) {
				copyLink(window.location.origin + "/raw" + encodePath(row.getAttribute("data-path")));
			}
		}, true);
		// The per-row extract button (data-action="unzip") extracts a .zip into its
		// containing directory, then refreshes the listing.
		document.addEventListener("click", function (event) {
			var btn = event.target.closest('[data-action="unzip"]');
			if (null === btn) {
				return;
			}
			event.preventDefault();
			event.stopPropagation();
			var row = btn.closest('tr[data-path]');
			if (null === row) {
				return;
			}
			postJson("/api/unzip", {
				path: row.getAttribute("data-path"),
			dir: currentDir()
			}, currentDir());
		}, true);

	// currentDir returns the directory shown by the listing, read from the
	// #file-list container so it works in read-only pages too.
	function currentDir() {

		var list = document.getElementById("file-list");

		return (null !== list) ? list.getAttribute("data-browse-dir") || "/" : "/";
	}

	// zipFromMenu packs a single row (the right-click target) into a .zip named
	// after it, placed in the current directory.
	function zipFromMenu(path) {

		var base = path.split("/").pop() || "archive";
		var dot = base.lastIndexOf(".");
		if (dot > 0) {
			base = base.substring(0, dot);
		}

		postJson("/api/zip", {
			paths: [path],
			dest: base + ".zip",
		dir: currentDir()
		}, currentDir());
	}

	// postJson sends a JSON POST and, on success, refreshes the given directory's
	// listing via htmx. Failures surface the server's localized message.
	function postJson(url, body, refreshDir) {

		fetch(url, {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify(body)
		}).then(function (response) {
			if (response.ok) {
				refreshListing(refreshDir);
				return null;
			}
			return response.text();
		}).then(function (text) {
			if (text && window.tdToast) {
				window.tdToast(text, "error");
			}
		});
	}

	function refreshListing(dir) {

		if (window.htmx) {
			window.htmx.ajax("GET", "/api/list?dir=" + encodeURIComponent(dir), { target: "#file-list", swap: "outerHTML" });
		}
	}
	}

	if (document.readyState === "loading") {
		document.addEventListener("DOMContentLoaded", init);
	} else {
		init();
	}
})();
