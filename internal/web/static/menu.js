// "New" dropdown menu: picking an item ("folder" or "file") swaps the menu list
// for that item's name input; closing the menu resets it back to the list. Uses
// event delegation so it keeps working after htmx swaps the file list, and a
// native <details> element for the open/close state.

(function () {
	"use strict";

	// showForm hides the choice list and reveals the picked action's form.
	function showForm(menu, kind) {

		var list = menu.querySelector("[data-menu-list]");
		var forms = menu.querySelectorAll("[data-menu-form]");

		if (null !== list) {
			list.hidden = true;
		}

		for (var i = 0; i < forms.length; i = i + 1) {

			var matches = forms[i].getAttribute("data-menu-form") === kind;
			forms[i].hidden = false === matches;

			if (matches) {
				var input = forms[i].querySelector("[data-menu-input]");
				if (null !== input) {
					input.focus();
				}
			}
		}
	}

	// resetMenu returns the menu to its initial choice-list state.
	function resetMenu(menu) {

		var list = menu.querySelector("[data-menu-list]");
		var forms = menu.querySelectorAll("[data-menu-form]");

		if (null !== list) {
			list.hidden = false;
		}

		for (var i = 0; i < forms.length; i = i + 1) {
			forms[i].hidden = true;
		}
	}

	function onClick(event) {

		// A menu item was picked: reveal its form.
		var pick = event.target.closest("[data-new-pick]");
		if (null !== pick) {
			var owner = pick.closest("[data-newmenu]");
			if (null !== owner) {
				showForm(owner, pick.getAttribute("data-new-pick"));
			}
			return;
		}

		// A click anywhere outside an open menu closes it.
		var open = document.querySelectorAll("[data-newmenu][open]");
		for (var i = 0; i < open.length; i = i + 1) {
			if (false === open[i].contains(event.target)) {
				open[i].open = false;
			}
		}
	}

	// onToggle resets a menu back to its choice list whenever it closes. The
	// toggle event does not bubble, so it is captured at the document level.
	function onToggle(event) {

		var menu = event.target;

		if (false === menu.matches("[data-newmenu]")) {
			return;
		}

		if (false === menu.open) {
			resetMenu(menu);
		}
	}

	document.addEventListener("click", onClick);
	document.addEventListener("toggle", onToggle, true);
})();
