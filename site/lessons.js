// The course text. The lessons are written by hand; every number in an outcome
// is read from the record being replayed, so the words cannot drift from the runs.

const sum = (run, name) => run.targets.find((t) => t.name === name)?.summary;
const eventAt = (run, pattern) => run.events.find((e) => pattern.test(e.text))?.t;
const s1 = (v) => Number(v).toFixed(1);
const plural = (n, one, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;
const span = (values) => {
  const lo = Math.min(...values), hi = Math.max(...values);
  return lo === hi ? `${lo}` : `between ${lo} and ${hi}`;
};
const failedAs = (summary) =>
  Object.entries(summary.by_outcome)
    .filter(([k]) => k !== "200")
    .map(([k, v]) => `${v} × ${/^\d+$/.test(k) ? `HTTP ${k}` : k === "timeout" ? "no answer in time" : `connection ${k}`}`)
    .join(", ");

export const LESSONS = [
  {
    id: "preflight",
    short: "Read before you break",
    title: "Read the install before you break it",
    body: [
      "The lab is small and real: two Tyk gateways installed from Tyk's own open-source Helm chart, three Redis pods watched by three Sentinels, and a plain HTTP service behind the gateway. Have a look at it first.",
      "Then run preflight. It reads the gateway's Deployment and the Redis beside it and tells you which failures the install is not prepared for. It changes nothing.",
    ],
    commands: ["kubectl get pods -n faildrill-lab", "faildrill preflight -n faildrill-lab"],
    aside: "Press Tab to complete a command, or use the button on the terminal to have it typed for you.",
  },
  {
    id: "hang",
    key: "redis-primary/hang",
    short: "Redis stops answering",
    title: "The Redis primary stops answering",
    body: [
      "Redis holds every API key, quota and rate-limit counter the gateway uses. In this drill the primary does not die. It stays up and simply stops answering for thirty seconds, which is how a frozen node or a stalled disk looks from outside.",
      "Two streams of callers run the whole time, fifty requests a second each: one to an open API, one to an API that needs a key. Watch the three Sentinels above Redis decide what to do.",
    ],
    commands: ["faildrill run redis-primary --fault hang"],
    aside: "The replay runs faster than life and slows to real time whenever something is happening. Click the timeline under the picture to jump around.",
  },
  {
    id: "crash",
    key: "redis-primary/crash",
    short: "Redis crashes",
    title: "The Redis primary is killed and comes straight back",
    body: [
      "Now the primary's pod is killed outright. Its data is on a persistent volume and Kubernetes restarts it within a couple of seconds, well inside the five seconds the Sentinels wait before calling a primary dead.",
      "It sounds like the gentlest failure there is: gone and back before anyone has decided anything. Run it and see whether the callers agree.",
    ],
    commands: ["faildrill run redis-primary --fault crash"],
    aside: "This drill was recorded several times. Use the run numbers at the top right of the picture to compare them.",
  },
  {
    id: "pod",
    key: "gateway-pod/crash",
    short: "A gateway disappears",
    title: "One of the two gateways disappears",
    body: [
      "Preflight warned that nothing protects the gateways from being taken away together. Start smaller: take one away, without warning, the way a dying node would.",
      "There is a gentler version too. Add <code>--fault evict</code> and the pod is deleted with its grace period, as a node drain or a scale-down does it.",
    ],
    commands: ["faildrill run gateway-pod --fault crash"],
    aside: "Try <code>faildrill run gateway-pod --fault evict</code> afterwards and compare.",
  },
  {
    id: "rollout",
    key: "rolling-restart/rollout",
    short: "Upgrade day",
    title: "Every gateway is restarted, as an upgrade does it",
    body: [
      "The most common disruption is the planned one. Every chart upgrade, every configuration change and every node rotation restarts each gateway pod in turn.",
      "Preflight had two things to say about this: there is no preStop hook, and the rollout is allowed to take one of the two gateways away at a time. Here is what that costs.",
    ],
    commands: ["faildrill run rolling-restart"],
    aside: "A planned restart is the one failure you can run on a real install with little risk. It is a good first drill.",
  },
];

export function outcomeFor(lesson, run, bundle, byKey) {
  if (lesson.id === "preflight") {
    const warnings = (bundle.preflight.match(/^WARN/gm) || []).length;
    const notes = (bundle.preflight.match(/^note/gm) || []).length;
    const oks = (bundle.preflight.match(/^ok/gm) || []).length;
    return {
      figures: [
        { value: warnings, label: "warnings" },
        { value: notes, label: plural(notes, "note").replace(/^\d+ /, "") },
        { value: oks, label: "things in order" },
      ],
      paragraphs: [
        "None of the warnings is about Redis: it has replicas, Sentinels and volumes. They are all about the gateway. The chart gives it two replicas, but no disruption budget, no preStop hook and nothing that keeps the two pods on different nodes, and here they share one.",
        "A warning is a guess about what would hurt. The next four lessons put the guesses to the test, and not all of them come out the way the warnings suggest.",
      ],
    };
  }
  if (!run) return { figures: [], paragraphs: [] };

  const runs = byKey.get(lesson.key);
  const keyed = sum(run, "keyed"), open = sum(run, "open");
  const acrossRuns = (name) => runs.map((r) => sum(r, name).failed);
  const paragraphs = [];
  const figures = [
    { value: keyed.failed, label: "keyed requests failed" },
    { value: open.failed, label: "open requests failed" },
  ];

  if (lesson.id === "hang") {
    const promotion = run.facts.promotion_s;
    const again = eventAt(run, /answering again, still as primary/);
    const demoted = run.facts.old_primary_returned_s;
    figures.push({ value: `${Math.round(keyed.after_max_ms)} ms`, label: "slowest keyed answer" });
    if (promotion) figures.push({ value: `${promotion} s`, label: "until a replica was promoted" });

    paragraphs.push(keyed.failed === 0
      ? `In this run nobody got an error. Callers with a key waited instead: the slowest answer took ${Math.round(keyed.after_max_ms)} ms, against ${s1(keyed.baseline_p99_ms)} ms for the slowest 1% before the fault.`
      : `${plural(keyed.failed, "request")} with a key got no answer within the caller's two-second limit, between ${s1(keyed.first_failure_s)} and ${s1(keyed.last_failure_s)} seconds after the fault. The gateway did not refuse them. It held them while it waited on a Redis that was not answering.`);
    if (open.failed === 0) {
      paragraphs.push("The open API never noticed. Its requests do not need Redis, so a Redis failure is not a gateway failure: it is a failure of whatever needs a key, a quota or a rate limit.");
    }
    if (promotion) {
      paragraphs.push(`The Sentinels promoted a replica ${promotion} seconds after the fault. Five of those seconds are their configured patience, the time a primary may stay silent before it is called dead. That setting is the main thing deciding how long callers wait.`);
    }
    if (again !== undefined && demoted) {
      paragraphs.push(`Look at ${s1(again)} seconds, when the old primary wakes up. It still calls itself primary, and stays that way for ${s1(demoted - again)} seconds until the Sentinels demote it. The gateway had already moved to the new primary, so callers saw nothing. Anything still writing to the old one in that window would have its writes thrown away.`);
    }
    paragraphs.push(`This drill was recorded ${runs.length} times, and ${span(acrossRuns("keyed"))} keyed requests failed per run. The same fault does not produce the same number twice, so run a drill more than once before you quote it.`);
  }

  if (lesson.id === "crash") {
    const again = eventAt(run, /answering again, still as primary/);
    const promotion = run.facts.promotion_s;
    const refused = keyed.by_outcome["403"] || 0;
    if (promotion) figures.push({ value: `${promotion} s`, label: "until a replica was promoted" });
    if (again !== undefined) figures.push({ value: `${s1(again)} s`, label: "until the pod answered again" });

    if (again !== undefined && promotion) {
      paragraphs.push(`Kubernetes had the pod answering again ${s1(again)} seconds after it was killed. The Sentinels promoted a different pod anyway, at ${promotion} seconds. So the quick restart did not prevent a failover; it only meant that for a few seconds two pods both claimed to be primary.`);
    }
    if (refused > 0) {
      paragraphs.push(`${plural(refused, "request")} carrying a valid key ${refused === 1 ? "was" : "were"} answered with HTTP 403, between ${s1(keyed.first_failure_s)} and ${s1(keyed.last_failure_s)} seconds after the fault. A 403 tells a caller that its key is not allowed. The key was fine. A client that reacts to 403 by dropping its credentials or alerting a customer would do the wrong thing here, where a 5xx would have made it retry.`);
      paragraphs.push("The gateway's own log says why. In the same second it writes <code>storage: Redis is either down or was not configured</code> and then <code>Attempted access with non-existent key.</code> It could not reach Redis, treated the failed lookup as a key that does not exist, and refused the caller. When Redis hangs, the lookup waits and the caller times out. When Redis is briefly unreachable, the lookup fails at once and the caller is turned away.");
    } else if (keyed.failed === 0) {
      paragraphs.push("In this run no request failed at all. That is the outcome you would expect from a restart this fast, and it is not the only one that was recorded.");
    } else {
      paragraphs.push(`${plural(keyed.failed, "keyed request")} failed in this run (${failedAs(keyed)}), between ${s1(keyed.first_failure_s)} and ${s1(keyed.last_failure_s)} seconds after the fault, shortly before the Sentinels promoted another pod.`);
    }
    const with403 = runs.filter((r) => (sum(r, "keyed").by_outcome["403"] || 0) > 0).length;
    const clean = runs.filter((r) => sum(r, "keyed").failed === 0).length;
    paragraphs.push(`This drill was recorded ${runs.length} times. Nothing failed in ${clean} of them${with403 ? `, and valid keys were refused with HTTP 403 in ${with403}` : ""}; across all of them ${span(acrossRuns("keyed"))} keyed requests failed per run. Whether anything fails depends on timing that a single run cannot show, which is the argument for repeating a drill.`);
  }

  if (lesson.id === "pod") {
    const strength = run.facts.full_strength_s;
    if (strength) figures.push({ value: `${strength} s`, label: "until two gateways were ready again" });
    const failed = keyed.failed + open.failed;
    paragraphs.push(failed === 0
      ? `No request failed. The pod was gone, Kubernetes stopped sending it traffic, and a replacement was ready ${strength} seconds later. The second gateway carried everything in between.`
      : `${plural(failed, "request")} failed (${[failedAs(open), failedAs(keyed)].filter(Boolean).join("; ")}), and a replacement was ready ${strength} seconds after the fault. The second gateway carried the rest.`);
    const evict = byKey.get("gateway-pod/evict") || [];
    const crash = byKey.get("gateway-pod/crash") || [];
    const total = (rs) => rs.map((r) => sum(r, "keyed").failed + sum(r, "open").failed);
    if (evict.length && crash.length) {
      paragraphs.push(`Across the recorded runs, a killed pod cost ${span(total(crash))} failed requests per run and a gracefully deleted one ${span(total(evict))}. Out of about ${Math.round(run.targets.reduce((n, t) => n + t.summary.requests, 0) / 100) * 100} requests in each run, that is very little either way.`);
    }
    paragraphs.push("Be careful what you conclude. This lab has one node, so the Service learns that a pod is gone almost at once. Preflight's warning was about something this drill cannot imitate: both gateways sit on that one node, and losing it would take them together.");
  }

  if (lesson.id === "rollout") {
    const rollout = run.facts.rollout_s;
    if (rollout) figures.push({ value: `${rollout} s`, label: "for the whole rollout" });
    const failed = keyed.failed + open.failed;
    paragraphs.push(failed === 0
      ? `No request failed in this run, and both gateways had been replaced ${rollout} seconds after the restart began.`
      : `${plural(failed, "request")} failed in this run, each one a refused connection, and both gateways had been replaced ${rollout} seconds after the restart began.`);
    const totals = runs.map((r) => sum(r, "keyed").failed + sum(r, "open").failed);
    paragraphs.push(`Across the ${runs.length} recorded runs, ${span(totals)} requests failed per run. A refused connection is what a caller gets when it is sent to a pod that has already stopped listening: the pod is told to stop at the same moment the Service is told to stop using it, and the Service is slower. That is the gap a preStop hook closes, by keeping the pod serving for a few seconds after it has been asked to go.`);
    paragraphs.push("A handful of failures in several thousand requests looks small, and at fifty requests a second it is. The count grows with traffic, and it repeats on every upgrade.");
  }

  return { figures, paragraphs };
}

// The short conclusions under the results table, each one checked against the
// records before it is shown.
export function takeaways(byKey) {
  const cards = [];
  const all = (key, name) => (byKey.get(key) || []).map((r) => sum(r, name));
  const total = (list) => list.reduce((n, s) => n + s.failed, 0);

  const hangKeyed = all("redis-primary/hang", "keyed"), hangOpen = all("redis-primary/hang", "open");
  if (hangKeyed.length && total(hangOpen) === 0) {
    const slowest = Math.max(...hangKeyed.map((s) => s.after_max_ms));
    cards.push({
      title: "A Redis failure is a failure of keyed APIs, not of the gateway",
      text: `While the primary was silent, the open API lost no requests in any run. Callers with a key waited up to ${Math.round(slowest)} ms, and ${span(hangKeyed.map((s) => s.failed))} of them per run got no answer in time.`,
    });
  }

  const crashKeyed = all("redis-primary/crash", "keyed");
  const refused = crashKeyed.reduce((n, s) => n + (s.by_outcome["403"] || 0), 0);
  if (refused > 0) {
    const runsWith = crashKeyed.filter((s) => (s.by_outcome["403"] || 0) > 0).length;
    cards.push({
      title: "A quick restart can still refuse valid keys",
      text: `When the primary was killed and restarted in place, ${runsWith} of ${crashKeyed.length} runs answered callers holding a valid key with HTTP 403 (${refused} requests in all). Its log shows a failed Redis lookup being treated as a key that does not exist: the gateway said "not allowed" when it meant "I cannot check right now".`,
    });
  }

  const restarts = [...all("rolling-restart/rollout", "keyed"), ...all("rolling-restart/rollout", "open"), ...all("gateway-pod/evict", "keyed"), ...all("gateway-pod/evict", "open")];
  const refusedConnections = restarts.reduce((n, s) => n + (s.by_outcome.refused || 0), 0);
  if (refusedConnections > 0) {
    cards.push({
      title: "Planned restarts leak refused connections",
      text: `Restarting or gracefully deleting a gateway pod refused ${plural(refusedConnections, "connection")} across the recorded runs. The chart sets no preStop hook, so a pod can stop listening before the Service has stopped sending to it.`,
    });
  }

  cards.push({
    title: "One run is an anecdote",
    text: "The same drill, on the same lab, minutes apart, gave different counts each time. Every figure on this page is shown as a range across runs for that reason, and none of them is a promise about another cluster.",
  });
  return cards;
}
