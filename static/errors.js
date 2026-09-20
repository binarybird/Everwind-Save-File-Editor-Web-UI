(function () {
  "use strict";

  // htmx 1.x only swaps 2xx (and not 204) responses -- a 4xx/5xx response
  // body (this app's small `<div class="error">...</div>` fragments from
  // renderError) is otherwise silently discarded, so a rejected edit/add/
  // remove looks like nothing happened at all. Surface it instead: show
  // the error fragment right next to whatever triggered the request.
  document.body.addEventListener("htmx:responseError", function (e) {
    var xhr = e.detail.xhr;
    var trigger = e.detail.elt;
    if (!trigger || !xhr) return;

    var container = trigger.closest("form") || trigger.parentElement || trigger;
    var existing = container.querySelector(".htmx-error-fragment");
    if (existing) existing.remove();

    var wrapper = document.createElement("div");
    wrapper.className = "htmx-error-fragment";
    wrapper.innerHTML = xhr.response;
    container.appendChild(wrapper);
  });
})();
