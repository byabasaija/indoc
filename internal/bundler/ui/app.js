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
    '  <button type="button" class="nav-toggle">Menu</button>',
    '  <div class="brand"></div>',
    '  <form class="search" role="search">',
    '    <input type="search" placeholder="Search docs" aria-label="Search docs">',
    '    <div class="results" hidden></div>',
    '  </form>',
    '  <button type="button" class="theme">Theme</button>',
    '</header>',
    '<div class="body">',
    '  <nav class="sidebar"></nav>',
    '  <main class="content"></main>',
    '  <aside class="toc"></aside>',
    '</div>'
  ].join("");
  app.innerHTML = shell;

  var brand = app.querySelector(".brand");
  var nav = app.querySelector(".sidebar");
  var content = app.querySelector(".content");
  var toc = app.querySelector(".toc");
  var input = app.querySelector("input");
  var results = app.querySelector(".results");
  var form = app.querySelector(".search");

  brand.textContent = data.project_name;

  function pagesIn(section) {
    return data.pages.filter(function (page) { return page.section === section; });
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
    var columns = page.kind === "routes" ? ["Method", "Path", "Source"] : ["Name", "Source"];
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
        var badge = document.createElement("span");
        var method = entry.detail || "ANY";
        badge.className = "method method-" + method.toLowerCase();
        badge.textContent = method;
        methodCell.appendChild(badge);
        row.appendChild(methodCell);
      }
      var nameCell = cell(row, entry.name, "path");
      if (entry.description) {
        var desc = document.createElement("div");
        desc.className = "entry-desc";
        desc.textContent = entry.description;
        nameCell.appendChild(desc);
      }
      cell(row, entry.source, "source");
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
    content.appendChild(title);
    if (page.html) {
      var article = document.createElement("article");
      article.innerHTML = page.html;
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
      return [entry.name, entry.detail, entry.description].join(" ");
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

  app.querySelector(".nav-toggle").addEventListener("click", function () {
    document.body.classList.toggle("nav-open");
  });

  document.addEventListener("keydown", function (event) {
    if (event.key === "/" && document.activeElement !== input) {
      event.preventDefault();
      input.focus();
    }
    if (event.key === "Escape") {
      results.hidden = true;
    }
  });

  window.addEventListener("hashchange", show);
  show();
})();
