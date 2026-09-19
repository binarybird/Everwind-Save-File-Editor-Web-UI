(function () {
  "use strict";

  var zone = document.getElementById("upload-zone");
  var input = document.getElementById("savefile");
  var form = document.getElementById("upload-form");
  if (!zone || !input || !form) return;

  ["dragenter", "dragover"].forEach(function (evt) {
    zone.addEventListener(evt, function (e) {
      e.preventDefault();
      e.stopPropagation();
      zone.classList.add("dragover");
    });
  });

  ["dragleave", "dragend", "drop"].forEach(function (evt) {
    zone.addEventListener(evt, function (e) {
      e.preventDefault();
      e.stopPropagation();
      zone.classList.remove("dragover");
    });
  });

  zone.addEventListener("drop", function (e) {
    var files = e.dataTransfer && e.dataTransfer.files;
    if (files && files.length > 0) {
      input.files = files;
      htmx.trigger(form, "submit");
    }
  });

  input.addEventListener("change", function () {
    if (input.files && input.files.length > 0) {
      htmx.trigger(form, "submit");
    }
  });
})();
