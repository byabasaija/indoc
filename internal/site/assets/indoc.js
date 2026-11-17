// indoc site behavior. Pages are complete without it; this adds the theme
// toggle, mobile navigation, search, copy buttons, syntax highlighting,
// diagrams, the active table-of-contents entry, and the GitHub star count.
(function () {
  "use strict";
  var root = document.body.getAttribute("data-root") || "";
  var doc = document.documentElement;

  function dark() {
    var t = doc.dataset.theme;
    return t ? t === "dark" : window.matchMedia("(prefers-color-scheme: dark)").matches;
  }

  // Theme toggle
  var themeButton = document.querySelector("[data-theme-toggle]");
  if (themeButton) {
    themeButton.addEventListener("click", function () {
      var next = dark() ? "light" : "dark";
      doc.dataset.theme = next;
      try { localStorage.setItem("indoc-theme", next); } catch (e) {}
      renderDiagrams(true);
    });
  }

  // Mobile navigation
  var menu = document.querySelector("[data-menu-toggle]");
  if (menu) {
    menu.addEventListener("click", function () {
      var open = document.body.classList.toggle("menu-open");
      menu.setAttribute("aria-expanded", open ? "true" : "false");
    });
  }

  // Copy buttons on code blocks
  var copyIcon = '<svg viewBox="0 0 16 16" aria-hidden="true"><rect x="5" y="5" width="9" height="9" rx="2"/><path d="M11 5V3.5A1.5 1.5 0 0 0 9.5 2h-6A1.5 1.5 0 0 0 2 3.5v6A1.5 1.5 0 0 0 3.5 11H5"/></svg>';
  var checkIcon = '<svg viewBox="0 0 16 16" aria-hidden="true"><path d="m3 8.5 3 3 7-7"/></svg>';
  function copyText(text) {
    if (navigator.clipboard && window.isSecureContext) return navigator.clipboard.writeText(text);
    return new Promise(function (resolve, reject) {
      var area = document.createElement("textarea");
      area.value = text;
      area.setAttribute("readonly", "");
      area.style.position = "fixed";
      area.style.opacity = "0";
      document.body.appendChild(area);
      area.select();
      try { document.execCommand("copy") ? resolve() : reject(); } catch (e) { reject(e); }
      document.body.removeChild(area);
    });
  }
  document.querySelectorAll(".prose pre > code").forEach(function (code) {
    var pre = code.parentNode;
    if (/\blanguage-mermaid\b/.test(code.className)) return;
    var wrap = document.createElement("div");
    wrap.className = "code-block";
    pre.parentNode.insertBefore(wrap, pre);
    wrap.appendChild(pre);
    var button = document.createElement("button");
    button.type = "button";
    button.className = "copy";
    button.setAttribute("aria-label", "Copy code");
    button.innerHTML = copyIcon + "<span>Copy</span>";
    button.addEventListener("click", function () {
      copyText(code.innerText.replace(/\n$/, "")).then(function () {
        button.classList.add("copied");
        button.innerHTML = checkIcon + "<span>Copied</span>";
        setTimeout(function () {
          button.classList.remove("copied");
          button.innerHTML = copyIcon + "<span>Copy</span>";
        }, 1600);
      }, function () {
        button.querySelector("span").textContent = "Press Ctrl+C";
      });
    });
    wrap.appendChild(button);
  });

  // Syntax highlighting (Prism, bundled; manual mode)
  var aliases = { sh: "bash", shell: "bash", zsh: "bash", console: "bash", py: "python", js: "javascript", ts: "typescript", yml: "yaml", html: "markup", xml: "markup", golang: "go", dockerfile: "docker", md: "markdown" };
  if (window.Prism) {
    document.querySelectorAll('.prose pre > code[class*="language-"]').forEach(function (code) {
      var m = code.className.match(/language-([\w-]+)/);
      var lang = m && (aliases[m[1]] || m[1]);
      if (lang && Prism.languages[lang]) {
        code.classList.add("language-" + lang);
        Prism.highlightElement(code);
      }
    });
  }

  // Diagrams (Mermaid, only shipped on pages that use it)
  var diagrams = [];
  document.querySelectorAll(".prose pre > code.language-mermaid").forEach(function (code) {
    var pre = code.parentNode;
    var div = document.createElement("div");
    div.className = "mermaid";
    div.setAttribute("data-source", code.textContent);
    pre.parentNode.replaceChild(div, pre);
    diagrams.push(div);
  });
  function renderDiagrams(again) {
    if (!window.mermaid || !diagrams.length) return;
    diagrams.forEach(function (d) {
      d.removeAttribute("data-processed");
      d.textContent = d.getAttribute("data-source");
    });
    window.mermaid.initialize({ startOnLoad: false, theme: dark() ? "dark" : "default", securityLevel: "strict" });
    window.mermaid.run({ nodes: diagrams });
  }
  renderDiagrams(false);

  // Active table-of-contents entry
  var tocLinks = document.querySelectorAll(".toc a");
  if (tocLinks.length && "IntersectionObserver" in window) {
    var byId = {};
    tocLinks.forEach(function (a) { byId[decodeURIComponent(a.hash.slice(1))] = a; });
    var visible = {};
    var observer = new IntersectionObserver(function (entries) {
      entries.forEach(function (e) { visible[e.target.id] = e.isIntersecting; });
      var first = null;
      document.querySelectorAll(".prose h2[id], .prose h3[id]").forEach(function (h) {
        if (!first && visible[h.id]) first = h.id;
      });
      if (first) {
        tocLinks.forEach(function (a) { a.classList.remove("active"); });
        if (byId[first]) byId[first].classList.add("active");
      }
    }, { rootMargin: "-64px 0px -60% 0px" });
    document.querySelectorAll(".prose h2[id], .prose h3[id]").forEach(function (h) { observer.observe(h); });
  }

  // GitHub star count, cached for the session
  var gh = document.querySelector("[data-repo]");
  if (gh && window.fetch) {
    var repo = gh.getAttribute("data-repo");
    var show = function (n) {
      var el = gh.querySelector(".stars");
      if (!el || typeof n !== "number" || !(n > 0)) return;
      el.textContent = n >= 1000 ? (n / 1000).toFixed(n >= 10000 ? 0 : 1) + "k" : String(n);
      el.hidden = false;
    };
    var cached = null;
    try { cached = sessionStorage.getItem("indoc-stars-" + repo); } catch (e) {}
    if (cached !== null) {
      show(Number(cached));
    } else {
      fetch("https://api.github.com/repos/" + repo).then(function (r) { return r.ok ? r.json() : null; }).then(function (d) {
        if (!d) return;
        try { sessionStorage.setItem("indoc-stars-" + repo, d.stargazers_count); } catch (e) {}
        show(d.stargazers_count);
      }).catch(function () {});
    }
  }

  // Search
  var box = document.querySelector("[data-search]");
  var index = window.INDOC_SEARCH || [];
  if (!box) return;
  var input = box.querySelector("input");
  var results = box.querySelector(".search-results");
  var selected = -1;

  function escapeHTML(s) {
    return s.replace(/[&<>"]/g, function (c) { return { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]; });
  }
  function highlight(text, terms) {
    var out = escapeHTML(text);
    terms.forEach(function (t) {
      if (t.length < 2) return;
      out = out.replace(new RegExp("(" + t.replace(/[.*+?^${}()|[\]\\]/g, "\\$&") + ")", "gi"), "<mark>$1</mark>");
    });
    return out;
  }
  function snippet(text, terms) {
    var lower = text.toLowerCase();
    var at = -1;
    for (var i = 0; i < terms.length && at < 0; i++) at = lower.indexOf(terms[i]);
    if (at < 0) return text.slice(0, 140);
    var start = Math.max(0, at - 50);
    return (start > 0 ? "…" : "") + text.slice(start, start + 160) + "…";
  }
  function score(entry, terms) {
    var total = 0;
    var title = entry.t.toLowerCase();
    var heads = (entry.h || []).join(" ").toLowerCase();
    var text = (entry.x || "").toLowerCase();
    for (var i = 0; i < terms.length; i++) {
      var t = terms[i];
      var s = 0;
      if (title === t) s += 30;
      if (title.indexOf(t) >= 0) s += 12;
      if (heads.indexOf(t) >= 0) s += 6;
      if (text.indexOf(t) >= 0) s += 2;
      if (!s) return 0; // every term must match
      total += s;
    }
    return total;
  }
  function search() {
    var q = input.value.trim().toLowerCase();
    selected = -1;
    if (!q) { results.hidden = true; results.innerHTML = ""; return; }
    var terms = q.split(/\s+/).filter(Boolean);
    var hits = index.map(function (e) { return { e: e, s: score(e, terms) }; })
      .filter(function (h) { return h.s > 0; })
      .sort(function (a, b) { return b.s - a.s; })
      .slice(0, 8);
    if (!hits.length) {
      results.innerHTML = '<div class="empty">No results for “' + escapeHTML(input.value.trim()) + "”</div>";
    } else {
      results.innerHTML = hits.map(function (h) {
        return '<a role="option" href="' + escapeHTML(root + h.e.u) + '">' +
          '<span class="r-title">' + highlight(h.e.t, terms) + "</span>" +
          (h.e.s ? ' <span class="r-section">· ' + escapeHTML(h.e.s) + "</span>" : "") +
          '<span class="r-snippet">' + highlight(snippet(h.e.x || "", terms), terms) + "</span></a>";
      }).join("");
    }
    results.hidden = false;
  }
  function move(delta) {
    var items = results.querySelectorAll("a");
    if (!items.length) return;
    selected = (selected + delta + items.length) % items.length;
    items.forEach(function (a, i) { a.setAttribute("aria-selected", i === selected ? "true" : "false"); });
    items[selected].scrollIntoView({ block: "nearest" });
  }
  input.addEventListener("input", search);
  input.addEventListener("focus", function () { if (input.value) search(); });
  input.addEventListener("keydown", function (e) {
    if (e.key === "ArrowDown") { e.preventDefault(); move(1); }
    else if (e.key === "ArrowUp") { e.preventDefault(); move(-1); }
    else if (e.key === "Enter") {
      var items = results.querySelectorAll("a");
      var target = items[selected >= 0 ? selected : 0];
      if (target) window.location.href = target.href;
    } else if (e.key === "Escape") { input.blur(); results.hidden = true; }
  });
  document.addEventListener("click", function (e) { if (!box.contains(e.target)) results.hidden = true; });
  document.addEventListener("keydown", function (e) {
    var tag = (e.target.tagName || "").toLowerCase();
    if (e.key === "/" && tag !== "input" && tag !== "textarea") { e.preventDefault(); input.focus(); }
  });
})();
