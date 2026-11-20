// No framework, no build step.
const $ = (id) => document.getElementById(id);

async function api(path, opts) {
  const res = await fetch(path, opts);
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error || ("HTTP " + res.status));
  return body;
}

function live(ok, text) {
  $("dot").classList.toggle("on", ok);
  $("status").textContent = text;
}

function renderState(st) {
  $("count").textContent = st.count;
  $("requests").textContent = st.requests;
  $("where").textContent = st.file;
  if (st.last) {
    const a = st.last;
    const t = new Date(a.at).toLocaleTimeString();
    $("last").textContent = "last " + a.kind + " at " + t + (a.error ? (" · error: " + a.error) : "");
  }
}

function renderNotes(notes) {
  const ul = $("notes");
  ul.textContent = "";
  if (!notes.length) {
    const li = document.createElement("li");
    li.className = "empty";
    li.textContent = $("q").value ? "nothing matches" : "nothing yet — add one above";
    ul.appendChild(li);
    return;
  }
  for (const n of notes) {
    const li = document.createElement("li");
    const body = document.createElement("div");
    body.className = "txt";
    body.textContent = n.text;
    const meta = document.createElement("div");
    meta.className = "meta";
    meta.textContent = new Date(n.at).toLocaleString() + " · " + n.id;
    for (const tag of n.tags || []) {
      const s = document.createElement("span");
      s.className = "tag";
      s.textContent = tag;
      meta.appendChild(s);
    }
    const del = document.createElement("button");
    del.className = "del";
    del.textContent = "delete";
    del.onclick = () => remove(n.id);
    const right = document.createElement("div");
    right.appendChild(del);
    li.append(body, meta, right);
    ul.appendChild(li);
  }
}

async function loadNotes() {
  try {
    const out = await api("/api/notes?q=" + encodeURIComponent($("q").value));
    renderNotes(out.notes || []);
    $("resultNote").textContent = out.count + " shown";
  } catch (e) {
    $("resultNote").textContent = "load failed: " + e.message;
  }
}

async function add(text, tags) {
  await api("/api/notes", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ text, tags }),
  });
  await poll();
}

async function remove(id) {
  await api("/api/notes/" + encodeURIComponent(id), { method: "DELETE" });
  await poll();
}

async function poll() {
  try {
    renderState(await api("/api/state"));
    live(true, "live");
    await loadNotes();
  } catch (e) {
    live(false, "gateway unreachable");
  }
}

$("addForm").onsubmit = async (ev) => {
  ev.preventDefault();
  const text = $("text").value.trim();
  if (!text) return;
  const tags = $("tags").value.split(/\s+/).filter(Boolean);
  try {
    await add(text, tags);
    $("text").value = "";
    $("tags").value = "";
  } catch (e) {
    alert("add failed: " + e.message);
  }
};

let typing = null;
$("q").oninput = () => {
  clearTimeout(typing);
  typing = setTimeout(loadNotes, 250);
};

poll();
setInterval(poll, 5000);
