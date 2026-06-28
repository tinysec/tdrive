// Markdown rendering extensions for tdrive.
//
// Adds common syntax that marked alone does not render, without pulling in a
// build step or extra libraries:
//   - ==highlight==      -> <mark> (inline highlight)
//   - > [!NOTE]/TIP/...  -> GitHub-style alert callouts
//   - heading anchors    -> # link on hover, and an id for deep linking
//   - <!-- toc -->       -> an optional table of contents built from headings
//
// render.js calls tdMdExt.preprocess(source) before marked.parse and
// tdMdExt.postProcess(container) after the HTML is in the DOM.

(function () {
	"use strict";

	// ALERT_TYPES maps a GitHub alert keyword to its css class.
	var ALERT_TYPES = {
		note: "note",
		tip: "tip",
		important: "important",
		warning: "warning",
		caution: "caution"
	};

	// ALERT_ICONS are tiny inline SVGs matching the colour of each alert kind.
	var ALERT_ICONS = {
		note: '<svg viewBox="0 0 16 16" fill="currentColor"><path d="M0 8a8 8 0 1 1 16 0A8 8 0 0 1 0 8Zm8-6.5A6.5 6.5 0 1 0 14.5 8 6.5 6.5 0 0 0 8 1.5ZM6.5 7a1.5 1.5 0 1 1 2.999 0A1.5 1.5 0 0 1 6.5 7Zm1.75 2.75a.75.75 0 0 1 .75.75v1.5a.75.75 0 0 1-1.5 0v-1.5a.75.75 0 0 1 .75-.75Z"/></svg>',
		tip: '<svg viewBox="0 0 16 16" fill="currentColor"><path d="M8 1.5c-2.363 0-4 1.69-4 3.75 0 .984.424 1.625.984 2.304l.214.253c.223.264.47.556.673.848.284.411.546.896.546 1.595a.75.75 0 0 1-1.5 0c0-.302-.092-.543-.276-.811-.165-.243-.4-.56-.625-.828l-.205-.243C3.337 8.955 2.5 7.938 2.5 5.75 2.5 3.364 4.693 0 8 0s5.5 3.364 5.5 5.75c0 2.188-.837 3.205-1.604 4.187l-.206.246c-.224.268-.46.584-.625.827-.184.268-.276.51-.276.812a.75.75 0 0 1-1.5 0c0-.699.263-1.184.547-1.595.202-.292.45-.584.673-.848l.214-.253c.56-.679.984-1.32.984-2.304 0-2.06-1.637-3.75-4-3.75ZM5.75 13.5a.75.75 0 0 0 0 1.5h4.5a.75.75 0 0 0 0-1.5h-4.5Z"/></svg>',
		important: '<svg viewBox="0 0 16 16" fill="currentColor"><path d="M0 1.75C0 .784.784 0 1.75 0h12.5C15.216 0 16 .784 16 1.75v9.5A1.75 1.75 0 0 1 14.25 13H8.06l-2.573 2.573A1.458 1.458 0 0 1 3 14.543V13H1.75A1.75 1.75 0 0 1 0 11.25Zm1.75-.25a.25.25 0 0 0-.25.25v9.5c0 .138.112.25.25.25h2a.75.75 0 0 1 .75.75v2.189l2.72-2.72a.749.749 0 0 1 .53-.219h6.5a.25.25 0 0 0 .25-.25v-9.5a.25.25 0 0 0-.25-.25Zm7 2.25v2.5a.75.75 0 0 1-1.5 0V3.75a.75.75 0 0 1 1.5 0ZM9 9a1 1 0 1 1-2 0 1 1 0 0 1 2 0Z"/></svg>',
		warning: '<svg viewBox="0 0 16 16" fill="currentColor"><path d="M6.457 1.047c.659-1.234 2.427-1.234 3.086 0l6.082 11.378A1.75 1.75 0 0 1 14.082 15H1.918a1.75 1.75 0 0 1-1.543-2.575Zm1.763.707a.25.25 0 0 0-.44 0L1.698 13.132a.25.25 0 0 0 .22.368h12.164a.25.25 0 0 0 .22-.368Zm.53 3.996v2.5a.75.75 0 0 1-1.5 0v-2.5a.75.75 0 0 1 1.5 0ZM9 11a1 1 0 1 1-2 0 1 1 0 0 1 2 0Z"/></svg>',
		caution: '<svg viewBox="0 0 16 16" fill="currentColor"><path d="M4.47.22A.749.749 0 0 1 5 0h6c.199 0 .389.079.53.22l4.25 4.25c.141.14.22.331.22.53v6a.749.749 0 0 1-.22.53l-4.25 4.25A.749.749 0 0 1 11 16H5a.749.749 0 0 1-.53-.22L.22 11.53A.749.749 0 0 1 0 11V5c0-.199.079-.389.22-.53Zm.84 1.28L1.5 5.31v5.38l3.81 3.81h5.38l3.81-3.81V5.31L10.69 1.5ZM8 4a.75.75 0 0 1 .75.75v3.5a.75.75 0 0 1-1.5 0v-3.5A.75.75 0 0 1 8 4Zm0 8a1 1 0 1 1 0-2 1 1 0 0 1 0 2Z"/></svg>'
	};

	var ALERT_LABELS = {
		note: "Note",
		tip: "Tip",
		important: "Important",
		warning: "Warning",
		caution: "Caution"
	};

	// preprocess runs on the raw markdown before marked. It is fence-aware: lines
	// inside a ``` or ~~~ block are left untouched.
	function preprocess(source) {

		var lines = source.split(/\r?\n/);
		var out = [];
		var fenced = false;
		var fenceMarker = null;

		for (var i = 0; i < lines.length; i = i + 1) {
			var line = lines[i];

			// Detect fence boundaries. ``` opens/closes ``` fences, ~~~ for ~~~.
			var fence = line.match(/^\s*(`{3,}|~{3,})/);
			if (null !== fence) {
				var marker = fence[1][0];
				if (false === fenced) {
					fenced = true;
					fenceMarker = marker;
				} else if (marker === fenceMarker) {
					fenced = false;
					fenceMarker = null;
				}
				out.push(line);
				continue;
			}

			if (fenced) {
				out.push(line);
				continue;
			}

			// ==highlight== -> <mark>...</mark>. Avoid touching == inside words
			// like "a==b" by requiring non-= boundaries.
			if (line.indexOf("==") !== -1) {
				line = line.replace(/(^|[^=])==([^=]+)==([^=]|$)/g, function (m, a, text, b) {
					return a + "<mark>" + text + "</mark>" + b;
				});
			}

			if (/^\[toc\]$/i.test(line.trim())) {
				out.push('<div class="toc-marker"></div>');
				continue;
			}

			out.push(line);
		}

		return applyAlertsSource(out.join("\n"));
	}

	// applyAlertsSource turns GitHub alert syntax into HTML so marked renders a
	// styled callout instead of a plain blockquote:
	//
	//   > [!NOTE]
	//   > body line
	//
	// becomes a blockquote carrying an .markdown-alert-* class whose first line is
	// the icon + title. Only consecutive ">" lines that start the block qualify;
	// a non-> line ends the alert.
	function applyAlertsSource(source) {

		var lines = source.split(/\r?\n/);
		var out = [];
		var i = 0;

		while (i < lines.length) {
			var match = lines[i].match(/^>\s*\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*$/i);

			if (null === match) {
				out.push(lines[i]);
				i = i + 1;
				continue;
			}

			var kind = match[1].toLowerCase();
			var body = [];

			i = i + 1;
			while (i < lines.length) {
				var bodyMatch = lines[i].match(/^>\s?(.*)$/);
				if (null === bodyMatch) {
					break;
				}
				body.push(bodyMatch[1]);
				i = i + 1;
			}

			out.push('<div class="markdown-alert markdown-alert-' + kind + '">');
			out.push('<p class="markdown-alert-title">' + (ALERT_ICONS[kind] || "") + " " + ALERT_LABELS[kind] + "</p>");
			out.push("<p>" + body.join("<br>") + "</p>");
			out.push("</div>");
			out.push(""); // blank line so marked does not glue the block to the next line
		}

		return out.join("\n");
	}

	// postProcess runs after marked has produced HTML. It adds heading anchors
	// and builds any <!-- toc --> placeholders.
	function postProcess(root) {

		addHeadingAnchors(root);
		buildToc(root);
	}

	// addHeadingAnchors gives every heading a stable id (if it lacks one) and
	// prepends a # anchor link revealed on hover.
	function addHeadingAnchors(root) {

		var headings = root.querySelectorAll("h1, h2, h3, h4, h5, h6");
		var used = {};

		for (var i = 0; i < headings.length; i = i + 1) {
			var heading = headings[i];

			if (false === heading.hasAttribute("id")) {
				var slug = slugify(heading.textContent);
				var candidate = slug;
				var n = 1;
				while (used[candidate]) {
					n = n + 1;
					candidate = slug + "-" + n;
				}
				used[candidate] = true;
				heading.setAttribute("id", candidate);
			}

			var anchor = document.createElement("a");
			anchor.className = "anchor";
			anchor.href = "#" + heading.getAttribute("id");
			anchor.textContent = "#";
			heading.insertBefore(anchor, heading.firstChild);
		}
	}

	// buildToc replaces each <!-- toc --> HTML comment placeholder (rendered by
	// marked as a text node) with a list of the headings in the document.
	function buildToc(root) {

		var html = root.innerHTML;
		if (html.indexOf("toc") === -1) {
			return;
		}

		// marked drops HTML comments, so the marker must be a visible one: the
		// literal text [toc] on its own line. Replace the first such text node.
		var marker = root.querySelector(".toc-marker");
		if (null === marker) {
			return;
		}

		var headings = root.querySelectorAll("h1[id], h2[id], h3[id], h4[id]");
		if (0 === headings.length) {
			return;
		}

		var list = document.createElement("ol");
		list.className = "markdown-toc-list";

		for (var i = 0; i < headings.length; i = i + 1) {
			var heading = headings[i];
			var level = parseInt(heading.tagName.substring(1), 10);

			var item = document.createElement("li");
			item.setAttribute("data-level", level);

			var link = document.createElement("a");
			link.href = "#" + heading.getAttribute("id");
			link.textContent = heading.textContent.replace(/^#/, "").trim();
			item.appendChild(link);

			list.appendChild(item);
		}

		var wrapper = document.createElement("div");
		wrapper.className = "markdown-toc";
		var title = document.createElement("div");
		title.className = "markdown-toc-title";
		title.textContent = "Contents";
		wrapper.appendChild(title);
		wrapper.appendChild(list);

		marker.replaceWith(wrapper);
	}

	// slugify turns heading text into a URL-safe id.
	function slugify(text) {

		return text.toLowerCase().trim()
			.replace(/[^\p{L}\p{N}\s-]/gu, "")
			.replace(/\s+/g, "-");
	}

	window.tdMdExt = {
		preprocess: preprocess,
		postProcess: postProcess
	};
})();
