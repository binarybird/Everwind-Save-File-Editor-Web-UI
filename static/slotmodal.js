(function () {
  "use strict";

  // Last-resort fallback if an <img> (including placeholder.png itself)
  // fails to load for some reason -- not the normal path. The normal
  // "no icon extracted for this item" case already renders the real
  // placeholder.png image via the template, not this. Picks the
  // fallback's size/class to match whichever <img> failed (the small
  // grid icon vs. the larger modal preview).
  function iconMissing(img) {
    var span = document.createElement("span");
    span.className = img.classList.contains("modal-icon") ? "modal-icon-missing" : "slot-icon-missing";
    span.textContent = "?";
    img.replaceWith(span);
  }
  window.__iconMissing = iconMissing;

  // esc HTML-escapes s for safe interpolation into both element text and
  // double-quoted attribute values. Values here (item names, object
  // paths) come from data-* attributes sourced from the save file, so a
  // crafted BaseData string is a real input, not just a theoretical one
  // -- textContent alone only escapes &/</>, not quote characters, which
  // would otherwise let such a value break out of an attribute like
  // value="...".
  function esc(s) {
    var d = document.createElement("div");
    d.textContent = s == null ? "" : s;
    return d.innerHTML.replace(/"/g, "&#34;").replace(/'/g, "&#39;");
  }

  function iconMarkup(icon, name) {
    var src = "/static/icons/" + esc(icon || "placeholder.png");
    return '<img class="modal-icon" src="' + src + '" alt="' + esc(name) +
      '" onerror="window.__iconMissing(this)">';
  }

  function buildOccupiedModal(slotEl, rowID) {
    var sessionID = slotEl.dataset.sessionId;
    var slotPath = slotEl.dataset.slotPath;
    var itemPath = slotEl.dataset.itemPath;
    var qtyPath = slotEl.dataset.qtyPath;
    var objectPath = slotEl.dataset.objectPath;
    var name = slotEl.dataset.name;
    var icon = slotEl.dataset.icon;
    var quantity = slotEl.dataset.quantity;
    var target = "#slot-" + esc(rowID);

    return (
      '<form method="dialog" class="modal-close-form"><button type="submit" class="modal-close-btn" aria-label="Close">&times;</button></form>' +
      "<h3>Edit item</h3>" +
      '<div class="modal-preview">' + iconMarkup(icon, name) + "</div>" +
      '<form class="modal-item-form" hx-post="/session/' + esc(sessionID) + '/edit" hx-target="' + target + '" hx-swap="outerHTML">' +
        '<label>Item' +
          '<input type="hidden" name="path" value="' + esc(itemPath) + '">' +
          '<input type="hidden" name="slotPath" value="' + esc(slotPath) + '">' +
          '<span class="item-picker">' +
            '<input type="text" name="value" value="' + esc(objectPath) + '" data-item-picker="1" autocomplete="off" class="modal-item-input">' +
          "</span>" +
        "</label>" +
        '<button type="submit">Save item</button>' +
      "</form>" +
      '<form class="modal-qty-form" hx-post="/session/' + esc(sessionID) + '/edit" hx-target="' + target + '" hx-swap="outerHTML">' +
        '<label>Quantity' +
          '<input type="hidden" name="path" value="' + esc(qtyPath) + '">' +
          '<input type="hidden" name="slotPath" value="' + esc(slotPath) + '">' +
          '<input type="number" name="value" value="' + esc(quantity) + '" min="0">' +
        "</label>" +
        '<button type="submit">Save quantity</button>' +
      "</form>"
    );
  }

  function buildEmptyModal(slotEl, rowID) {
    var sessionID = slotEl.dataset.sessionId;
    var slotPath = slotEl.dataset.slotPath;
    var target = "#slot-" + esc(rowID);

    return (
      '<form method="dialog" class="modal-close-form"><button type="submit" class="modal-close-btn" aria-label="Close">&times;</button></form>' +
      "<h3>Add item</h3>" +
      '<form class="modal-add-form" hx-post="/session/' + esc(sessionID) + '/slot/add" hx-target="' + target + '" hx-swap="outerHTML">' +
        '<input type="hidden" name="slotPath" value="' + esc(slotPath) + '">' +
        '<label>Item' +
          '<span class="item-picker">' +
            '<input type="text" name="itemPath" placeholder="Search items…" data-item-picker="1" autocomplete="off" class="modal-item-input">' +
          "</span>" +
        "</label>" +
        '<button type="submit">Add item</button>' +
      "</form>"
    );
  }

  function openModalForSlot(slotEl) {
    var dialog = document.getElementById("slot-modal");
    if (!dialog) return;
    var rowID = slotEl.id.replace(/^slot-/, "");
    var occupied = slotEl.dataset.occupied === "true";

    dialog.innerHTML = occupied ? buildOccupiedModal(slotEl, rowID) : buildEmptyModal(slotEl, rowID);
    if (window.wireItemPickers) window.wireItemPickers(dialog);
    if (window.htmx) window.htmx.process(dialog);
    dialog.showModal();
  }

  function wireSlotClickTargets(root) {
    var buttons = root.querySelectorAll(".slot-click-target");
    for (var i = 0; i < buttons.length; i++) {
      if (buttons[i].dataset.modalWired) continue;
      buttons[i].dataset.modalWired = "1";
      buttons[i].addEventListener("click", function () {
        openModalForSlot(this.closest(".slot"));
      });
    }
  }

  document.addEventListener("DOMContentLoaded", function () {
    wireSlotClickTargets(document);
  });

  // htmx swaps in new tree/inventory fragments (uploading a file,
  // switching tabs) without a full page load, so re-scan after every
  // settle for newly-rendered slots.
  document.body.addEventListener("htmx:afterSettle", function (e) {
    wireSlotClickTargets(e.target);
  });

  // The #slot-modal dialog only exists once a save has been uploaded and
  // the Inventory tab has rendered at least once -- it's part of that
  // htmx-fetched fragment, not the initial page shell. Wiring its
  // close-on-success and backdrop-click behavior can't happen at
  // DOMContentLoaded (the dialog isn't in the DOM yet), so both listeners
  // are delegated on document/document.body instead, which are always
  // present -- no re-wiring needed as the dialog is repeatedly rebuilt.
  document.body.addEventListener("htmx:afterRequest", function (e) {
    if (!e.detail.successful) return;
    var dialog = document.getElementById("slot-modal");
    if (dialog && dialog.open && dialog.contains(e.target)) dialog.close();
  });

  // Click on the dialog's own backdrop closes it -- a <dialog> element's
  // click target during a ::backdrop click is the dialog element itself
  // (not any of its content), so a click whose target IS the dialog means
  // the backdrop was clicked, not something inside it.
  document.addEventListener("click", function (e) {
    if (e.target && e.target.id === "slot-modal" && e.target.open) {
      e.target.close();
    }
  });
})();
