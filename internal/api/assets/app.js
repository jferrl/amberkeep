// The viewer, in the browser. No library, no build step, no network beyond this
// machine: the page fetches from the program that served it and from nowhere else.
//
// A conversation is loaded from its end and grows upwards as somebody scrolls,
// because the largest one in a real archive is ninety thousand messages and asking
// for all of it would neither arrive nor render.
(function () {
  "use strict";

  // What the index wraps a match in. Control characters, because no message
  // contains one, so nothing somebody wrote can be mistaken for a mark.
  var MARK_OPEN = String.fromCharCode(2);
  var MARK_CLOSE = String.fromCharCode(3);

  var state = { chat: null, before: null, loading: false, searching: false };

  function el(tag, className, text) {
    var node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined && text !== null) node.textContent = text;
    return node;
  }

  function get(path) {
    return fetch(path, { credentials: "same-origin" }).then(function (response) {
      if (!response.ok) throw new Error(response.status + " " + response.statusText);
      return response.json();
    });
  }

  function count(n) { return (n || 0).toLocaleString(); }

  function when(iso, withDate) {
    if (!iso) return "";
    var d = new Date(iso);
    if (isNaN(d)) return "";
    return withDate
      ? d.toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" }) +
        " " + d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })
      : d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  }

  function day(iso) {
    var d = new Date(iso);
    if (isNaN(d)) return "Unknown date";
    return d.toLocaleDateString(undefined, {
      weekday: "long", day: "numeric", month: "long", year: "numeric",
    });
  }

  // ---- the conversation list -------------------------------------------------

  function showChats(chats) {
    var list = document.getElementById("list");
    list.textContent = "";

    if (!chats.length) {
      list.appendChild(el("p", "empty", "No conversation by that name."));
      return;
    }

    var ul = el("ul", "chats");
    chats.forEach(function (chat) {
      var li = el("li");
      var a = el("a");
      a.href = "#";
      a.dataset.jid = chat.address;

      var name = el("span", "name", chat.name);
      var about = [];
      if (chat.kind !== "direct") about.push(chat.kind);
      if (chat.last_message_at) {
        about.push(new Date(chat.last_message_at).toLocaleDateString(undefined, {
          day: "numeric", month: "short", year: "numeric",
        }));
      }
      if (about.length) {
        name.appendChild(document.createElement("br"));
        name.appendChild(el("span", "sub", about.join(" - ")));
      }

      a.appendChild(name);
      a.appendChild(el("span", "n", count(chat.message_count)));
      a.addEventListener("click", function (e) {
        e.preventDefault();
        openChat(chat);
      });
      li.appendChild(a);
      ul.appendChild(li);
    });
    list.appendChild(ul);
  }

  function markCurrent() {
    var links = document.querySelectorAll(".side .list a");
    for (var i = 0; i < links.length; i++) {
      links[i].classList.toggle("on", state.chat && links[i].dataset.jid === state.chat.address);
    }
  }

  // ---- one conversation ------------------------------------------------------

  function openChat(chat, scrollTo) {
    state.chat = chat;
    state.before = null;
    markCurrent();
    document.body.classList.remove("browsing");

    document.getElementById("who").textContent = chat.name;
    var about = [count(chat.message_count) + " messages"];
    if (chat.participants && chat.participants.length) {
      about.push(chat.participants.length + " members");
    }
    document.getElementById("detail").textContent = about.join(" - ");

    var thread = document.getElementById("thread");
    thread.textContent = "";
    thread.appendChild(el("div", "inner"));

    return loadMore(true).then(function () {
      if (scrollTo) highlightAround(scrollTo);
    });
  }

  function loadMore(first) {
    if (state.loading || !state.chat) return Promise.resolve();
    state.loading = true;

    var url = "/api/chats/" + encodeURIComponent(state.chat.address) + "/messages";
    if (state.before) url += "?before=" + encodeURIComponent(state.before);

    return get(url).then(function (page) {
      var thread = document.getElementById("thread");
      var inner = thread.querySelector(".inner");
      var button = inner.querySelector(".more");
      if (button) button.remove();

      var fragment = document.createDocumentFragment();
      var previous = null;
      page.messages.forEach(function (m) {
        var date = day(m.sent_at);
        if (date !== previous) {
          var separator = el("div", "day");
          separator.appendChild(el("span", null, date));
          fragment.appendChild(separator);
          previous = date;
        }
        fragment.appendChild(renderMessage(m));
      });

      if (page.before) {
        var more = el("button", "more", "Earlier messages");
        more.addEventListener("click", function () { loadMore(false); });
        fragment.insertBefore(more, fragment.firstChild);
      }

      var height = thread.scrollHeight;
      inner.insertBefore(fragment, inner.firstChild);
      state.before = page.before || null;

      if (first) {
        thread.scrollTop = thread.scrollHeight;
      } else {
        // Keep the message somebody was reading where it was on the screen.
        thread.scrollTop += thread.scrollHeight - height;
      }
      state.loading = false;
    }).catch(function (err) {
      state.loading = false;
      report(err);
    });
  }

  function renderMessage(m) {
    var article = el("article", "msg" + (m.from_me ? " mine" : "") + (m.kind === "system" ? " notice" : ""));
    article.dataset.sent = m.sent_at;
    var bubble = el("div", "bubble");

    if (m.kind !== "system") {
      var head = el("div");
      head.appendChild(el("span", "who", m.from_me ? "You" : (m.sender_name || m.sender || "Unknown")));
      head.appendChild(el("span", "when", when(m.sent_at)));
      bubble.appendChild(head);
    }

    if (m.reply_to) {
      var quote = el("blockquote");
      quote.appendChild(el("span", "who", m.reply_to.from_me ? "You" : (m.reply_to.sender_name || "Unknown")));
      quote.appendChild(el("div", "body", m.reply_to.text || ("<" + m.reply_to.kind + ">")));
      bubble.appendChild(quote);
    }

    if (m.attachment && m.attachment.preview_base64) {
      var img = el("img", "preview");
      img.loading = "lazy";
      img.src = "data:image/jpeg;base64," + m.attachment.preview_base64;
      img.alt = "recovered preview";
      bubble.appendChild(img);
      bubble.appendChild(el("div", "recovered",
        "recovered preview - the file itself is not in this archive"));
    }

    // The rendered line describes what the message was when it was not words. It is
    // shown only when it says something the text does not.
    if (m.rendered && m.rendered !== m.text) {
      bubble.appendChild(el("div", "note", m.rendered));
    }
    if (m.text && m.kind !== "poll") {
      bubble.appendChild(el("div", "body", m.text));
    }

    if (m.poll) {
      var poll = el("div", "poll");
      (m.poll.options || []).forEach(function (option) {
        var row = el("div", "opt");
        row.appendChild(el("span", null, option.name));
        row.appendChild(el("span", "votes", option.votes === 1 ? "1 vote" : option.votes + " votes"));
        poll.appendChild(row);
      });
      bubble.appendChild(poll);
    }

    if (m.link) {
      var card = el("div", "card");
      if (m.link.title) card.appendChild(el("div", "title", m.link.title));
      if (m.link.description) card.appendChild(el("div", null, m.link.description));
      if (m.link.url) card.appendChild(el("div", "url", m.link.url));
      bubble.appendChild(card);
    }

    if (m.reactions && m.reactions.length) {
      var rx = el("div", "rx");
      m.reactions.forEach(function (r) {
        rx.appendChild(el("span", null, r.emoji + " " + (r.from_me ? "You" : (r.sender_name || ""))));
      });
      bubble.appendChild(rx);
    }

    var tags = [];
    if (m.edited_at) tags.push("edited");
    if (m.forwarded) tags.push("forwarded");
    if (m.starred) tags.push("starred");
    if (tags.length) bubble.appendChild(el("div", "tags", tags.join(" - ")));

    article.appendChild(bubble);
    return article;
  }

  // After jumping to a search result, show which message it was.
  function highlightAround(sentAt) {
    var thread = document.getElementById("thread");
    var wanted = thread.querySelector('[data-sent="' + sentAt + '"]');
    if (wanted) {
      wanted.scrollIntoView({ block: "center" });
      wanted.querySelector(".bubble").style.outline = "2px solid var(--accent)";
    } else if (state.before) {
      // It is further back than the first page; keep fetching until it appears.
      loadMore(false).then(function () { highlightAround(sentAt); });
    }
  }

  // ---- searching everything --------------------------------------------------

  function search(term) {
    if (!term.trim()) {
      state.searching = false;
      document.getElementById("found").textContent = "";
      return loadChats("");
    }

    return get("/api/search?q=" + encodeURIComponent(term) + "&limit=100").then(function (result) {
      state.searching = true;
      var list = document.getElementById("list");
      list.textContent = "";
      document.getElementById("found").textContent =
        result.total ? count(result.total) + " matches" : "nothing found";

      var box = el("div", "results");
      result.hits.forEach(function (hit) {
        var button = el("button", "hit");
        button.type = "button";
        var head = el("div");
        head.appendChild(el("span", "who", hit.chat));
        head.appendChild(el("span", "when", when(hit.sent_at, true)));
        button.appendChild(head);
        button.appendChild(marked(hit.snippet));
        button.addEventListener("click", function () { jumpTo(hit); });
        box.appendChild(button);
      });
      list.appendChild(box);
    }).catch(report);
  }

  // The index marks a match with two control characters, which cannot appear in a
  // message. Turning them into elements here is what keeps a message's own text
  // from ever being treated as markup: every piece becomes a text node.
  function marked(snippet) {
    var line = el("div", "line");
    (snippet || "").split(MARK_OPEN).forEach(function (part, i) {
      if (i === 0) {
        line.appendChild(document.createTextNode(part));
        return;
      }
      var halves = part.split(MARK_CLOSE);
      line.appendChild(el("mark", null, halves[0]));
      line.appendChild(document.createTextNode(halves.slice(1).join("")));
    });
    return line;
  }

  function jumpTo(hit) {
    get("/api/chats?limit=1000").then(function (result) {
      var chat = result.chats.filter(function (c) { return c.address === hit.chat_jid; })[0];
      if (chat) openChat(chat, hit.sent_at);
    }).catch(report);
  }

  // ---- wiring ----------------------------------------------------------------

  function loadChats(q) {
    return get("/api/chats?q=" + encodeURIComponent(q || "")).then(function (result) {
      showChats(result.chats);
      markCurrent();
    }).catch(report);
  }

  function report(err) {
    var list = document.getElementById("list");
    list.textContent = "";
    list.appendChild(el("p", "empty", String(err && err.message ? err.message : err)));
  }

  function ready() {
    get("/api/archive").then(function (archive) {
      document.getElementById("title").textContent = archive.title;
      document.getElementById("about").textContent =
        count(archive.conversations) + " conversations, " + count(archive.messages) + " messages";
      if (!archive.searchable) {
        var box = document.getElementById("q");
        box.placeholder = "Search conversation names";
        box.dataset.namesOnly = "1";
      }
    }).catch(report);

    loadChats("");

    var box = document.getElementById("q");
    var timer = null;
    box.addEventListener("input", function () {
      if (timer) clearTimeout(timer);
      timer = setTimeout(function () {
        if (box.dataset.namesOnly) {
          loadChats(box.value);
        } else {
          search(box.value);
        }
      }, 180);
    });
    document.getElementById("clear").addEventListener("click", function () {
      box.value = "";
      loadChats("");
      document.getElementById("found").textContent = "";
      box.focus();
    });

    document.getElementById("thread").addEventListener("scroll", function (e) {
      if (e.target.scrollTop < 80 && state.before) loadMore(false);
    });

    document.addEventListener("keydown", function (e) {
      if (e.key === "/" && document.activeElement !== box) {
        e.preventDefault();
        box.focus();
        box.select();
      }
      if (e.key === "Escape") document.body.classList.add("browsing");
    });

    var zoom = document.querySelector("dialog.zoom");
    var big = zoom.querySelector("img");
    document.addEventListener("click", function (e) {
      if (e.target.classList && e.target.classList.contains("preview")) {
        big.src = e.target.src;
        zoom.showModal();
      } else if (e.target === big || e.target === zoom) {
        zoom.close();
      }
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", ready);
  } else {
    ready();
  }
})();
