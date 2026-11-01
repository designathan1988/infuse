// signals — reactive state primitive shaped after the TC39 Signals proposal (Stage 1). A signal holds
// one value; computed derives; effect subscribes and reruns on change; batch coalesces notifications.
// The store and every component build on this — nothing renders from state that signals don't own.
let current = null;   // { run, deps:Set<Set<fn>> } — the effect now evaluating
let pending = new Set();
let depth = 0;

export function signal(value) {
  const subs = new Set(); // subscriber run() functions
  return {
    get() {
      if (current) { subs.add(current.run); current.deps.add(subs); }
      return value;
    },
    set(v) {
      if (Object.is(v, value)) return;
      value = v;
      for (const run of subs) pending.add(run);
      if (depth === 0) flush();
    },
    peek: () => value,
    subs
  };
}

export function computed(fn) {
  const s = signal(undefined);
  let started = false;
  const start = () => { started = true; effect(() => s.set(fn())); };
  return {
    get() { if (!started) start(); return s.get(); },
    peek() { if (!started) start(); return s.peek(); }
  };
}

export function effect(fn) {
  const e = { deps: new Set(), dead: false };
  e.run = () => {
    if (e.dead) return;
    for (const subs of e.deps) subs.delete(e.run);
    e.deps.clear();
    const old = current; current = e;
    try { fn(); } finally { current = old; }
  };
  e.run();
  return () => { e.dead = true; for (const subs of e.deps) subs.delete(e.run); };
}

function flush() {
  const list = [...pending]; pending.clear();
  for (const run of list) run();
}

export function batch(fn) {
  depth++;
  try { fn(); } finally { if (--depth === 0) flush(); }
}
