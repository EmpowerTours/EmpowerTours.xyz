// /start#spec=<base64url(UTF-8 JSON)> arrives from the Claude Project Scoper.
// The spec stays in the URL fragment, so it never reaches server logs; this page only
// shows it and sends it, together with the contact details, to the quote API.
const API = "https://api.empowertours.xyz";

const LABELS = {
  login: {
    none: "No login",
    email: "Email login",
    google: "Google login",
    phone: "Phone login",
  },
  pay: { codi: "CoDi", card: "Card", cash: "Cash", mon: "MON (crypto)" },
  notify: { email: "Email", whatsapp: "WhatsApp", sms: "SMS" },
  ui: { es: "Spanish", en: "English", both: "Spanish and English" },
  device: {
    phone: "Phone (web)",
    desk: "Desktop (web)",
    pwa: "Installable web app",
    native: "Native app",
  },
  stack: {
    auto: "Your choice",
    next: "Next.js",
    go: "Go",
    py: "Python",
    php: "PHP",
    other: "Other",
  },
  have: {
    nothing: "Nothing yet",
    site: "A website",
    repo: "A code repo",
    sheet: "A spreadsheet",
  },
};

function readSpec() {
  const m = /^#spec=([A-Za-z0-9_-]+={0,2})$/.exec(location.hash);
  if (!m || m[1].length > 90000) return null;
  try {
    let b64 = m[1].replace(/-/g, "+").replace(/_/g, "/").replace(/=+$/, "");
    b64 += "=".repeat((4 - (b64.length % 4)) % 4);
    const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
    const spec = JSON.parse(
      new TextDecoder("utf-8", { fatal: true }).decode(bytes),
    );
    if (
      !spec ||
      typeof spec !== "object" ||
      Array.isArray(spec) ||
      spec.v !== 1 ||
      typeof spec.name !== "string" ||
      !spec.name.trim()
    )
      return null;
    return spec;
  } catch {
    return null;
  }
}

const str = (v) => (typeof v === "string" ? v.trim() : "");
const arr = (v) => (Array.isArray(v) ? v : []);

function addRow(dl, label, value) {
  if (value == null || value === "" || (Array.isArray(value) && !value.length))
    return;
  const dt = document.createElement("dt");
  dt.textContent = label;
  const dd = document.createElement("dd");
  if (Array.isArray(value)) {
    const ul = document.createElement("ul");
    for (const v of value) {
      const li = document.createElement("li");
      li.textContent = v;
      ul.appendChild(li);
    }
    dd.appendChild(ul);
  } else {
    dd.textContent = value;
  }
  dl.append(dt, dd);
}

function items(list) {
  return arr(list)
    .filter((it) => it && str(it.t))
    .map((it) => str(it.t) + (it.later ? " (later)" : ""));
}

function names(list, labels) {
  return arr(list)
    .map((v) => labels[v] || String(v))
    .join(", ");
}

function renderSummary(spec) {
  document.getElementById("specName").textContent = str(spec.name);
  const dl = document.getElementById("specList");
  addRow(dl, "Problem", str(spec.problem));
  addRow(dl, "Main thing it does", str(spec.core));
  addRow(dl, "Who uses it", arr(spec.roles).map(str).filter(Boolean));
  addRow(dl, "Screens", items(spec.screens));
  addRow(dl, "Data it keeps", items(spec.data));
  addRow(dl, "Connects to", items(spec.integ));
  addRow(dl, "Login", LABELS.login[spec.login] || str(spec.login));
  addRow(
    dl,
    "Payments",
    [names(spec.pay, LABELS.pay), str(spec.payrule)]
      .filter(Boolean)
      .join(" · "),
  );
  addRow(
    dl,
    "Notifications",
    [names(spec.notify, LABELS.notify), str(spec.notifyrule)]
      .filter(Boolean)
      .join(" · "),
  );
  addRow(dl, "Language", LABELS.ui[spec.ui] || str(spec.ui));
  addRow(dl, "Runs on", LABELS.device[spec.device] || str(spec.device));
  addRow(dl, "Looks like", str(spec.like));
  addRow(dl, "Brand", str(spec.brand));
  addRow(
    dl,
    "Tech stack",
    spec.stack === "other" && str(spec.stackother)
      ? str(spec.stackother)
      : LABELS.stack[spec.stack] || str(spec.stack),
  );
  addRow(
    dl,
    "Already have",
    [names(spec.have, LABELS.have), str(spec.havelink)]
      .filter(Boolean)
      .join(" · "),
  );
  addRow(dl, "Done when", arr(spec.done).map(str).filter(Boolean));
  addRow(dl, "Not now", str(spec.notnow));
  addRow(dl, "When", str(spec.when));
}

function say(msg, ok) {
  const p = document.getElementById("quote-msg");
  p.hidden = false;
  p.className = "formmsg " + (ok ? "ok" : "err");
  p.textContent = msg;
}

function wire(spec) {
  const form = document.getElementById("quote");
  if (str(spec.email))
    form.elements.email.value = str(spec.email).slice(0, 200);
  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const fd = new FormData(form);
    const btn = form.querySelector("button[type=submit]");
    const name = String(fd.get("name") || "").trim();
    const email = String(fd.get("email") || "").trim();
    const country = String(fd.get("country") || "");
    if (!name || !email) return say("Add your name and email.");
    if (!form.elements.email.checkValidity())
      return say("Check your email address.");
    if (!country) return say("Choose where this will be invoiced.");
    if (!fd.get("consent"))
      return say("Tick the box to agree to the privacy notice.");
    btn.disabled = true;
    btn.textContent = "Sending…";
    try {
      const r = await fetch(API + "/api/v1/site/quote-request", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          spec,
          name,
          email,
          country,
          website: String(fd.get("website") || ""),
        }),
      });
      const j = await r.json().catch(() => ({}));
      if (!r.ok)
        throw new Error(j.error || "Your request didn't send. Try again.");
      form.hidden = true;
      const done = document.createElement("p");
      done.className = "formmsg ok";
      done.setAttribute("role", "status");
      done.textContent =
        "We got it. You'll get a quote by email within 2 business days.";
      form.after(done);
    } catch (err) {
      say(err.message || "Your request didn't send. Try again.");
      btn.disabled = false;
      btn.textContent = "Request my quote";
    }
  });
}

const spec = readSpec();
if (spec) {
  renderSummary(spec);
  wire(spec);
  document.getElementById("haveSpec").hidden = false;
} else {
  document.getElementById("nospec").hidden = false;
}
