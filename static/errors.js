(function () {
  "use strict";

  // htmx 1.x refuses to swap a 4xx/5xx response body by default, so a
  // rejected edit/add/remove was a silent no-op in the browser even
  // though the server sent back a perfectly good HTML fragment (every
  // handler's error response is shaped to match its form's existing
  // hx-target/hx-swap -- a full "slot"/"leafRow" fragment with an
  // <span class="error"> inside it, or (for a handful of
  // session/upload-level failures with no specific element to merge
  // into) a small standalone <div class="error">). Force the swap to
  // happen for every response this app's own endpoints return, success
  // or failure alike, so those fragments land normally instead of being
  // discarded.
  document.body.addEventListener("htmx:beforeSwap", function (e) {
    if (e.detail.xhr && e.detail.xhr.status >= 400) {
      e.detail.shouldSwap = true;
      e.detail.isError = false;
    }
  });
})();
