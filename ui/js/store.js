'use strict';
// store
// The one state container: a reducer, dispatch as the only way to change state, selectors, and
// subscriptions. State is frozen after every change, so assigning to S.* throws instead of drifting.

function createStore(reducer, initial, { persist } = {}) {
  let state = freezeState(initial);
  const listeners = new Set();
  function dispatch(action) {
    const next = reducer(state, action);
    if (next === state) return action;
    state = freezeState(next);
    if (persist) persist(state, action);
    for (const fn of [...listeners]) fn(state, action);
    return action;
  }
  function getState() { return state; }
  function select(fn) { return fn(state); }
  function subscribe(fn) { listeners.add(fn); return () => listeners.delete(fn); }
  return { dispatch, getState, select, subscribe };
}

// deepFreeze makes any direct assignment to application state throw: the only writer is dispatch.
function deepFreeze(v) {
  if (!v || typeof v !== 'object' || Object.isFrozen(v)) return v;
  Object.freeze(v);
  for (const k of Object.keys(v)) deepFreeze(v[k]);
  return v;
}

function freezeState(state) { return deepFreeze(state); }
