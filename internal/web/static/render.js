// Client-side rendering shared by the file viewer and the editor's live preview.
//
// Rendering on the client (rather than the server) guarantees that the preview
// shown while editing is produced by exactly the same code as the final view, so
// the two always match. Markdown is rendered with marked; fenced ```mermaid
// blocks are turned into Mermaid diagrams; HTML is previewed in an iframe.

(function () {
	"use strict";

	var mermaidReady = false;

	// renderMarkdown fills target with the rendered HTML of the given source text,
	// renders any Mermaid diagrams, and syntax-highlights the code blocks.
	function renderMarkdown(target, source) {

		target.innerHTML = window.marked.parse(window.tdMdExt ? window.tdMdExt.preprocess(source) : source);

		if (window.tdMdExt) { window.tdMdExt.postProcess(target); }
		convertMermaidBlocks(target);
		runMermaid(target);
		highlightCode(target);
		renderMath(target);
	}

	// renderMath typesets LaTeX math ($...$, $$...$$, \(...\), \[...\]) with KaTeX.
	// It skips code/pre by default, and is a no-op when KaTeX is not loaded.
	function renderMath(target) {

		if (false === Boolean(window.renderMathInElement)) {
			return;
		}

		window.renderMathInElement(target, {
			delimiters: [
				{ left: "$$", right: "$$", display: true },
				{ left: "$", right: "$", display: false },
				{ left: "\\(", right: "\\)", display: false },
				{ left: "\\[", right: "\\]", display: true }
			],
			throwOnError: false
		});
	}

	// highlightCode applies syntax highlighting to code blocks that carry a
	// language class (Mermaid blocks have already been removed). It is a no-op
	// when highlight.js is not loaded on the page.
	function highlightCode(target) {

		if (false === Boolean(window.hljs)) {
			return;
		}

		var blocks = target.querySelectorAll("code[class*=\"language-\"]");

		for (var i = 0; i < blocks.length; i = i + 1) {
			window.hljs.highlightElement(blocks[i]);
		}
	}

	// convertMermaidBlocks rewrites <code class="language-mermaid"> blocks into the
	// <div class="mermaid"> containers Mermaid expects.
	function convertMermaidBlocks(target) {

		var blocks = target.querySelectorAll("code.language-mermaid");

		for (var i = 0; i < blocks.length; i = i + 1) {

			var code = blocks[i];

			var container = document.createElement("div");
			container.className = "mermaid";
			container.textContent = code.textContent;

			var pre = code.closest("pre");

			if (null !== pre) {
				pre.replaceWith(container);
			} else {
				code.replaceWith(container);
			}
		}
	}

	// runMermaid renders the Mermaid containers found inside target.
	function runMermaid(target) {

		if (false === Boolean(window.mermaid)) {
			return;
		}

		if (false === mermaidReady) {
			window.mermaid.initialize({ startOnLoad: false, securityLevel: "strict" });
			mermaidReady = true;
		}

		var nodes = target.querySelectorAll(".mermaid");

		if (nodes.length > 0) {
			window.mermaid.run({ nodes: nodes });
		}
	}

	// debounce delays calling fn until input stops for the given milliseconds.
	function debounce(fn, delayMs) {

		var timer = null;

		return function () {
			if (null !== timer) {
				clearTimeout(timer);
			}
			timer = setTimeout(fn, delayMs);
		};
	}

	// initMarkdownView renders a read-only markdown view by fetching its source.
	function initMarkdownView() {

		var view = document.querySelector("[data-markdown-view]");

		if (null === view) {
			return;
		}

		fetch(view.getAttribute("data-src")).then(function (response) {
			return response.text();
		}).then(function (source) {
			renderMarkdown(view, source);
		});
	}

	// initMarkdownEditor wires the editor textarea to a live markdown preview.
	function initMarkdownEditor() {

		var source = document.querySelector("[data-md-source]");
		var preview = document.querySelector("[data-md-preview]");

		if (null === source || null === preview) {
			return;
		}

		function update() {
			renderMarkdown(preview, source.value);
		}

		source.addEventListener("input", debounce(update, 250));
		update();
	}

	// initHtmlEditor wires the editor textarea to a live HTML preview iframe.
	function initHtmlEditor() {

		var source = document.querySelector("[data-html-source]");
		var preview = document.querySelector("[data-html-preview]");

		if (null === source || null === preview) {
			return;
		}

		function update() {
			preview.srcdoc = source.value;
		}

		source.addEventListener("input", debounce(update, 250));
		update();
	}

	// initReadmeText loads a plain-text directory README (README.txt) into its
	// <pre> container. Markdown READMEs are handled by initMarkdownView.
	function initReadmeText() {

		var node = document.querySelector("[data-dir-readme-text]");

		if (null === node) {
			return;
		}

		fetch(node.getAttribute("data-src")).then(function (response) {
			return response.text();
		}).then(function (text) {
			node.textContent = text;
		});
	}

	function init() {
		// Highlight any server-rendered code block (the text viewer).
		highlightCode(document);

		initMarkdownView();
		initMarkdownEditor();
		initHtmlEditor();
		initReadmeText();
	}

	if (document.readyState === "loading") {
		document.addEventListener("DOMContentLoaded", init);
	} else {
		init();
	}
})();
