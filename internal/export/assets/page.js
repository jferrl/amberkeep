// Everything this page can do, with no network and no library.
//
// The expensive case is real: a single conversation in a real archive reaches
// ninety thousand messages. So the searchable text is read out of the document
// once, on the first search, and kept in a plain array afterwards. Nothing is
// stored in the file itself, which would double its size for no benefit.
(function () {
  "use strict";

  var rows = null;      // the elements that can be shown or hidden
  var haystack = null;  // their searchable text, folded once
  var marked = [];      // rows whose text we rewrote to highlight a match
  var matches = [];     // rows matching the current search, in order
  var at = -1;          // where we are within them

  // fold makes a comparison ignore case and accents, so "jose" finds "José".
  function fold(s) {
    s = s.toLowerCase();
    try {
      return s.normalize("NFD").replace(/[̀-ͯ]/g, "");
    } catch (e) {
      return s;
    }
  }

  function collect() {
    if (rows) return;
    rows = Array.prototype.slice.call(document.querySelectorAll("[data-find]"));
    haystack = rows.map(function (row) {
      return fold(row.textContent || "");
    });
  }

  function unmark() {
    for (var i = 0; i < marked.length; i++) {
      var row = marked[i];
      if (row.__original !== undefined) {
        row.innerHTML = row.__original;
        row.__original = undefined;
      }
    }
    marked = [];
  }

  // highlight rewrites the text of a match so the words stand out. It runs only
  // on rows that matched, and only on the first few hundred of them, because
  // rewriting thousands of elements on every keystroke is what makes a search
  // feel broken.
  var highlightLimit = 300;

  function highlight(row, needle) {
    var parts = row.querySelectorAll(".body, .who");
    for (var i = 0; i < parts.length; i++) {
      var el = parts[i];
      var text = el.textContent || "";
      var folded = fold(text);
      if (folded.indexOf(needle) === -1) continue;
      // Folding can change a string's length, and an offset measured in the
      // folded text would then mark the wrong characters. Those are left plain.
      if (folded.length !== text.length) continue;
      if (el.__original === undefined) el.__original = el.innerHTML;

      var out = document.createDocumentFragment();
      var from = 0;
      var hit = folded.indexOf(needle, from);
      while (hit !== -1) {
        out.appendChild(document.createTextNode(text.slice(from, hit)));
        var m = document.createElement("mark");
        m.textContent = text.slice(hit, hit + needle.length);
        out.appendChild(m);
        from = hit + needle.length;
        hit = folded.indexOf(needle, from);
      }
      out.appendChild(document.createTextNode(text.slice(from)));
      el.textContent = "";
      el.appendChild(out);
      marked.push(el);
    }
  }

  function search(term) {
    collect();
    unmark();
    matches = [];
    at = -1;

    var needle = fold(term.trim());
    var days = document.querySelectorAll(".day");

    if (!needle) {
      for (var i = 0; i < rows.length; i++) rows[i].classList.remove("hide");
      for (var d = 0; d < days.length; d++) days[d].classList.remove("hide");
      return { shown: rows.length, total: rows.length, searching: false };
    }

    for (var j = 0; j < rows.length; j++) {
      var hit = haystack[j].indexOf(needle) !== -1;
      rows[j].classList.toggle("hide", !hit);
      if (hit) {
        matches.push(rows[j]);
        if (matches.length <= highlightLimit) highlight(rows[j], needle);
      }
    }
    // Day separators belong to messages that may all be hidden now.
    for (var k = 0; k < days.length; k++) days[k].classList.add("hide");

    return { shown: matches.length, total: rows.length, searching: true };
  }

  function step(forward) {
    if (!matches.length) return;
    at = (at + (forward ? 1 : -1) + matches.length) % matches.length;
    matches[at].scrollIntoView({ block: "center" });
  }

  function ready() {
    var input = document.querySelector(".find input");
    var count = document.querySelector(".find .count");
    var noun = (input && input.dataset.noun) || "messages";
    var timer = null;

    function report(result) {
      if (!count) return;
      if (!result.searching) {
        count.textContent = result.total.toLocaleString() + " " + noun;
        return;
      }
      count.textContent = result.shown === 0
        ? "nothing found"
        : result.shown.toLocaleString() + " of " + result.total.toLocaleString();
    }

    if (input) {
      // A search runs over every message, so it waits for a pause in typing
      // rather than running on each keystroke.
      input.addEventListener("input", function () {
        if (timer) clearTimeout(timer);
        timer = setTimeout(function () {
          report(search(input.value));
        }, 140);
      });
      input.addEventListener("keydown", function (e) {
        if (e.key === "Enter") {
          e.preventDefault();
          if (timer) { clearTimeout(timer); timer = null; report(search(input.value)); }
          step(!e.shiftKey);
        } else if (e.key === "Escape") {
          input.value = "";
          report(search(""));
        }
      });
      var clear = document.querySelector(".find button.clear");
      if (clear) {
        clear.addEventListener("click", function () {
          input.value = "";
          report(search(""));
          input.focus();
        });
      }
    }

    // A slash focuses the search, the way it does everywhere else.
    document.addEventListener("keydown", function (e) {
      if (e.key === "/" && input && document.activeElement !== input) {
        e.preventDefault();
        input.focus();
        input.select();
      }
    });

    // Recovered pictures are small by nature, so clicking one shows it as large
    // as it goes. A photograph that travelled with the archive opens the same way:
    // it is shown at a readable size in the thread, not at the size it was taken.
    var zoom = document.querySelector("dialog.zoom");
    if (zoom) {
      var big = zoom.querySelector("img");
      document.addEventListener("click", function (e) {
        var target = e.target;
        if (
          target &&
          target.classList &&
          (target.classList.contains("preview") || target.classList.contains("shot")) &&
          target.tagName === "IMG"
        ) {
          big.src = target.src;
          big.alt = target.alt;
          zoom.showModal();
        } else if (target === big || target === zoom) {
          zoom.close();
        }
      });
    }

    collect();
    report({ shown: rows.length, total: rows.length, searching: false });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", ready);
  } else {
    ready();
  }
})();
