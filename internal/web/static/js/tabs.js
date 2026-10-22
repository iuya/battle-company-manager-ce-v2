// The tab bar of a campaign scrolls sideways on a phone: bring the current tab into view when the page is shown.
(function () {
  function show() {
    document.querySelectorAll('nav.tab-bar').forEach(nav => {
      const cur = nav.querySelector('[aria-current="page"]');
      if (cur) nav.scrollLeft = Math.max(0, cur.offsetLeft - (nav.clientWidth - cur.offsetWidth) / 2);
    });
  }
  document.addEventListener('DOMContentLoaded', show);
  document.addEventListener('htmx:afterSettle', show);
})();
