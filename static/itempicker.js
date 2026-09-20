(function () {
  "use strict";

  var MAX_RESULTS = 50;
  var catalogPromise = null;

  // Lazily fetch and cache the item catalog (Name, Category, ObjectPath
  // triples) generated from the installed game's own asset manifest. Only
  // fetched once, on first use, not on page load.
  function loadCatalog() {
    if (!catalogPromise) {
      catalogPromise = fetch("/static/items.json").then(function (res) {
        return res.json();
      });
    }
    return catalogPromise;
  }

  function closeDropdown(dropdown) {
    dropdown.hidden = true;
    dropdown.innerHTML = "";
  }

  function openDropdownFor(input, dropdown, items) {
    var query = input.value.trim().toLowerCase();
    var matches = [];
    for (var i = 0; i < items.length && matches.length < MAX_RESULTS; i++) {
      var name = items[i][0];
      if (query === "" || name.toLowerCase().indexOf(query) !== -1) {
        matches.push(items[i]);
      }
    }
    if (matches.length === 0) {
      closeDropdown(dropdown);
      return;
    }
    dropdown.innerHTML = "";
    matches.forEach(function (item) {
      var name = item[0], category = item[1], objectPath = item[2];
      var row = document.createElement("div");
      row.className = "item-picker-option";
      row.setAttribute("role", "option");

      var nameEl = document.createElement("span");
      nameEl.className = "item-picker-name";
      nameEl.textContent = name;

      var catEl = document.createElement("span");
      catEl.className = "item-picker-category";
      catEl.textContent = category;

      row.appendChild(nameEl);
      row.appendChild(catEl);

      row.addEventListener("mousedown", function (e) {
        // mousedown (not click) fires before the input's blur, so the
        // dropdown is still open and we can read/close it cleanly.
        e.preventDefault();
        input.value = objectPath;
        closeDropdown(dropdown);
        input.focus();
      });

      dropdown.appendChild(row);
    });
    dropdown.hidden = false;
  }

  function wireInput(input) {
    if (input.dataset.itemPickerWired) return;
    input.dataset.itemPickerWired = "1";

    var wrapper = input.closest(".item-picker");
    if (!wrapper) return;

    var dropdown = document.createElement("div");
    dropdown.className = "item-picker-dropdown";
    dropdown.hidden = true;
    wrapper.appendChild(dropdown);

    input.addEventListener("focus", function () {
      loadCatalog().then(function (items) {
        openDropdownFor(input, dropdown, items);
      });
    });
    input.addEventListener("input", function () {
      loadCatalog().then(function (items) {
        openDropdownFor(input, dropdown, items);
      });
    });
    input.addEventListener("blur", function () {
      closeDropdown(dropdown);
    });
    input.addEventListener("keydown", function (e) {
      if (e.key === "Escape") {
        closeDropdown(dropdown);
      }
    });
  }

  function wireAll(root) {
    var inputs = root.querySelectorAll("input[data-item-picker]");
    for (var i = 0; i < inputs.length; i++) {
      wireInput(inputs[i]);
    }
  }

  document.addEventListener("DOMContentLoaded", function () {
    wireAll(document);
  });
  // htmx swaps in new tree/leaf fragments (expanding a node, editing a
  // field) without a full page load, so re-scan after every settle.
  document.body.addEventListener("htmx:afterSettle", function (e) {
    wireAll(e.target);
  });
})();
