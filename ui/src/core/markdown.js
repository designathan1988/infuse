// markdown — small, safe renderer: escape FIRST, then build tags. Paths that look like file references
// become clickable (.path-link). Code blocks get a copy button. No raw HTML ever passes through.
import { esc } from './dom.js';
import { t } from './i18n.js';

export function md(src) {
  const blocks = [];
  src = String(src ?? '').replace(/```[\w+-]*\n([\s\S]*?)```/g, (_, c) => {
    blocks.push(`<div class="code-wrap"><button class="icon-btn mini codecopy" title="${t('action.copy')}" data-copy-code>${CODE_ICON}</button><pre><code>${esc(c.replace(/\n$/, ''))}</code></pre></div>`);
    return `\u0000${blocks.length - 1}\u0000`;
  });
  let x = esc(src);
  x = x.replace(/^\|(.+)\|\n\|[-:| ]+\|\n((?:\|.*\|\n?)*)/gm, (_, head, body) =>
    `<table><tr>${head.split('|').map(c => `<th>${c.trim()}</th>`).join('')}</tr>${body.trim().split('\n').map(r => `<tr>${r.replace(/^\||\|$/g, '').split('|').map(c => `<td>${c.trim()}</td>`).join('')}</tr>`).join('')}</table>`);
  x = x.replace(/^### (.*)$/gm, '<h3>$1</h3>').replace(/^## (.*)$/gm, '<h2>$1</h2>').replace(/^# (.*)$/gm, '<h1>$1</h1>').replace(/^&gt; (.*)$/gm, '<blockquote>$1</blockquote>');
  x = x.replace(/`([^`\n]+)`/g, (_, c) => /^[\w.\-/\\]+\.[a-z0-9]{1,6}(:\d+)?$/i.test(c) && c.includes('.') ? `<code class="path-link" data-path="${c.replace(/:\d+$/, '')}">${c}</code>` : `<code>${c}</code>`);
  x = x.replace(/\*\*([^*\n]+)\*\*/g, '<b>$1</b>').replace(/(^|[^*\w])\*([^*\n]+)\*/g, '$1<i>$2</i>').replace(/\[([^\]]+)\]\((https?:[^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>');
  x = x.replace(/(?:^[ \t]*(?:[-*]|\d+\.) .*(?:\n|$)(?:^[ \t]{2,}\S.*(?:\n|$))*)+/gm, mdList);
  x = x.split(/\n{2,}/).map(p => /^\s*<(h\d|ul|ol|pre|table|blockquote)/.test(p) || /^\u0000\d+\u0000$/.test(p.trim()) ? p : `<p>${p.replace(/\n/g, '<br>')}</p>`).join('');
  return x.replace(/\u0000(\d+)\u0000/g, (_, i) => blocks[i]);
}

function mdList(m) {
  let html = ''; const stack = [];
  for (const l of m.replace(/\n$/, '').split('\n')) {
    const mm = l.match(/^([ \t]*)([-*]|\d+\.) (.*)$/);
    if (!mm) { html += ' ' + l.trim(); continue; }
    const ind = mm[1].replace(/\t/g, '    ').length, type = /\d/.test(mm[2]) ? 'ol' : 'ul', n = parseInt(mm[2], 10);
    while (stack.length > 1 && ind < stack[stack.length - 1].ind) html += `</li></${stack.pop().type}>`;
    const top = stack[stack.length - 1];
    const open = () => { html += `<${type}${type === 'ol' && n > 1 ? ` start="${n}"` : ''}><li>`; stack.push({ ind, type }); };
    if (!top || ind > top.ind) open();
    else if (top.type !== type) { html += `</li></${stack.pop().type}>`; open(); }
    else html += '</li><li>';
    html += mm[3];
  }
  while (stack.length) html += `</li></${stack.pop().type}>`;
  return html;
}

const CODE_ICON = '<svg class="i code-ic" viewBox="0 0 24 24"><rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2"/></svg>';
