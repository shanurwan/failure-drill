// faildrill course page.
//
// A replay, not a simulation and not a live cluster. The terminal prints what
// the real command printed, and the scene is driven by the same record: which
// pod was primary when, which pods existed, how many requests were answered or
// failed in each second, and how slow the slowest were. Two things are drawn
// only to make the picture readable and are not measured: the individual dots
// (a record keeps counts per second, not paths), and which gateway a dot visits.

import * as THREE from "three";
import { OrbitControls } from "three/addons/controls/OrbitControls.js";
import { RoundedBoxGeometry } from "three/addons/geometries/RoundedBoxGeometry.js";
import { LESSONS, outcomeFor, takeaways } from "./lessons.js";

const $ = (id) => document.getElementById(id);
const clamp = (x, a, b) => Math.min(b, Math.max(a, x));
const reducedMotion = matchMedia("(prefers-reduced-motion: reduce)").matches;

const store = {
  get(key, fallback) { try { return JSON.parse(localStorage.getItem(key)) ?? fallback; } catch { return fallback; } },
  set(key, value) { try { localStorage.setItem(key, JSON.stringify(value)); } catch { /* private window */ } },
};

const S = {
  bundle: null,
  byKey: new Map(),      // "drill/fault" -> runs, oldest first
  lesson: 0,
  done: new Set(store.get("faildrill.done", [])),
  pick: {},              // "drill/fault" -> which run to replay
  run: null,             // the run being replayed
  t: 0, playing: false, speed: 6, finished: false,
  printed: 0, lines: [], block: null,
};

// ---------- data ---------------------------------------------------------------------

const keyOf = (run) => `${run.drill}/${run.fault}`;

async function load() {
  const res = await fetch("data/runs.json");
  if (!res.ok) throw new Error(`data/runs.json: ${res.status}`);
  S.bundle = await res.json();
  for (const run of S.bundle.runs) {
    if (!S.byKey.has(keyOf(run))) S.byKey.set(keyOf(run), []);
    S.byKey.get(keyOf(run)).push(run);
  }
  const first = S.bundle.runs[0];
  const day = new Date(first.started).toLocaleDateString("en-GB", { day: "numeric", month: "long", year: "numeric", timeZone: "UTC" });
  const image = (S.bundle.preflight.match(/tyk-gateway:(v[\d.]+)/) || [])[1] || "";
  $("provenance").textContent =
    `${S.bundle.runs.length} recorded runs from ${day}, on a single-node k3s lab with Tyk Gateway ${image} ` +
    `from Tyk's open-source Helm chart. Nothing on this page talks to a cluster.`;
}

function secondsOf(run, name) {
  const target = run.targets.find((t) => t.name === name);
  return target ? target.summary.seconds : [];
}

function bucket(run, name, t) {
  const second = Math.floor(t);
  return secondsOf(run, name).find((s) => s.t === second) || { t: second, ok: 0, fail: 0, p99_ms: 0 };
}

// What the system looked like at t, rebuilt from the record's events.
function stateAt(run, t) {
  const redis = {};
  for (const name of ["redis-0", "redis-1", "redis-2"]) redis[name] = { role: "replica", frozen: false, stale: false };
  let primary = run.facts.primary_before || "redis-0";
  if (run.drill !== "redis-primary") {
    // Gateway drills do not look at Redis; show the usual arrangement.
    primary = "redis-0";
  }
  redis[primary].role = "primary";

  // Gateways that existed before the fault: the ones an event mentions before
  // any event creates them, padded to the count the record gives.
  const created = new Set();
  const initial = [];
  for (const e of run.events) {
    const m = e.text.match(/^(\S+) (created|ready|no longer ready|gone)$/);
    if (!m || e.kind !== "pod") continue;
    if (m[2] === "created") created.add(m[1]);
    else if (!created.has(m[1]) && !initial.includes(m[1])) initial.push(m[1]);
  }
  const count = Number(run.facts.ready_before || run.facts.pods || 2);
  while (initial.length < count) initial.push(`gateway-${"ab"[initial.length] || initial.length}`);
  const gateways = initial.map((name) => ({ name, status: "ready" }));

  let phase = t < 0 ? "baseline" : "after";
  for (const e of run.events) {
    if (e.t > t) break;
    let m;
    if ((m = e.text.match(/^(\S+) stopped answering$/))) { redis[m[1]].frozen = true; }
    else if ((m = e.text.match(/^(\S+) promoted to primary$/))) {
      for (const r of Object.values(redis)) if (r.role === "primary") { r.role = "replica"; r.stale = true; }
      redis[m[1]].role = "primary"; redis[m[1]].stale = false;
    }
    else if ((m = e.text.match(/^(\S+) is answering again, still as primary$/))) { redis[m[1]].frozen = false; }
    else if ((m = e.text.match(/^(\S+) is back, now as a replica$/))) { redis[m[1]].frozen = false; redis[m[1]].stale = false; }
    else if ((m = e.text.match(/^(\S+) created$/))) { gateways.push({ name: m[1], status: "starting" }); }
    else if ((m = e.text.match(/^(\S+) ready$/))) { const g = gateways.find((x) => x.name === m[1]); if (g) g.status = "ready"; }
    else if ((m = e.text.match(/^(\S+) no longer ready$/))) { const g = gateways.find((x) => x.name === m[1]); if (g) g.status = "stopping"; }
    else if ((m = e.text.match(/^(\S+) gone$/))) { const i = gateways.findIndex((x) => x.name === m[1]); if (i >= 0) gateways.splice(i, 1); }
  }
  // A stale primary is one that has been replaced but has not been told yet:
  // only meaningful once it answers again.
  for (const r of Object.values(redis)) if (r.stale && r.frozen) r.stale = false;
  return { redis, gateways, phase };
}

// ---------- lessons ------------------------------------------------------------------

function renderLessons() {
  const list = $("lessons");
  list.innerHTML = "";
  LESSONS.forEach((lesson, i) => {
    const li = document.createElement("li");
    const b = document.createElement("button");
    b.type = "button";
    b.innerHTML = `<span class="num">${S.done.has(lesson.id) ? "✓" : i + 1}</span>${lesson.short}`;
    if (S.done.has(lesson.id)) b.classList.add("done");
    if (i === S.lesson) b.setAttribute("aria-current", "step");
    b.addEventListener("click", () => openLesson(i));
    li.append(b);
    list.append(li);
  });
}

function openLesson(i, { focus = true } = {}) {
  S.lesson = i;
  store.set("faildrill.lesson", i);
  const lesson = LESSONS[i];
  const el = $("lesson");
  el.innerHTML = `
    <p class="kicker">Lesson ${i + 1} of ${LESSONS.length}</p>
    <h2>${lesson.title}</h2>
    ${lesson.body.map((p) => `<p>${p}</p>`).join("")}
    <h3>Type this in the terminal</h3>
    ${lesson.commands.map((c) => `<code class="type">${c}</code>`).join("")}
    <p class="aside">${lesson.aside}</p>
    <div class="outcome" id="outcome" hidden></div>`;
  renderLessons();
  if (S.done.has(lesson.id)) showOutcome(lesson, lastRunFor(lesson));
  if (focus) $("term-in").focus({ preventScroll: true });
  // Show the lab at rest for this lesson until a command is typed.
  if (!S.playing) idleScene(lesson);
}

function lastRunFor(lesson) {
  if (!lesson.key) return null;
  const runs = S.byKey.get(lesson.key) || [];
  return runs[S.pick[lesson.key] || 0] || null;
}

function showOutcome(lesson, run) {
  const box = $("outcome");
  if (!box) return;
  const { figures, paragraphs } = outcomeFor(lesson, run, S.bundle, S.byKey);
  const next = S.lesson < LESSONS.length - 1
    ? `<div class="next"><button type="button" id="next">Next: ${LESSONS[S.lesson + 1].short}</button></div>`
    : `<div class="next"><a class="button" href="#results" style="margin-top:0">See all the results</a></div>`;
  box.innerHTML = `
    <h3>What just happened</h3>
    ${figures.length ? `<div class="figures">${figures.map((f) => `<div class="figure"><b>${f.value}</b><span>${f.label}</span></div>`).join("")}</div>` : ""}
    ${paragraphs.map((p) => `<p>${p}</p>`).join("")}
    ${next}`;
  box.hidden = false;
  const btn = $("next");
  if (btn) btn.addEventListener("click", () => { openLesson(S.lesson + 1); $("course").scrollIntoView({ block: "start" }); });
}

function markDone(lesson, run) {
  S.done.add(lesson.id);
  store.set("faildrill.done", [...S.done]);
  renderLessons();
  showOutcome(lesson, run);
}

// ---------- terminal -----------------------------------------------------------------

const out = $("term-out");
const input = $("term-in");
const history = [];
let historyAt = 0;

function print(text, cls) {
  const div = document.createElement("div");
  if (cls) div.className = cls;
  div.textContent = text === "" ? " " : text;
  out.append(div);
  const body = $("term-body");
  body.scrollTop = body.scrollHeight;
  return div;
}

function classify(line) {
  if (/^WARN/.test(line)) return "warn";
  if (/fault injected|stopped answering|gone$|no longer ready/.test(line)) return "fault";
  if (/^\s+(see it happen|[A-Z].*\.)$/.test(line) || /^(record|report) /.test(line)) return "note";
  return "";
}

const HELP = [
  "This terminal replays recorded runs. Commands it knows:",
  "",
  "  kubectl get pods -n faildrill-lab",
  "  faildrill preflight -n faildrill-lab",
  "  faildrill run redis-primary --fault hang",
  "  faildrill run redis-primary --fault crash",
  "  faildrill run gateway-pod --fault crash",
  "  faildrill run gateway-pod --fault evict",
  "  faildrill run rolling-restart",
  "  faildrill              the tool's own usage text",
  "  clear, help",
  "",
  "Tab completes the current lesson's command. Up and down walk your history.",
  "On a real cluster the run commands also need --url for each address to call;",
  "here the lab's two addresses are filled in for you.",
];

function execute(raw) {
  const line = raw.trim();
  print(line, "cmd");
  if (!line) return;
  history.push(line); historyAt = history.length;
  const words = line.split(/\s+/);

  if (S.playing) stopReplay();

  if (line === "help") return HELP.forEach((l) => print(l, "note"));
  if (line === "clear") { out.innerHTML = ""; return; }

  if (words[0] === "kubectl") {
    if (words[1] === "get" && /^pods?$/.test(words[2] || "")) {
      S.bundle.pods.split("\n").forEach((l) => print(l));
      return maybeDone("pods");
    }
    return print("This replay only recorded: kubectl get pods -n faildrill-lab", "note");
  }

  if (words[0] !== "faildrill") {
    return print(`${words[0]}: not part of this replay. Type help to see what is.`, "note");
  }
  const sub = words[1];
  if (!sub || sub === "help" || sub === "-h" || sub === "--help") return S.bundle.usage.split("\n").forEach((l) => print(l));
  if (sub === "version") return print("faildrill (recorded build)");
  if (sub === "preflight") {
    S.bundle.preflight.trimEnd().split("\n").forEach((l) => print(l, classify(l)));
    return maybeDone("preflight");
  }
  if (sub !== "run") return print(`unknown command "${sub}"`, "warn");

  const drill = words[2];
  const drills = ["redis-primary", "gateway-pod", "rolling-restart"];
  if (!drills.includes(drill)) {
    return print(`faildrill: unknown drill "${drill || ""}": use redis-primary, gateway-pod or rolling-restart`, "warn");
  }
  const flag = words.indexOf("--fault");
  let fault = flag >= 0 ? words[flag + 1] : (words.find((w) => w.startsWith("--fault=")) || "").slice(8);
  if (drill === "rolling-restart") fault = "rollout";
  else if (!fault) fault = "crash";
  const key = `${drill}/${fault}`;
  const runs = S.byKey.get(key);
  if (!runs) {
    const allowed = drill === "redis-primary" ? "crash or hang" : "crash or evict";
    return print(`faildrill: unknown --fault "${fault}" for ${drill}: use ${allowed}`, "warn");
  }
  startReplay(runs[S.pick[key] || 0]);
}

function maybeDone(what) {
  const lesson = LESSONS[S.lesson];
  if (lesson.id === "preflight" && what === "preflight") markDone(lesson, null);
}

input.addEventListener("keydown", (e) => {
  if (e.key === "Enter") { const v = input.value; input.value = ""; execute(v); }
  else if (e.key === "Tab") {
    e.preventDefault();
    const want = LESSONS[S.lesson].commands.find((c) => c.startsWith(input.value)) || LESSONS[S.lesson].commands.at(-1);
    input.value = want;
  }
  else if (e.key === "ArrowUp") { if (historyAt > 0) input.value = history[--historyAt]; e.preventDefault(); }
  else if (e.key === "ArrowDown") { input.value = historyAt < history.length - 1 ? history[++historyAt] : ((historyAt = history.length), ""); e.preventDefault(); }
  else if (e.key === "l" && e.ctrlKey) { e.preventDefault(); out.innerHTML = ""; }
});
$("term-body").addEventListener("click", () => { if (!getSelection().toString()) input.focus({ preventScroll: true }); });

// "Type it for me" types the lesson's last command, a letter at a time, and runs it.
$("hint").addEventListener("click", () => {
  const command = LESSONS[S.lesson].commands.at(-1);
  input.focus({ preventScroll: true });
  if (reducedMotion) { input.value = ""; execute(command); return; }
  let i = 0;
  input.value = "";
  const tick = () => {
    input.value = command.slice(0, ++i);
    if (i < command.length) setTimeout(tick, 22 + Math.random() * 38);
    else setTimeout(() => { input.value = ""; execute(command); }, 260);
  };
  tick();
});

// ---------- replay engine ------------------------------------------------------------

function parseOutput(run) {
  // Lines before the first timed line print at once; each timed line prints
  // when the clock reaches it; everything after the last timed line prints
  // with it.
  let at = -Infinity;
  return run.output.replace(/\n+$/, "").split("\n").map((text) => {
    const m = text.match(/^\s+([+-]\d+\.\d)s\s/);
    if (m) at = parseFloat(m[1]);
    return { text, at };
  });
}

function startReplay(run) {
  S.run = run;
  S.lines = parseOutput(run);
  S.printed = 0;
  S.block = document.createElement("div");
  out.append(S.block);
  S.t = -run.baseline_s;
  S.finished = false;
  S.playing = true;
  renderRunPick();
  drawStrip();
  scene.reset();
}

function stopReplay() {
  if (S.playing && S.block) {
    const div = document.createElement("div");
    div.className = "note";
    div.textContent = "(replay stopped)";
    S.block.append(div);
  }
  S.playing = false;
}

function printUpTo(t) {
  while (S.printed < S.lines.length && S.lines[S.printed].at <= t) {
    const { text } = S.lines[S.printed++];
    const div = document.createElement("div");
    const cls = classify(text);
    if (cls) div.className = cls;
    div.textContent = text === "" ? " " : text;
    S.block.append(div);
  }
  const body = $("term-body");
  body.scrollTop = body.scrollHeight;
}

function seek(t) {
  if (!S.run) return;
  S.t = clamp(t, -S.run.baseline_s, S.run.observe_s);
  S.block.innerHTML = "";
  S.printed = 0;
  S.finished = false;
  S.playing = true;
  printUpTo(S.t);
  scene.reset();
}

// The replay slows to real time wherever something is happening, so a failure
// that lasted under a second is not gone before you can see it.
function busyAt(run, t) {
  for (const target of run.targets) {
    const b = bucket(run, target.name, t);
    if (b.fail > 0 || b.p99_ms > 150) return true;
  }
  return run.events.some((e) => e.kind !== "phase" && Math.abs(e.t - t) < 1.2);
}

function advance(dt) {
  if (!S.playing || !S.run) return;
  const run = S.run;
  const speed = busyAt(run, S.t) ? Math.min(S.speed, 1.5) : S.speed;
  S.t += dt * speed;
  if (S.t >= run.observe_s) {
    S.t = run.observe_s;
    printUpTo(Infinity);
    S.playing = false;
    S.finished = true;
    const lesson = LESSONS.find((l) => l.key === keyOf(run));
    if (lesson) {
      if (LESSONS[S.lesson] !== lesson) openLesson(LESSONS.indexOf(lesson), { focus: false });
      markDone(lesson, run);
    }
  } else {
    printUpTo(S.t);
  }
  const chip = $("clock");
  const sign = S.t < 0 ? "−" : "+";
  chip.textContent = S.finished ? "run complete" : `${S.t < 0 ? "baseline" : "after the fault"}  ${sign}${Math.abs(S.t).toFixed(1)} s`;
  chip.classList.toggle("fault", S.t >= 0 && !S.finished);
  drawStrip();
}

function renderRunPick() {
  const el = $("runpick");
  el.innerHTML = "";
  el.hidden = !S.run;
  if (!S.run) return;
  const key = keyOf(S.run);
  const runs = S.byKey.get(key);
  el.append("run ");
  runs.forEach((run, i) => {
    const b = document.createElement("button");
    b.type = "button";
    b.textContent = String(i + 1);
    b.setAttribute("aria-pressed", String(run === S.run));
    b.title = `Replay run ${i + 1} of ${runs.length}, recorded ${new Date(run.started).toISOString().slice(11, 19)} UTC`;
    b.addEventListener("click", () => {
      S.pick[key] = i;
      print(`(switching to run ${i + 1} of ${runs.length} of this drill)`, "note");
      startReplay(run);
    });
    el.append(b);
  });

}

function renderSpeeds() {
  const el = $("speeds");
  el.innerHTML = "";
  for (const speed of [1, 6, 20]) {
    const b = document.createElement("button");
    b.type = "button";
    b.textContent = `${speed}×`;
    b.setAttribute("aria-pressed", String(speed === S.speed));
    b.addEventListener("click", () => { S.speed = speed; renderSpeeds(); });
    el.append(b);
  }
}

// ---------- the strip under the scene ------------------------------------------------

const strip = $("strip");
function drawStrip() {
  const dpr = Math.min(devicePixelRatio || 1, 2);
  const w = strip.clientWidth, h = strip.clientHeight;
  if (strip.width !== w * dpr) { strip.width = w * dpr; strip.height = h * dpr; }
  const c = strip.getContext("2d");
  c.setTransform(dpr, 0, 0, dpr, 0, 0);
  c.clearRect(0, 0, w, h);
  const run = S.run;
  if (!run) {
    c.fillStyle = "#8494a0"; c.font = "12px 'IBM Plex Sans', sans-serif";
    c.fillText("Run a drill in the terminal and its timeline appears here.", 2, h / 2 + 4);
    return;
  }
  const from = -run.baseline_s, to = run.observe_s, span = to - from;
  const x = (t) => ((t - from) / span) * w;
  const barW = Math.max(1, w / span - 1);
  const perSecond = run.rps * run.targets.length;

  for (let second = Math.floor(from); second < to; second++) {
    let ok = 0, fail = 0;
    for (const target of run.targets) { const b = bucket(run, target.name, second); ok += b.ok; fail += b.fail; }
    const okH = clamp(ok / perSecond, 0, 1) * (h - 16);
    c.fillStyle = second < S.t ? "#3fa798" : "rgba(63,167,152,0.28)";
    c.fillRect(x(second), h - okH, barW, okH);
    if (fail > 0) {
      const failH = Math.max(5, (fail / perSecond) * (h - 16));
      c.fillStyle = "#e2674a";
      c.fillRect(x(second), h - okH - failH - 1, barW, failH);
    }
  }
  // Slowest 1% of the keyed API (or of the only target): the top of the strip
  // is the caller's timeout.
  const slowOf = run.targets.find((t) => t.name === "keyed") || run.targets[0];
  c.strokeStyle = "#d9a441"; c.lineWidth = 1.5; c.beginPath();
  let started = false;
  for (const b of slowOf.summary.seconds) {
    const y = h - 3 - clamp(b.p99_ms / run.timeout_ms, 0, 1) * (h - 8);
    const px = x(b.t) + barW / 2;
    started ? c.lineTo(px, y) : c.moveTo(px, y);
    started = true;
  }
  c.stroke();

  c.fillStyle = "#f2a48d"; c.fillRect(x(0), 0, 1, h);
  c.font = "10.5px 'IBM Plex Sans', sans-serif"; c.fillText("fault", x(0) + 4, 10);
  c.fillStyle = "#fff"; c.fillRect(x(S.t), 0, 1.5, h);
}
strip.addEventListener("click", (e) => {
  if (!S.run) return;
  const r = strip.getBoundingClientRect();
  seek(-S.run.baseline_s + ((e.clientX - r.left) / r.width) * (S.run.baseline_s + S.run.observe_s));
});
new ResizeObserver(drawStrip).observe(strip);

// ---------- the scene ----------------------------------------------------------------

const COLOR = {
  ok: new THREE.Color("#58c2b2"), keyed: new THREE.Color("#8fb4e8"), bad: new THREE.Color("#ee6a4b"),
  slow: new THREE.Color("#e0a93f"), body: new THREE.Color("#5d7688"), bodyHi: new THREE.Color("#8fb0c6"),
  frozen: new THREE.Color("#3a434b"), primary: new THREE.Color("#e9c46a"), stale: new THREE.Color("#ee6a4b"),
  starting: new THREE.Color("#55636e"),
};
const P = {
  callers: { open: new THREE.Vector3(-7.2, 0.55, 2.0), keyed: new THREE.Vector3(-7.2, 0.55, -1.2) },
  gatewayX: -1.4,
  upstream: [new THREE.Vector3(4.6, 0.5, 3.6), new THREE.Vector3(6.4, 0.5, 3.6)],
  redis: { "redis-0": new THREE.Vector3(2.6, 0.6, -2.8), "redis-1": new THREE.Vector3(5.4, 0.6, -2.8), "redis-2": new THREE.Vector3(8.2, 0.6, -2.8) },
  sentinel: [new THREE.Vector3(3.8, 2.9, -4.6), new THREE.Vector3(5.4, 3.2, -4.6), new THREE.Vector3(7.0, 2.9, -4.6)],
};

const scene = (() => {
  const canvas = $("scene");
  // Without WebGL the page still teaches: the terminal and the timeline replay
  // the run, and the picture is replaced by a line saying why it is missing.
  const flat = (why) => {
    document.querySelector(".stage").classList.add("flat");
    $("labels").innerHTML = `<p class="no3d">The 3D view could not start in this browser (${why}). The terminal and the timeline below still replay the run.</p>`;
    return { frame() {}, reset() {} };
  };
  let renderer;
  try {
    renderer = new THREE.WebGLRenderer({ canvas, antialias: true, alpha: true });
  } catch {
    return flat("WebGL is not available");
  }
  let lost = false;
  canvas.addEventListener("webglcontextlost", (e) => { e.preventDefault(); lost = true; flat("the graphics context was lost"); });
  renderer.setPixelRatio(Math.min(devicePixelRatio || 1, 2));
  const world = new THREE.Scene();
  world.fog = new THREE.Fog(0x10161b, 22, 44);
  const camera = new THREE.PerspectiveCamera(34, 2, 0.1, 100);
  camera.position.set(-2.2, 11.8, 14.2);
  const controls = new OrbitControls(camera, canvas);
  controls.target.set(0.5, 0.9, 0.2);
  controls.enablePan = false; controls.enableDamping = true;
  controls.minDistance = 10; controls.maxDistance = 26;
  controls.minPolarAngle = 0.5; controls.maxPolarAngle = 1.42;
  controls.update();

  world.add(new THREE.HemisphereLight(0xe6f0f6, 0x1c2831, 1.5));
  const sun = new THREE.DirectionalLight(0xffffff, 2.2);
  sun.position.set(-6, 12, 8);
  world.add(sun);

  const floor = new THREE.GridHelper(40, 40, 0x24313b, 0x1a242c);
  floor.position.y = -0.02;
  world.add(floor);

  // The routes a request takes, drawn faintly so the layout reads at rest.
  const lanes = [];
  const lane = (from, to) => lanes.push(from.x, 0.03, from.z, to.x, 0.03, to.z);
  const gwHub = new THREE.Vector3(P.gatewayX, 0, 0);
  lane(P.callers.open, gwHub); lane(P.callers.keyed, gwHub);
  lane(gwHub, new THREE.Vector3(5.5, 0, 3.6)); lane(gwHub, new THREE.Vector3(5.4, 0, -2.8));
  const laneGeo = new THREE.BufferGeometry();
  laneGeo.setAttribute("position", new THREE.Float32BufferAttribute(lanes, 3));
  world.add(new THREE.LineSegments(laneGeo, new THREE.LineBasicMaterial({ color: 0x3a4c59 })));

  const labels = $("labels");
  const tracked = [];   // { object, el, dy }
  function label(object, dy = 1.1) {
    const el = document.createElement("div");
    el.className = "label3d";
    labels.append(el);
    const entry = { object, el, dy };
    tracked.push(entry);
    return entry;
  }
  function setLabel(entry, title, sub = "", tone = "") {
    const html = `${title}${sub ? `<small>${sub}</small>` : ""}`;
    if (entry.html !== html) { entry.el.innerHTML = html; entry.html = html; }
    entry.el.className = `label3d ${tone}`;
  }

  const boxGeo = new RoundedBoxGeometry(1.5, 1.1, 1.5, 4, 0.16);
  const mat = () => new THREE.MeshStandardMaterial({ color: COLOR.body.clone(), roughness: 0.5, metalness: 0.05, transparent: true });

  // Callers.
  const callerLabels = {};
  for (const [name, pos] of Object.entries(P.callers)) {
    const mesh = new THREE.Mesh(new THREE.SphereGeometry(0.55, 28, 18), new THREE.MeshStandardMaterial({ color: name === "open" ? COLOR.ok : COLOR.keyed, roughness: 0.4 }));
    mesh.position.copy(pos);
    world.add(mesh);
    callerLabels[name] = label(mesh, 1.15);
  }

  // Upstream.
  const upstreamGroup = new THREE.Group();
  for (const pos of P.upstream) {
    const mesh = new THREE.Mesh(boxGeo, mat());
    mesh.position.copy(pos); mesh.scale.set(0.9, 0.8, 0.9);
    upstreamGroup.add(mesh);
  }
  world.add(upstreamGroup);
  const upstreamAnchor = new THREE.Object3D();
  upstreamAnchor.position.set(5.5, 0.5, 3.6);
  world.add(upstreamAnchor);
  setLabel(label(upstreamAnchor, 1.05), "upstream", "2 pods");

  // Redis and its crown.
  const redis = {};
  for (const [name, pos] of Object.entries(P.redis)) {
    const mesh = new THREE.Mesh(new THREE.CylinderGeometry(0.78, 0.78, 1.2, 36), mat());
    mesh.position.copy(pos);
    world.add(mesh);
    const ring = new THREE.Mesh(new THREE.TorusGeometry(1.02, 0.07, 12, 56), new THREE.MeshBasicMaterial({ color: COLOR.primary, transparent: true, opacity: 0 }));
    ring.rotation.x = Math.PI / 2; ring.position.set(pos.x, pos.y + 0.8, pos.z);
    world.add(ring);
    redis[name] = { mesh, ring, label: label(mesh, 1.9), glow: 0 };
  }

  // Sentinels, and the lines along which they watch.
  const sentinelMeshes = P.sentinel.map((pos) => {
    const mesh = new THREE.Mesh(new THREE.OctahedronGeometry(0.4), new THREE.MeshStandardMaterial({ color: 0xc3d2dc, roughness: 0.35, metalness: 0.2 }));
    mesh.position.copy(pos);
    world.add(mesh);
    return mesh;
  });
  const sentinelLabel = label(sentinelMeshes[1], 0.8);
  const watchGeo = new THREE.BufferGeometry();
  watchGeo.setAttribute("position", new THREE.BufferAttribute(new Float32Array(18), 3));
  const watchMat = new THREE.LineBasicMaterial({ color: 0x9fb2c0, transparent: true, opacity: 0.35 });
  world.add(new THREE.LineSegments(watchGeo, watchMat));

  // Gateways come and go, so they are made on demand.
  const gateways = new Map();   // name -> { mesh, label, z }
  function gatewayFor(name) {
    if (!gateways.has(name)) {
      const mesh = new THREE.Mesh(boxGeo, mat());
      mesh.position.set(P.gatewayX, 0.5, 0);
      mesh.material.opacity = 0;
      world.add(mesh);
      gateways.set(name, { mesh, label: label(mesh, 1.25), z: 0, alive: true });
    }
    return gateways.get(name);
  }

  // Dots.
  const MAX = 420;
  const dotGeo = new THREE.SphereGeometry(0.12, 12, 8);
  const dots = new THREE.InstancedMesh(dotGeo, new THREE.MeshBasicMaterial(), MAX);
  dots.instanceMatrix.setUsage(THREE.DynamicDrawUsage);
  dots.count = 0;
  world.add(dots);
  const live = [];
  const m4 = new THREE.Matrix4();
  const shown = new Map();   // "target:second" -> failing dots already shown for that second
  let spawnDebt = { open: 0, keyed: 0 };
  let lastState = null;

  function reset() { live.length = 0; shown.clear(); spawnDebt = { open: 0, keyed: 0 }; }

  function spawn(target, run, state) {
    if (live.length >= MAX) return;
    const b = bucket(run, target, S.t);
    const total = b.ok + b.fail;
    if (total === 0) return;
    const key = `${target}:${b.t}`;
    let fail = Math.random() < b.fail / total;
    // A second with any failures shows at least three failing dots.
    if (b.fail > 0 && (shown.get(key) || 0) < 3) fail = true;
    if (fail) shown.set(key, (shown.get(key) || 0) + 1);

    const ready = state.gateways.filter((g) => g.status === "ready");
    const pool = ready.length ? ready : state.gateways;
    if (!pool.length) return;
    const gw = pool[Math.floor(Math.random() * pool.length)];
    const slow = !fail && b.p99_ms > 150 && Math.random() < 0.35;
    live.push({
      target, gw: gw.name, fail, slow,
      hold: slow ? clamp(b.p99_ms / 1000, 0.3, 2) * 0.8 : 0,
      up: Math.floor(Math.random() * P.upstream.length),
      leg: 0, p: 0, wait: 0,
      jitter: (Math.random() - 0.5) * 0.5,
    });
  }

  const tmp = new THREE.Vector3(), a = new THREE.Vector3(), b2 = new THREE.Vector3();
  function legPoints(dot, leg, primary) {
    const caller = P.callers[dot.target];
    const gw = gateways.get(dot.gw);
    const gwPos = gw ? gw.mesh.position : tmp.set(P.gatewayX, 0.5, 0);
    switch (leg) {
      case 0: return [caller, gwPos];
      case 1: return dot.target === "keyed" && primary ? [gwPos, primary] : null;       // look the key up
      case 2: return dot.target === "keyed" && primary ? [primary, gwPos] : null;
      case 3: return [gwPos, P.upstream[dot.up]];
      case 4: return [P.upstream[dot.up], gwPos];
      case 5: return [gwPos, caller];
      default: return null;
    }
  }

  function frame(dt, run, state) {
    if (lost) return;
    lastState = state;
    const primaryName = Object.keys(state.redis).find((n) => state.redis[n].role === "primary");
    const primary = state.redis[primaryName];
    const primaryPos = P.redis[primaryName];

    // Gateways: place the ones that exist, fade the ones that do not.
    const names = state.gateways.map((g) => g.name);
    names.forEach((name, i) => {
      const g = gatewayFor(name);
      g.alive = true;
      g.z = (i - (names.length - 1) / 2) * 3.6 + 0.4;
      const status = state.gateways[i].status;
      g.mesh.position.z += (g.z - g.mesh.position.z) * Math.min(1, dt * 6);
      const targetOpacity = status === "ready" ? 1 : 0.5;
      g.mesh.material.opacity += (targetOpacity - g.mesh.material.opacity) * Math.min(1, dt * 5);
      g.mesh.material.color.lerp(status === "ready" ? COLOR.bodyHi : status === "stopping" ? COLOR.stale : COLOR.starting, Math.min(1, dt * 5));
      const short = /^gateway-[ab]$/.test(name) ? name.replace("-", " ") : `gateway …${name.slice(-5)}`;
      setLabel(g.label, short, status === "ready" ? "ready" : status, status === "ready" ? "" : "bad");
      g.label.el.style.opacity = g.mesh.material.opacity > 0.2 ? 1 : 0;
    });
    for (const [name, g] of gateways) {
      if (names.includes(name)) continue;
      g.mesh.material.opacity += (0 - g.mesh.material.opacity) * Math.min(1, dt * 5);
      g.label.el.style.opacity = 0;
      if (g.mesh.material.opacity < 0.02) {
        world.remove(g.mesh); g.label.el.remove();
        tracked.splice(tracked.indexOf(g.label), 1);
        gateways.delete(name);
      }
    }

    // Redis.
    for (const [name, r] of Object.entries(redis)) {
      const st = state.redis[name];
      const isPrimary = st.role === "primary";
      r.mesh.material.color.lerp(st.frozen ? COLOR.frozen : isPrimary ? COLOR.bodyHi : COLOR.body, Math.min(1, dt * 5));
      const ringOn = isPrimary || st.stale;
      r.ring.material.opacity += ((ringOn ? 1 : 0) - r.ring.material.opacity) * Math.min(1, dt * 6);
      r.ring.material.color.lerp(st.stale ? COLOR.stale : st.frozen ? COLOR.frozen : COLOR.primary, Math.min(1, dt * 6));
      r.ring.rotation.z += dt * (st.frozen ? 0 : 0.6);
      let sub = isPrimary ? "primary" : "replica", tone = isPrimary ? "good" : "";
      if (st.frozen) { sub = isPrimary ? "primary, not answering" : "not answering"; tone = "bad"; }
      else if (st.stale) { sub = "still thinks it is primary"; tone = "bad"; }
      setLabel(r.label, name, sub, tone);
    }

    // Sentinels watch the primary; they are uneasy while it does not answer.
    const pos = watchGeo.attributes.position;
    P.sentinel.forEach((s, i) => { pos.setXYZ(i * 2, s.x, s.y, s.z); pos.setXYZ(i * 2 + 1, primaryPos.x, primaryPos.y + 0.6, primaryPos.z); });
    pos.needsUpdate = true;
    const uneasy = primary.frozen;
    watchMat.color.lerp(uneasy ? COLOR.slow : new THREE.Color(0x9fb2c0), Math.min(1, dt * 5));
    watchMat.opacity = uneasy ? 0.5 + 0.35 * Math.sin(performance.now() / 110) : 0.35;
    sentinelMeshes.forEach((mesh, i) => { mesh.rotation.y += dt * (uneasy ? 3 : 0.5); mesh.position.y = P.sentinel[i].y + Math.sin(performance.now() / 900 + i) * 0.06; });
    setLabel(sentinelLabel, "3 Sentinels", uneasy ? "is the primary down?" : "watching the primary", uneasy ? "bad" : "");

    setLabel(callerLabels.open, "callers, open API", `${run ? run.rps : 50} requests a second`);
    setLabel(callerLabels.keyed, "callers, keyed API", "every request carries a key");

    // Spawn: a steady stream of dots, a sample of the real traffic.
    if (run && S.playing && !reducedMotion) {
      for (const target of ["open", "keyed"]) {
        if (!run.targets.some((t) => t.name === target)) continue;
        spawnDebt[target] += dt * 9;
        while (spawnDebt[target] >= 1) { spawnDebt[target] -= 1; spawn(target, run, state); }
      }
    }

    // Move.
    let n = 0;
    for (let i = live.length - 1; i >= 0; i--) {
      const dot = live[i];
      if (dot.wait > 0) { dot.wait -= dt; }
      else { dot.p += dt * (dot.leg === 1 || dot.leg === 2 ? 3.4 : 1.9); }

      if (dot.p >= 1) {
        dot.p = 0; dot.leg++;
        // At the gateway, after the lookup: a failed request turns back here,
        // a slow one waits here.
        if (dot.leg === 1 && dot.target !== "keyed") dot.leg = 3;
        if (dot.leg === 3) {
          if (dot.fail) dot.leg = 5;
          else if (dot.hold) { dot.wait = dot.hold; dot.hold = 0; }
        }
        if (dot.leg === 1 && dot.fail && run && run.targets.find((t) => t.name === dot.target)?.summary.by_outcome.refused) dot.leg = 5;
      }
      const pts = dot.leg <= 5 ? legPoints(dot, dot.leg, primaryPos) : null;
      if (!pts) { live.splice(i, 1); continue; }
      a.copy(pts[0]); b2.copy(pts[1]);
      tmp.lerpVectors(a, b2, dot.p);
      tmp.y += Math.sin(dot.p * Math.PI) * (dot.leg === 1 || dot.leg === 2 ? 0.9 : 0.5) + 0.05;
      tmp.z += dot.jitter * Math.sin(dot.p * Math.PI);
      m4.makeTranslation(tmp.x, tmp.y, tmp.z);
      if (n < MAX) {
        dots.setMatrixAt(n, m4);
        const failing = dot.fail && dot.leg >= 3;
        dots.setColorAt(n, failing ? COLOR.bad : dot.wait > 0 ? COLOR.slow : dot.target === "open" ? COLOR.ok : COLOR.keyed);
        n++;
      }
    }
    dots.count = n;
    dots.instanceMatrix.needsUpdate = true;
    if (dots.instanceColor) dots.instanceColor.needsUpdate = true;

    // Draw.
    const w = canvas.clientWidth, h = canvas.clientHeight;
    if (canvas.width !== Math.floor(w * renderer.getPixelRatio()) || canvas.height !== Math.floor(h * renderer.getPixelRatio())) {
      renderer.setSize(w, h, false);
      camera.aspect = w / h;
      // A narrow stage (a phone) widens the lens so the whole lab stays in view.
      camera.fov = clamp(34 * (1.9 / camera.aspect), 34, 66);
      camera.updateProjectionMatrix();
    }
    controls.update();
    renderer.render(world, camera);
    for (const entry of tracked) {
      tmp.copy(entry.object.position); tmp.y += entry.dy;
      tmp.project(camera);
      entry.el.style.left = `${(tmp.x * 0.5 + 0.5) * w}px`;
      entry.el.style.top = `${(-tmp.y * 0.5 + 0.5) * h}px`;
      entry.el.style.display = tmp.z < 1 ? "" : "none";
    }
  }

  return { frame, reset, get state() { return lastState; } };
})();

let idle = null;   // what to show when nothing is being replayed
function idleScene(lesson) {
  const runs = lesson.key ? S.byKey.get(lesson.key) : null;
  const run = runs ? runs[S.pick[lesson.key] || 0] : S.bundle.runs[0];
  idle = stateAt(run, -run.baseline_s);
  if (!S.run || !S.playing) {
    $("clock").textContent = "the lab at rest";
    $("clock").classList.remove("fault");
  }
}

let last = performance.now();
function loop(now) {
  const dt = Math.min(0.05, (now - last) / 1000);
  last = now;
  advance(dt);
  const showRun = S.run && (S.playing || S.finished);
  const state = showRun ? stateAt(S.run, S.t) : idle;
  if (state) scene.frame(dt, showRun ? S.run : null, state);
  requestAnimationFrame(loop);
}

// ---------- results table ------------------------------------------------------------

// "3", or "0–85" when runs differ; the unit is written once.
const range = (values, digits = 0, unit = "") => {
  const lo = Math.min(...values).toFixed(digits), hi = Math.max(...values).toFixed(digits);
  return `${lo === hi ? lo : `${lo}–${hi}`}${unit}`;
};

function renderResults() {
  const names = {
    "redis-primary/hang": "Redis primary stops answering for 30 s",
    "redis-primary/crash": "Redis primary killed, restarts in place",
    "gateway-pod/crash": "One gateway pod killed without warning",
    "gateway-pod/evict": "One gateway pod deleted gracefully",
    "rolling-restart/rollout": "Every gateway pod restarted, as in an upgrade",
  };
  const rows = [];
  for (const [key, label] of Object.entries(names)) {
    const runs = S.byKey.get(key);
    if (!runs) continue;
    for (const target of runs[0].targets.map((t) => t.name)) {
      const sums = runs.map((r) => r.targets.find((t) => t.name === target).summary);
      const failed = sums.map((s) => s.failed);
      const outcomes = {};
      for (const s of sums) for (const [k, v] of Object.entries(s.by_outcome)) if (k !== "200") outcomes[k] = (outcomes[k] || 0) + v;
      const what = Object.keys(outcomes).map((k) => (/^\d+$/.test(k) ? `HTTP ${k}` : k)).join(", ") || "none";
      rows.push(`<tr>
        <td>${target === runs[0].targets[0].name ? label : ""}</td>
        <td>${target}</td>
        <td>${runs.length}</td>
        <td class="${Math.max(...failed) > 0 ? "hot" : ""}">${range(failed)}</td>
        <td>${what}</td>
        <td>${range(sums.map((s) => s.longest_outage_s), 1, " s")}</td>
        <td>${range(sums.map((s) => s.baseline_p99_ms), 1, " ms")}</td>
        <td>${range(sums.map((s) => s.after_max_ms), 0, " ms")}</td>
      </tr>`);
    }
  }
  $("results-table").innerHTML = `
    <thead><tr><th>Drill</th><th>API</th><th>Runs</th><th>Failed per run</th><th>Failed as</th><th>Longest gap</th><th>p99 before</th><th>Slowest after</th></tr></thead>
    <tbody>${rows.join("")}</tbody>`;
  $("takeaways").innerHTML = takeaways(S.byKey).map((t) => `<div class="take"><h3>${t.title}</h3><p>${t.text}</p></div>`).join("");
}

// ---------- links to a moment --------------------------------------------------------

// ?drill=redis-primary/hang&run=2&at=30 opens that run at that second, so a
// finding can be pointed at.
function openLink() {
  const q = new URLSearchParams(location.search);
  const runs = S.byKey.get(q.get("drill"));
  if (!runs) return;
  const run = runs[clamp(Number(q.get("run") || 1) - 1, 0, runs.length - 1)];
  S.pick[keyOf(run)] = runs.indexOf(run);
  const lesson = LESSONS.findIndex((l) => l.key === keyOf(run));
  if (lesson >= 0) openLesson(lesson, { focus: false });
  const [drill, fault] = keyOf(run).split("/");
  print(`faildrill run ${drill}${fault === "rollout" ? "" : ` --fault ${fault}`}`, "cmd");
  startReplay(run);
  if (q.has("at")) seek(Number(q.get("at")));
  $("course").scrollIntoView();
}

// ---------- start --------------------------------------------------------------------

load().then(() => {
  renderSpeeds();
  renderRunPick();
  renderResults();
  openLesson(clamp(store.get("faildrill.lesson", 0), 0, LESSONS.length - 1), { focus: false });
  print("Welcome. This terminal replays real runs recorded on a lab cluster.", "note");
  print("Type the command from the lesson, or press Tab to complete it. Type help for the list.", "note");
  print("");
  drawStrip();
  openLink();
  requestAnimationFrame(loop);
}).catch((err) => {
  $("provenance").textContent = `The recorded runs could not be loaded (${err.message}).`;
  print(`could not load the recorded runs: ${err.message}`, "warn");
});
