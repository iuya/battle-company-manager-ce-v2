// A fragment swapped in by htmx is only wired (its hx-get, hx-post and hx-trigger attributes read) when htmx settles the swap, a
// timer of 20 ms after it; in between the new buttons are on screen and a click on them does nothing: no request leaves the page.
// A menu entry clicked in that gap (a slow device, a loaded machine) would be lost, with the window left open. So the new content is wired as
// soon as it is in the page: htmx.process is idempotent, so the later pass of htmx itself finds nothing left to do.
(function () {
  document.addEventListener('htmx:afterSwap', e => {
    if (!window.htmx) return;
    const d = e.detail || {};
    let el = d.target || e.target;
    if (el && !el.isConnected && el.id) el = document.getElementById(el.id); // an outerHTML swap replaced the target by its twin
    if (el && el.isConnected) htmx.process(el);
  });
})();
