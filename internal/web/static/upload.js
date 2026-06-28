// Chunked upload, panel-free.
//
// The toolbar's upload button opens the native file picker; files dropped
// anywhere on the page are also accepted. Dropping a folder (or picking one
// with the folder picker) recreates its subtree under the current directory,
// creating sub-directories as the server writes each file. Progress is shown in
// a small toast in the bottom-right corner. The browser slices each file into
// fixed-size chunks and uploads them one by one so very large files do not need
// to fit in a single request.

(function () {
	"use strict";

	var CHUNK_SIZE = 5 * 1024 * 1024; // 5 MB per chunk.

	var toast = null;
	var input = null;
	var listElement = null;
	var overallProgress = null;
	var titleElement = null;

	// Upload queue and aggregate progress across the current run. Each queue
	// entry is a task { file, dir } so a dropped folder keeps its relative path.
	var queue = [];
	var running = false;
	var hadFailure = false;
		queue = [];
		active = 0;
	var hideTimer = null;
	var totalBytesAll = 0;
	var confirmedBytesAll = 0;

	function init() {

		toast = document.querySelector("[data-upload-toast]");
		input = document.querySelector("[data-upload-input]");

		// Both are absent on read-only pages, where uploads are disabled.
		if (null === toast || null === input) {
			return;
		}

		listElement = toast.querySelector("[data-upload-list]");
		overallProgress = toast.querySelector("[data-upload-overall]");
		titleElement = toast.querySelector("[data-upload-title]");

		bindTrigger();
		bindInput();
		bindClose();
		bindPageDrop();
	}

	// bindTrigger wires the upload button(s) to open the native file picker.
	function bindTrigger() {

		var buttons = document.querySelectorAll("[data-upload-open]");

		for (var i = 0; i < buttons.length; i = i + 1) {
			buttons[i].addEventListener("click", onPickClick);
		}
	}

	// onPickClick opens the picker. The data-upload-folder buttons enable folder
	// selection (webkitdirectory); plain ones pick files only.
	function onPickClick(event) {

		var asFolder = "1" === event.currentTarget.getAttribute("data-upload-folder");
		toggleFolderAttribute(asFolder);
		input.click();
	}

	function toggleFolderAttribute(on) {

		if (on) {
			input.setAttribute("webkitdirectory", "");
			input.setAttribute("directory", "");
		} else {
			input.removeAttribute("webkitdirectory");
			input.removeAttribute("directory");
		}
	}

	function bindInput() {

		input.addEventListener("change", function () {
			enqueue(input.files, uploadDir());
			input.value = "";
		});
	}

	function bindClose() {

		var closeButtons = toast.querySelectorAll("[data-upload-close]");

		for (var i = 0; i < closeButtons.length; i = i + 1) {
			closeButtons[i].addEventListener("click", function () {
				hideToast();
				resetRun();
			});
		}
	}

	// bindPageDrop accepts files OR folders dropped anywhere on the window. When
	// the drag carries DataTransferItems (folders), they are walked recursively
	// to collect every file with its relative path; otherwise a plain file list
	// is enqueued directly.
	function bindPageDrop() {

		window.addEventListener("dragover", function (event) {
			if (false === hasFiles(event)) {
				return;
			}
			event.preventDefault();
			highlight(true);
		});

		window.addEventListener("dragleave", function (event) {
			if (null === event.relatedTarget) {
				highlight(false);
			}
		});

		window.addEventListener("drop", function (event) {
			if (false === hasFiles(event)) {
				return;
			}
			event.preventDefault();
			highlight(false);

			var items = event.dataTransfer.items;

			if (null !== items && items.length > 0 && items[0].webkitGetAsEntry) {
				collectFromItems(items, uploadDir());
				return;
			}

			enqueue(event.dataTransfer.files, uploadDir());
		});
	}

	// collectFromItems walks each dropped item. If it is a directory, its tree is
	// read recursively; if it is a file, a task is created from its fullPath.
	function collectFromItems(items, baseDir) {

		var pending = items.length;
		var tasks = [];

		function done() {
			pending = pending - 1;
			if (0 === pending) {
				enqueueTasks(tasks);
			}
		}

		function pushFile(entry) {
			entry.file(function (file) {
				tasks.push(taskFromEntry(file, entry, baseDir));
				done();
			}, function () {
				done();
			});
		}

		function walk(entry) {
			if (entry.isFile) {
				pushFile(entry);
				return;
			}
			if (entry.isDirectory) {
				var reader = entry.createReader();
				readAllEntries(reader, function (children) {
					pending = pending + children.length - 1;
					for (var i = 0; i < children.length; i = i + 1) {
						walk(children[i]);
					}
					if (0 === children.length) {
						done();
					}
				});
				return;
			}
			done();
		}

		for (var i = 0; i < items.length; i = i + 1) {
			var entry = items[i].webkitGetAsEntry();
			if (null === entry) {
				done();
				continue;
			}
			walk(entry);
		}
	}

	// readAllEntries keeps calling readEntries until it returns an empty page,
	// because the API paginates and a single call may miss entries in large dirs.
	function readAllEntries(reader, accumulate) {

		var all = [];

		function page() {
			reader.readEntries(function (entries) {
				if (0 === entries.length) {
					accumulate(all);
					return;
				}
				all = all.concat(Array.prototype.slice.call(entries));
				page();
			}, function () {
				accumulate(all);
			});
		}

		page();
	}

	// taskFromEntry builds a task from a File plus the entry's fullPath. The
	// leading directory segment of fullPath (the dropped folder name) is kept,
	// so dropping "photos/" recreates "photos/" under the current directory.
	function taskFromEntry(file, entry, baseDir) {

		var fullPath = entry.fullPath || file.name;
		var clean = fullPath.replace(/^\/+/, "");
		var slash = clean.lastIndexOf("/");
		var relDir = slash >= 0 ? clean.substring(0, slash) : "";

		return { file: file, dir: joinDir(baseDir, relDir) };
	}

	function hasFiles(event) {

		if (null === event.dataTransfer || null === event.dataTransfer.types) {
			return false;
		}

		var types = event.dataTransfer.types;

		for (var i = 0; i < types.length; i = i + 1) {
			if ("Files" === types[i]) {
				return true;
			}
		}

		return false;
	}

	function highlight(on) {

		var fileList = document.getElementById("file-list");

		if (null === fileList) {
			return;
		}

		if (on) {
			fileList.classList.add("file-drop-active");
		} else {
			fileList.classList.remove("file-drop-active");
		}
	}

	// enqueue turns dropped or picked files into upload tasks. Each task carries
	// a file plus its target directory (the current dir, plus any relative
	// sub-path the browser reported via webkitRelativePath).
	function enqueue(fileList, baseDir) {

		if (0 === fileList.length) {
			return;
		}

		var tasks = [];

		for (var i = 0; i < fileList.length; i = i + 1) {
			tasks.push(makeTask(fileList[i], baseDir));
		}

		startTasks(tasks);
	}

	function enqueueTasks(tasks) {

		if (0 === tasks.length) {
			return;
		}

		startTasks(tasks);
	}

	// makeTask builds a task from a picker File. The folder picker attaches a
	// webkitRelativePath like "photos/a/b.jpg"; the directory portion is used so
	// the server recreates that subtree.
	function makeTask(file, baseDir) {

		var rel = file.webkitRelativePath || "";
		var slash = rel.lastIndexOf("/");
		var relDir = slash >= 0 ? rel.substring(0, slash) : "";

		return { file: file, dir: joinDir(baseDir, relDir) };
	}

	// joinDir composes a base directory with a relative subpath, trimming any
	// leading slashes so the result stays relative to the data root.
	function joinDir(base, rel) {

		if (rel) {
			rel = rel.replace(/^\/+/, "");
		}

		if (base && rel) {
			return base.replace(/\/+$/, "") + "/" + rel;
		}

		return base || "/";
	}

	// CONCURRENCY is the number of files uploaded at once. Files are uploaded in
	// parallel while the chunks within a single file stay sequential, which keeps
	// the request count bounded and matches how most upload UIs behave.
	var CONCURRENCY = 3;

	// active counts how many workers are mid-upload so the run finishes only when
	// every queued file has completed.
	var active = 0;

	function startTasks(tasks) {

		for (var i = 0; i < tasks.length; i = i + 1) {
			queue.push(tasks[i]);
			totalBytesAll = totalBytesAll + tasks[i].file.size;
		}

		cancelHide();
		showToast();
		setTitle("uploading");

		if (false === running) {
			running = true;
			startWorkers();
		}
	}

	// startWorkers launches as many workers as the concurrency limit (and the
	// queue size) allow.
	function startWorkers() {

		while (active < CONCURRENCY && queue.length > 0) {
			active = active + 1;
			runWorker();
		}
	}

	// runWorker pulls one task off the queue, uploads it, then either takes the
	// next task or, when the queue is empty and no workers remain, finishes.
	function runWorker() {

		if (0 === queue.length) {
			active = active - 1;
			if (0 === active) {
				running = false;
				finishRun();
			}
			return;
		}

		var task = queue.shift();
		var file = task.file;
		var item = createItem(displayName(task));

		uploadOneFile(task, item).then(function () {
			markItem(item, "done", statusText("upload.done"));
		}).catch(function () {
			hadFailure = true;
			markItem(item, "failed", statusText("upload.failed"));
		}).then(function () {
			confirmedBytesAll = confirmedBytesAll + file.size;
			updateOverall(0);
			runWorker();
		});
	}
	// displayName shows the relative path (when uploading a folder) so the user
	// can tell where each file is going.
	function displayName(task) {

		if (task.dir && "/" !== task.dir) {
			return task.dir + "/" + task.file.name;
		}

		return task.file.name;
	}

	// finishRun refreshes the listing and either auto-hides the toast (success) or
	// keeps it open with a failure title so the user can see what went wrong.
	function finishRun() {

		refreshFileList();

		if (hadFailure) {
			setTitle("failed");
			return;
		}

		setTitle("done");
		scheduleHide();
	}

	// uploadOneFile runs the init -> chunks -> complete sequence for one task.
	async function uploadOneFile(task, item) {

		var file = task.file;

		markItem(item, "uploading", statusText("upload.uploading"));

		var totalChunks = Math.max(1, Math.ceil(file.size / CHUNK_SIZE));
		var uploadId = await initUpload(task, totalChunks);

		for (var index = 0; index < totalChunks; index = index + 1) {

			var start = index * CHUNK_SIZE;
			var end = Math.min(start + CHUNK_SIZE, file.size);
			var blob = file.slice(start, end);

			await sendChunk(uploadId, index, blob, function (loaded) {
				updateItemProgress(item, file.size, start + loaded);
				updateOverall(start + loaded);
			});
		}

		await completeUpload(uploadId);
		updateItemProgress(item, file.size, file.size);
	}

	function uploadDir() {

		return toast.getAttribute("data-upload-dir") || "/";
	}

	async function initUpload(task, totalChunks) {

		var file = task.file;

		var response = await fetch("/api/upload/init", {
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({
				name: file.name,
				dir: task.dir,
				size: file.size,
				totalChunks: totalChunks
			})
		});

		if (false === response.ok) {
			throw new Error("init failed");
		}

		var data = await response.json();

		return data.uploadId;
	}

	// sendChunk uploads one chunk via XHR so per-chunk progress is observable.
	function sendChunk(uploadId, index, blob, onProgress) {

		return new Promise(function (resolve, reject) {

			var xhr = new XMLHttpRequest();
			var url = "/api/upload/chunk?uploadId=" + encodeURIComponent(uploadId) + "&index=" + index;

			xhr.open("POST", url, true);

			xhr.upload.onprogress = function (event) {
				if (event.lengthComputable) {
					onProgress(event.loaded);
				}
			};

			xhr.onload = function () {
				if (xhr.status >= 200 && xhr.status < 300) {
					resolve();
				} else {
					reject(new Error("chunk failed"));
				}
			};

			xhr.onerror = function () {
				reject(new Error("chunk error"));
			};

			xhr.send(blob);
		});
	}

	async function completeUpload(uploadId) {

		var response = await fetch("/api/upload/complete?uploadId=" + encodeURIComponent(uploadId), {
			method: "POST"
		});

		if (false === response.ok) {
			throw new Error("complete failed");
		}
	}

	function createItem(name) {

		var item = document.createElement("li");
		item.className = "upload-item";

		var head = document.createElement("div");
		head.className = "upload-item-head";

		var nameSpan = document.createElement("span");
		nameSpan.className = "upload-item-name";
		nameSpan.textContent = name;

		var statusSpan = document.createElement("span");
		statusSpan.className = "upload-item-status";
		statusSpan.textContent = statusText("upload.waiting");

		head.appendChild(nameSpan);
		head.appendChild(statusSpan);

		var progress = document.createElement("progress");
		progress.value = 0;
		progress.max = 100;

		item.appendChild(head);
		item.appendChild(progress);
		listElement.appendChild(item);

		return item;
	}

	function updateItemProgress(item, fileSize, uploadedBytes) {

		var progress = item.querySelector("progress");
		var ratio = 100;

		if (fileSize > 0) {
			ratio = Math.round((uploadedBytes / fileSize) * 100);
		}

		progress.value = ratio;
	}

	function updateOverall(inflightBytes) {

		var ratio = 0;

		if (totalBytesAll > 0) {
			ratio = Math.round(((confirmedBytesAll + inflightBytes) / totalBytesAll) * 100);
		}

		if (ratio > 100) {
			ratio = 100;
		}

		overallProgress.value = ratio;
	}

	function markItem(item, state, text) {

		item.classList.remove("uploading", "done", "failed");
		item.classList.add(state);

		var statusSpan = item.querySelector(".upload-item-status");
		statusSpan.textContent = text;
	}

	// setTitle updates the toast heading from a localized state label.
	function setTitle(state) {

		if (null === titleElement) {
			return;
		}

		titleElement.textContent = statusText("upload." + state);
	}

	// statusText reads a localized string from data attributes on the toast so the
	// script stays language-agnostic.
	function statusText(key) {

		var labels = {
			"upload.waiting": toast.getAttribute("data-label-waiting"),
			"upload.uploading": toast.getAttribute("data-label-uploading"),
			"upload.done": toast.getAttribute("data-label-done"),
			"upload.failed": toast.getAttribute("data-label-failed")
		};

		return labels[key] || key;
	}

	function showToast() {

		toast.hidden = false;
	}

	function hideToast() {

		toast.hidden = true;
	}

	function scheduleHide() {

		hideTimer = setTimeout(function () {
			hideToast();
			resetRun();
		}, 2500);
	}

	function cancelHide() {

		if (null !== hideTimer) {
			clearTimeout(hideTimer);
			hideTimer = null;
		}
	}

	// resetRun clears the aggregate counters and the item list for the next run.
	function resetRun() {

		totalBytesAll = 0;
		confirmedBytesAll = 0;
		hadFailure = false;

		if (null !== listElement) {
			listElement.innerHTML = "";
		}

		if (null !== overallProgress) {
			overallProgress.value = 0;
		}
	}

	function refreshFileList() {

		var url = "/api/list?dir=" + encodeURIComponent(uploadDir());

		if (window.htmx) {
			window.htmx.ajax("GET", url, { target: "#file-list", swap: "outerHTML" });
		}
	}

	if (document.readyState === "loading") {
		document.addEventListener("DOMContentLoaded", init);
	} else {
		init();
	}
})();
