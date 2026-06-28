// Drag-and-drop move: drag a file or folder row onto a folder (or the
// "up one level" row) to move it there. Uses event delegation so it keeps
// working after htmx swaps the list, and posts to /api/move via htmx.

(function () {
	"use strict";

	var draggedPath = null;

	function currentDir() {

		var input = document.querySelector(".files-form [name=\"dir\"]");

		if (null !== input) {
			return input.value;
		}

		return "/";
	}

	function onDragStart(event) {

		var row = event.target.closest("[data-drag-path]");

		if (null === row) {
			return;
		}

		draggedPath = row.getAttribute("data-drag-path");

		if (null !== event.dataTransfer) {
			event.dataTransfer.effectAllowed = "move";
		}
	}

	function onDragOver(event) {

		var target = event.target.closest("[data-drop-dir]");

		if (null === target || null === draggedPath) {
			return;
		}

		event.preventDefault();
		target.classList.add("drop-hover");
	}

	function onDragLeave(event) {

		var target = event.target.closest("[data-drop-dir]");

		if (null !== target) {
			target.classList.remove("drop-hover");
		}
	}

	function onDrop(event) {

		var target = event.target.closest("[data-drop-dir]");

		if (null === target || null === draggedPath) {
			return;
		}

		event.preventDefault();
		target.classList.remove("drop-hover");

		var targetDir = target.getAttribute("data-drop-dir");
		var path = draggedPath;

		draggedPath = null;

		if (window.htmx) {
			window.htmx.ajax("POST", "/api/move", {
				values: { path: path, dir: currentDir(), targetDir: targetDir },
				target: "#file-list",
				swap: "outerHTML"
			});
		}
	}

	document.addEventListener("dragstart", onDragStart);
	document.addEventListener("dragover", onDragOver);
	document.addEventListener("dragleave", onDragLeave);
	document.addEventListener("drop", onDrop);
})();
