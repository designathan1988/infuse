'use strict';
// Interface language. The source strings are Portuguese; translations are data (`/locales/<lang>.json`),
// loaded once at boot, so adding a language never touches the code that renders it.
let EN = {};
let LANG = 'pt';

async function loadLocale(lang) {
  LANG = lang === 'en' ? 'en' : 'pt';
  if (LANG !== 'en') { EN = {}; return; }
  try {
    const r = await fetch('/locales/en.json', { cache: 'no-cache' });
    EN = await r.json();
  } catch { EN = {}; }
}

function t(s) {
  if (LANG !== 'en' || typeof s !== 'string') return s;
  const k = s.trim();
  return EN[k] !== undefined ? s.replace(k, EN[k]) : s;
}

function trNode(n) {
  if (n.nodeType === 3) { const v = t(n.nodeValue); if (v !== n.nodeValue) n.nodeValue = v; return; }
  if (n.nodeType !== 1 || n.tagName === 'SCRIPT' || n.tagName === 'STYLE' || n.classList?.contains('xterm')) return;
  for (const a of ['placeholder', 'title']) if (n.hasAttribute(a)) { const v = t(n.getAttribute(a)); if (v !== n.getAttribute(a)) n.setAttribute(a, v); }
  for (const c of n.childNodes) trNode(c);
}

function startTranslation() {
  LANG = S.lang;
  if (LANG !== 'en') return;
  document.documentElement.lang = 'en';
  trNode(document.body);
  new MutationObserver(ms => { for (const m of ms) { if (m.type === 'childList') m.addedNodes.forEach(trNode); else trNode(m.target); } }).observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ['placeholder', 'title'] });
}
