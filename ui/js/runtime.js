'use strict';
// runtime
// Objects a view or a session owns but that are not application state: DOM nodes, DOM-bound flags,
// attachments being composed and the SSE connection. Keeping them here lets the store stay plain and
// frozen, so every change to application state goes through an action.

const RT = { views: new Map(), live: new Map() };
let viewSeq = 0;

function newViewId() { return 'v' + (++viewSeq); }

function viewRT(v) {
  let r = RT.views.get(v.id);
  if (!r) {
    r = { el: null, atts: [], images: [], sideEl: null, stick: true, jump: null, prog: false, liveRaf: 0, hi: -1, vim: '', vimD: false };
    RT.views.set(v.id, r);
  }
  return r;
}

function elOf(v) { return v ? viewRT(v).el : null; }
function setViewEl(v, el) { viewRT(v).el = el; }
// a view object read before an action is stale afterwards; its id is not
function freshView(v) { return v && (S.views.find(x => x.id === v.id) || v); }

function liveRT(id) {
  let r = RT.live.get(id);
  if (!r) {
    r = { es: null, planPrev: {}, autofix: false, autofixSig: '', prAt: 0, pr: null, verdict: null };
    RT.live.set(id, r);
  }
  return r;
}
