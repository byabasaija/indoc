(function () {
  var data = window.INDOC_DOCS;
  var app = document.getElementById("app");
  if (!data || !data.pages || !data.pages.length) {
    app.textContent = "This folder has no indoc payload. Run indoc bundle.";
    return;
  }

  document.title = data.project_name;

  var shell = [
    '<header class="top">',
    '  <div class="top-start">',
    '    <button type="button" class="nav-toggle" aria-label="Open navigation" aria-expanded="false" aria-controls="docs-nav"><span></span><span></span><span></span></button>',
    '    <a class="brand" href="#/overview"><span class="brand-mark" aria-hidden="true">i</span><span class="brand-name"></span></a>',
    '    <span class="top-divider" aria-hidden="true"></span>',
    '    <span class="top-context">Documentation</span>',
    '  </div>',
    '  <form class="search" role="search">',
    '    <svg class="search-icon" aria-hidden="true" viewBox="0 0 20 20"><circle cx="8.5" cy="8.5" r="5.5"></circle><path d="m13 13 4 4"></path></svg>',
    '    <input type="search" placeholder="Search documentation" aria-label="Search documentation" autocomplete="off">',
    '    <kbd class="search-shortcut" aria-hidden="true">/</kbd>',
    '    <div class="results" hidden></div>',
    '  </form>',
    '  <button type="button" class="theme" aria-label="Toggle color theme" title="Toggle color theme">',
    '    <svg aria-hidden="true" viewBox="0 0 20 20"><circle cx="10" cy="10" r="3.5"></circle><path d="M10 1.75v2M10 16.25v2M18.25 10h-2M3.75 10h-2m14.08-5.83-1.42 1.42M5.59 14.41l-1.42 1.42m11.66 0-1.42-1.42M5.59 5.59 4.17 4.17"></path></svg>',
    '    <span class="theme-label">Theme</span>',
    '  </button>',
    '</header>',
    '<div class="body">',
    '  <nav id="docs-nav" class="sidebar" aria-label="Documentation navigation"></nav>',
    '  <main class="content"></main>',
    '  <aside class="toc" aria-label="On this page"></aside>',
    '</div>'
  ].join("");
  app.innerHTML = shell;

  var nav = app.querySelector(".sidebar");
  var content = app.querySelector(".content");
  var toc = app.querySelector(".toc");
  var input = app.querySelector("input");
  var results = app.querySelector(".results");
  var form = app.querySelector(".search");

  app.querySelector(".brand-name").textContent = data.project_name;
  var brandMark = app.querySelector(".brand-mark");
  if (data.appearance && data.appearance.logo) {
    var logo = document.createElement("img");
    logo.src = data.appearance.logo;
    logo.alt = "";
    brandMark.classList.add("has-logo");
    brandMark.textContent = "";
    brandMark.appendChild(logo);
  } else {
    var initial = data.project_name.trim().charAt(0);
    brandMark.textContent = initial ? initial.toUpperCase() : "D";
  }
  if (data.appearance && data.appearance.colors) {
    var colorPattern = /^#[0-9a-f]{6}$/i;
    var colors = data.appearance.colors;
    if (colorPattern.test(colors.primary || "")) {
      document.documentElement.style.setProperty("--accent", colors.primary);
      document.documentElement.style.setProperty("--accent-strong", colors.primary);
    }
    if (colorPattern.test(colors.accent || "")) {
      document.documentElement.style.setProperty("--accent-2", colors.accent);
    }
  }

  function pagesIn(section) {
    return data.pages.filter(function (page) { return page.section === section; });
  }

  function addGuideCards(article) {
    var guides = pagesIn("guides").slice(0, 4);
    if (!guides.length) return;

    var launchpad = document.createElement("section");
    launchpad.className = "guide-launchpad";
    launchpad.setAttribute("aria-label", "Start with a guide");

    var heading = document.createElement("div");
    heading.className = "guide-launchpad-heading";
    var label = document.createElement("div");
    label.className = "guide-launchpad-label";
    label.textContent = "START HERE";
    var title = document.createElement("h2");
    title.className = "guide-launchpad-title";
    title.textContent = "Choose your next step";
    heading.appendChild(label);
    heading.appendChild(title);
    launchpad.appendChild(heading);

    var grid = document.createElement("div");
    grid.className = "guide-grid";
    guides.forEach(function (page, index) {
      var link = document.createElement("a");
      link.className = "guide-card";
      link.href = hrefFor(page);

      var top = document.createElement("div");
      top.className = "guide-card-top";
      var number = document.createElement("span");
      number.className = "guide-card-number";
      number.textContent = index < 9 ? "0" + (index + 1) : String(index + 1);
      var arrow = document.createElement("span");
      arrow.className = "guide-card-arrow";
      arrow.setAttribute("aria-hidden", "true");
      arrow.textContent = "↗";
      top.appendChild(number);
      top.appendChild(arrow);

      var heading = document.createElement("strong");
      heading.className = "guide-card-title";
      heading.textContent = page.title;
      link.appendChild(top);
      link.appendChild(heading);

      var preview = document.createElement("div");
      preview.innerHTML = page.html || "";
      var paragraph = preview.querySelector("p");
      if (paragraph && paragraph.textContent.trim()) {
        var summary = document.createElement("span");
        summary.className = "guide-card-summary";
        var text = paragraph.textContent.trim();
        summary.textContent = text.length > 112 ? text.slice(0, 109).trim() + "..." : text;
        link.appendChild(summary);
      }

      var action = document.createElement("span");
      action.className = "guide-card-action";
      action.textContent = "Open guide";
      link.appendChild(action);
      grid.appendChild(link);
    });
    launchpad.appendChild(grid);

    var intro = article.querySelector("p");
    if (intro) {
      intro.classList.add("lede");
      article.insertBefore(launchpad, intro.nextSibling);
    } else {
      article.insertBefore(launchpad, article.firstChild);
    }
  }

  function hrefFor(page) {
    if (page.section === "guides") return "#/guides/" + encodeURIComponent(page.id);
    if (page.section === "reference") return "#/reference/" + encodeURIComponent(page.id);
    return "#/overview";
  }

  function addGroup(label, items) {
    if (!items.length) return;
    var heading = document.createElement("div");
    heading.className = "group";
    heading.textContent = label;
    nav.appendChild(heading);
    items.forEach(function (page) {
      var link = document.createElement("a");
      link.href = hrefFor(page);
      link.textContent = page.title;
      link.dataset.section = page.section;
      link.dataset.id = page.id;
      nav.appendChild(link);
    });
  }

  addGroup("Start", pagesIn("overview"));
  addGroup("Guides", pagesIn("guides"));
  addGroup("Reference", pagesIn("reference"));

  function find(section, id) {
    for (var i = 0; i < data.pages.length; i++) {
      var page = data.pages[i];
      if (page.section === section && page.id === id) return page;
    }
    return data.pages[0];
  }

  function parseHash() {
    var parts = location.hash.replace(/^#\/?/, "").split("/").filter(Boolean);
    if (parts[0] === "guides" && parts[1]) {
      return { section: "guides", id: decodeURIComponent(parts[1]) };
    }
    if (parts[0] === "reference" && parts[1]) {
      return { section: "reference", id: decodeURIComponent(parts[1]) };
    }
    return { section: "overview", id: "overview" };
  }

  function cell(row, text, className) {
    var td = document.createElement("td");
    td.className = className;
    td.textContent = text || "";
    row.appendChild(td);
    return td;
  }

  function referenceTable(page) {
    var wrap = document.createElement("div");
    wrap.className = "ref-wrap";
    var table = document.createElement("table");
    table.className = "ref";
    var head = document.createElement("tr");
    var columns = page.kind === "routes" ? ["Method", "Path", "Source"]
      : page.kind === "commands" ? ["Command", "Arguments", "Source"]
      : ["Name", "Details", "Source"];
    columns.forEach(function (name) {
      var th = document.createElement("th");
      th.textContent = name;
      head.appendChild(th);
    });
    table.appendChild(head);

    page.entries.forEach(function (entry) {
      var row = document.createElement("tr");
      if (page.kind === "routes") {
        var methodCell = document.createElement("td");
        (entry.detail || "ANY").split(/,\s*/).forEach(function (method) {
          var badge = document.createElement("span");
          badge.className = "method method-" + method.toLowerCase();
          badge.textContent = method;
          methodCell.appendChild(badge);
        });
        row.appendChild(methodCell);
      }
      var nameCell = cell(row, entry.name, "path");
      var notes = [];
      if (page.kind === "routes" && entry.handler) notes.push(entry.handler);
      if (page.kind === "commands" && entry.handler) notes.push(entry.handler);
      if (entry.description) notes.push(entry.description);
      notes.forEach(function (text, i) {
        var desc = document.createElement("div");
        desc.className = i === 0 && entry.handler ? "entry-handler" : "entry-desc";
        desc.textContent = text;
        nameCell.appendChild(desc);
      });
      if (page.kind !== "routes") cell(row, entry.detail || "", "detail");
      var sourceCell = cell(row, entry.url ? "" : entry.source, "source");
      if (entry.url) {
        var a = document.createElement("a");
        a.href = entry.url;
        a.textContent = entry.source;
        a.rel = "noopener";
        sourceCell.appendChild(a);
      }
      table.appendChild(row);
    });
    wrap.appendChild(table);
    return wrap;
  }

  function renderToc(page) {
    toc.innerHTML = "";
    if (!page.headings || !page.headings.length) {
      toc.hidden = true;
      return;
    }
    toc.hidden = false;
    var label = document.createElement("div");
    label.className = "group";
    label.textContent = "On this page";
    toc.appendChild(label);
    page.headings.forEach(function (heading) {
      var link = document.createElement("a");
      link.href = hrefFor(page);
      link.textContent = heading.text;
      link.style.paddingLeft = Math.max(0, heading.level - 2) * 12 + "px";
      link.addEventListener("click", function (event) {
        var target = document.getElementById(heading.id);
        if (!target) return;
        event.preventDefault();
        target.scrollIntoView({ behavior: "smooth", block: "start" });
      });
      toc.appendChild(link);
    });
  }

  function setActive(page) {
    var links = nav.querySelectorAll("a");
    for (var i = 0; i < links.length; i++) {
      var on = links[i].dataset.section === page.section && links[i].dataset.id === page.id;
      links[i].classList.toggle("active", on);
    }
  }

  function render(page) {
    content.innerHTML = "";
    var title = document.createElement("h1");
    title.textContent = page.title;
    var kicker = document.createElement("div");
    kicker.className = "page-kicker";
    kicker.textContent = page.section === "reference" ? "REFERENCE" : page.section === "guides" ? "GUIDE" : "OVERVIEW";
    content.appendChild(kicker);
    content.appendChild(title);
    if (page.html) {
      var article = document.createElement("article");
      article.innerHTML = page.html;
      if (page.section === "overview") addGuideCards(article);
      content.appendChild(article);
    }
    if (page.entries && page.entries.length) {
      content.appendChild(referenceTable(page));
    }
    renderToc(page);
    setActive(page);
    content.scrollTop = 0;
    document.body.classList.remove("nav-open");
  }

  function show() {
    var loc = parseHash();
    render(find(loc.section, loc.id));
  }

  function entryText(page) {
    if (!page.entries) return "";
    return page.entries.map(function (entry) {
      return [entry.name, entry.detail, entry.handler, entry.description].join(" ");
    }).join(" ");
  }

  form.addEventListener("submit", function (event) { event.preventDefault(); });
  input.addEventListener("input", function () {
    var query = input.value.trim().toLowerCase();
    results.innerHTML = "";
    if (!query) {
      results.hidden = true;
      return;
    }
    var hits = data.pages.filter(function (page) {
      var haystack = (page.title + " " + (page.text || "") + " " + entryText(page)).toLowerCase();
      return haystack.indexOf(query) !== -1;
    }).slice(0, 8);
    if (!hits.length) {
      results.hidden = true;
      return;
    }
    hits.forEach(function (page) {
      var button = document.createElement("button");
      button.type = "button";
      var section = document.createElement("span");
      section.className = "result-section";
      section.textContent = page.section;
      button.appendChild(section);
      button.appendChild(document.createTextNode(page.title));
      button.addEventListener("click", function () {
        results.hidden = true;
        input.value = "";
        location.hash = hrefFor(page);
      });
      results.appendChild(button);
    });
    results.hidden = false;
  });

  app.querySelector(".theme").addEventListener("click", function () {
    var next = document.documentElement.getAttribute("data-theme") === "dark" ? "light" : "dark";
    document.documentElement.setAttribute("data-theme", next);
    try { localStorage.setItem("indoc-theme", next); } catch (e) {}
  });

  app.querySelector(".nav-toggle").addEventListener("click", function (event) {
    var open = document.body.classList.toggle("nav-open");
    event.currentTarget.setAttribute("aria-expanded", String(open));
  });

  document.addEventListener("keydown", function (event) {
    if (event.key === "/" && document.activeElement !== input) {
      event.preventDefault();
      input.focus();
    }
    if (event.key === "Escape") {
      results.hidden = true;
      document.body.classList.remove("nav-open");
      app.querySelector(".nav-toggle").setAttribute("aria-expanded", "false");
    }
  });

  window.addEventListener("hashchange", show);
  show();
})();
