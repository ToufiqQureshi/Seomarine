// Uplumo Analytics tracker. No cookies, no storage, no dependencies.
(function () {
  var script = document.currentScript;
  var nav = navigator;
  var site = script && script.getAttribute("data-site");
  if (!site || nav.doNotTrack === "1" || window.doNotTrack === "1" || nav.globalPrivacyControl) return;
  var endpoint = new URL("/collect", script.src).href;
  var last = "";

  // post sends one hit. A string body goes as text/plain, which needs no
  // CORS preflight; sendBeacon survives the page unloading on a link click.
  function post(data) {
    var body = JSON.stringify(data);
    if (!(nav.sendBeacon && nav.sendBeacon(endpoint, body))) {
      fetch(endpoint, { method: "POST", body: body, keepalive: true, mode: "no-cors" }).catch(function () {});
    }
  }

  // send reports a pageview of the current URL, reached from referrer. It
  // skips repeats and #fragment-only changes, which are not new pages.
  function send(referrer) {
    var url = location.href.split("#")[0];
    if (url === last) return;
    last = url;
    post({ k: site, u: url, r: referrer, w: screen.width });
  }

  // track reports a custom event, such as a button click, on this page.
  function track(name) {
    name = String(name || "").trim().slice(0, 64);
    if (name) post({ k: site, u: location.href.split("#")[0], w: screen.width, n: name });
  }

  // Any element with data-uplumo-event="Name" is tracked when clicked, with
  // no code: <button data-uplumo-event="Signup">. Code can call
  // window.uplumo("Name") instead.
  addEventListener(
    "click",
    function (e) {
      var el = e.target && e.target.closest && e.target.closest("[data-uplumo-event]");
      if (el) track(el.getAttribute("data-uplumo-event"));
    },
    true
  );
  window.uplumo = track;

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
