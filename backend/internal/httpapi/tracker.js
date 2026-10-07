// Seomarine Analytics tracker. No cookies, no storage, no dependencies.
(function () {
  var script = document.currentScript;
  var nav = navigator;
  var site = script && script.getAttribute("data-site");
  if (!site || nav.doNotTrack === "1" || window.doNotTrack === "1" || nav.globalPrivacyControl) return;
  var endpoint = new URL("/collect", script.src).href;
  var last = "";

  // send reports a pageview of the current URL, reached from referrer. It
  // skips repeats and #fragment-only changes, which are not new pages.
  function send(referrer) {
    var url = location.href.split("#")[0];
    if (url === last) return;
    last = url;
    // A string body goes as text/plain, which needs no CORS preflight.
    var body = JSON.stringify({ k: site, u: url, r: referrer, w: screen.width });
    if (!(nav.sendBeacon && nav.sendBeacon(endpoint, body))) {
      fetch(endpoint, { method: "POST", body: body, keepalive: true, mode: "no-cors" }).catch(function () {});
    }
  }

  // Single-page apps change pages through the History API; the previous URL
  // is the referrer, so those views count as internal navigation.
  var pushState = history.pushState;
  history.pushState = function () {
    var from = last;
    pushState.apply(this, arguments);
    send(from);
  };
  addEventListener("popstate", function () {
    send(last);
  });
  send(document.referrer);
})();
