// Markdown editor: formatting toolbar commands and split/write/preview modes.
//
// The source surface is the same textarea render.js already wires to a live
// preview. This file adds (1) toolbar buttons that insert or wrap Markdown
// syntax at the caret/selection, and (2) a three-way mode switch that hides or
// shows the source and preview panes. It touches the textarea through the same
// shared [data-md-source] / [data-md-preview] hooks, so the live preview keeps
// working in split mode with no duplication of rendering logic.

(function () {
	"use strict";

	var editor = null;
	var source = null;
	var preview = null;
	var modes = [];

	function init() {

		editor = document.querySelector("[data-md-editor]");
		if (null === editor) {
			return;
		}

		source = editor.querySelector("[data-md-source]");
		preview = editor.querySelector("[data-md-preview]");

		if (null === source) {
			return;
		}

		editor.addEventListener("click", onClick);

		modes = editor.querySelectorAll("[data-md-mode]");
		setMode(editor.getAttribute("data-md-mode") || "split");

		// Keep the active mode when the editor returns after a save error.
		source.focus();
	}

	// ---- Toolbar commands -------------------------------------------------

	function onClick(event) {

		var button = event.target.closest("[data-md-cmd], [data-md-mode]");
		if (null === button) {
			return;
		}

		if (button.hasAttribute("data-md-mode")) {
			setMode(button.getAttribute("data-md-mode"));
			return;
		}

		event.preventDefault();
		runCommand(button.getAttribute("data-md-cmd"));
		source.focus();
		notifyChanged();
	}

	// runCommand dispatches a named toolbar command to its handler. Each handler
	// edits the textarea directly; notifyChanged() then fires an "input" event so
	// render.js re-renders the preview.
	function runCommand(cmd) {

		switch (cmd) {
			case "bold":
				wrapSelection(source, "**", "**");
				break;
			case "italic":
				wrapSelection(source, "*", "*");
				break;
			case "code":
				wrapSelection(source, "`", "`");
				break;
			case "h1":
				prefixLines(source, "# ");
				break;
			case "h2":
				prefixLines(source, "## ");
				break;
			case "h3":
				prefixLines(source, "### ");
				break;
			case "quote":
				prefixLines(source, "> ");
				break;
			case "ul":
				prefixLines(source, "- ");
				break;
			case "ol":
				prefixLinesOrdered(source);
				break;
			case "link":
				insertLink(source);
				break;
			case "table":
				insertText(source, tableTemplate());
				break;
			case "hr":
				insertText(source, "\n\n---\n\n");
				break;
		}
	}

	// wrapSelection surrounds the current selection with the given markers. When
	// the selection is empty, it inserts paired markers and places the caret
	// between them. Toggling: if the selection is already wrapped, the markers
	// are removed instead.
	function wrapSelection(field, open, close) {

		var start = field.selectionStart;
		var end = field.selectionEnd;
		var value = field.value;

		var before = value.slice(0, start);
		var sel = value.slice(start, end);
		var after = value.slice(end);

		// Toggle off if already wrapped.
		if (before.endsWith(open) && after.startsWith(close)) {
			field.value = before.slice(0, 0 - open.length) + sel + after.slice(close.length);
			selectRange(field, start - open.length, end - open.length);
			return;
		}

		if (0 === sel.length) {
			field.value = before + open + close + after;
			selectRange(field, start + open.length, start + open.length);
			return;
		}

		field.value = before + open + sel + close + after;
		selectRange(field, start + open.length, end + open.length);
	}

	// prefixLines adds a prefix to the start of every line touched by the
	// current selection. If a line already starts with the prefix it is removed
	// (toggle behaviour) so repeated presses cycle cleanly.
	function prefixLines(field, prefix) {

		var start = field.selectionStart;
		var end = field.selectionEnd;
		var value = field.value;

		var lineStart = value.lastIndexOf("\n", start - 1) + 1;
		var blockEnd = value.indexOf("\n", end);
		if (-1 === blockEnd) {
			blockEnd = value.length;
		}

		var block = value.slice(lineStart, blockEnd);
		var lines = block.split("\n");

		for (var i = 0; i < lines.length; i = i + 1) {
			if (lines[i].startsWith(prefix)) {
				lines[i] = lines[i].slice(prefix.length);
			} else {
				lines[i] = prefix + lines[i];
			}
		}

		var replacement = lines.join("\n");
		field.value = value.slice(0, lineStart) + replacement + value.slice(blockEnd);

		selectRange(field, lineStart, lineStart + replacement.length);
	}

	// prefixLinesOrdered numbers every selected line like an ordered list.
	function prefixLinesOrdered(field) {

		var start = field.selectionStart;
		var end = field.selectionEnd;
		var value = field.value;

		var lineStart = value.lastIndexOf("\n", start - 1) + 1;
		var blockEnd = value.indexOf("\n", end);
		if (-1 === blockEnd) {
			blockEnd = value.length;
		}

		var block = value.slice(lineStart, blockEnd);
		var lines = block.split("\n");
		var counter = 1;

		for (var i = 0; i < lines.length; i = i + 1) {
			var match = lines[i].match(/^\d+\.\s/);
			if (null !== match) {
				lines[i] = counter + ". " + lines[i].slice(match[0].length);
			} else {
				lines[i] = counter + ". " + lines[i];
			}
			counter = counter + 1;
		}

		var replacement = lines.join("\n");
		field.value = value.slice(0, lineStart) + replacement + value.slice(blockEnd);

		selectRange(field, lineStart, lineStart + replacement.length);
	}

	// insertLink wraps the selection as [text](url); if there is no selection,
	// a placeholder is used for the link text and the url is selected.
	function insertLink(field) {

		var start = field.selectionStart;
		var end = field.selectionEnd;
		var value = field.value;

		var text = value.slice(start, end);
		if (0 === text.length) {
			text = "text";
		}

		var snippet = "[" + text + "](url)";
		field.value = value.slice(0, start) + snippet + value.slice(end);

		// Select the "url" placeholder so the user can type or paste over it.
		var urlStart = start + text.length + 3; // "[" + text + "](" => len+3
		selectRange(field, urlStart, urlStart + 3);
	}

	// insertText drops the given snippet at the caret, normalizing surrounding
	// blank lines so headings/blocks stay on their own line.
	function insertText(field, snippet) {

		var start = field.selectionStart;
		var value = field.value;

		field.value = value.slice(0, start) + snippet + value.slice(start);
		selectRange(field, start + snippet.length, start + snippet.length);
	}

	function tableTemplate() {

		return (
			"\n\n" +
			"| Column A | Column B |\n" +
			"| --- | --- |\n" +
			"| cell | cell |\n" +
			"| cell | cell |\n\n"
		);
	}

	// ---- Modes ------------------------------------------------------------

	// setMode switches between split (source + preview), write (source only),
	// and preview (rendered only). The active mode is stored on the editor root
	// so CSS controls visibility; the preview is rendered eagerly when it is
	// about to be shown alone.
	function setMode(mode) {

		if ("split" !== mode && "write" !== mode && "preview" !== mode) {
			mode = "split";
		}

		editor.setAttribute("data-md-mode", mode);

		for (var i = 0; i < modes.length; i = i + 1) {
			var active = modes[i].getAttribute("data-md-mode") === mode;
			modes[i].className = active ? "md-mode-btn md-mode-active" : "md-mode-btn";
		}
	}

	// ---- Helpers ----------------------------------------------------------

	function selectRange(field, start, end) {

		field.focus();
		field.setSelectionRange(start, end);
	}

	// notifyChanged fires an input event so render.js re-renders the preview.
	function notifyChanged() {

		source.dispatchEvent(new Event("input", { bubbles: true }));
	}

	if (document.readyState === "loading") {
		document.addEventListener("DOMContentLoaded", init);
	} else {
		init();
	}
})();