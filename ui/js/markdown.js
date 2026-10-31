'use strict';
// Markdown rendering.

function mdList(m) {
  let html = ''; const stack = [];
  for (const l of m.replace(/\n$/, '').split('\n')) {
    const mm = l.match(/^([ \t]*)([-*]|\d+\.) (.*)$/);
    if (!mm) { html += ' ' + l.trim(); continue; } // a wrapped line of the item above
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
function md(src) {
  const blocks = [];
  src = src.replace(/```[\w+-]*\n([\s\S]*?)```/g, (_, c) => { blocks.push(`<div class="codewrap"><button class="ib codecopy" title="Copiar">${I.copy}</button><pre><code>${esc(c.replace(/\n$/, ''))}</code></pre></div>`); return `\u0000${blocks.length - 1}\u0000`; });
  let x = esc(src);
  x = x.replace(/^\|(.+)\|\n\|[-:| ]+\|\n((?:\|.*\|\n?)*)/gm, (_, head, body) => `<table><tr>${head.split('|').map(c => `<th>${c.trim()}</th>`).join('')}</tr>${body.trim().split('\n').map(r => `<tr>${r.replace(/^\||\|$/g, '').split('|').map(c => `<td>${c.trim()}</td>`).join('')}</tr>`).join('')}</table>`);
  x = x.replace(/^### (.*)$/gm, '<h3>$1</h3>').replace(/^## (.*)$/gm, '<h2>$1</h2>').replace(/^# (.*)$/gm, '<h1>$1</h1>').replace(/^&gt; (.*)$/gm, '<blockquote>$1</blockquote>');
  x = x.replace(/`([^`\n]+)`/g, (_, c) => /^[\w.\-/\\]+\.[a-z0-9]{1,6}(:\d+)?$/i.test(c) && c.includes('.') ? `<code class="path" data-path="${c.replace(/:\d+$/, '')}">${c}</code>` : `<code>${c}</code>`);
  x = x.replace(/\*\*([^*\n]+)\*\*/g, '<b>$1</b>').replace(/(^|[^*\w])\*([^*\n]+)\*/g, '$1<i>$2</i>').replace(/\[([^\]]+)\]\((https?:[^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>');
  x = x.replace(/(?:^[ \t]*(?:[-*]|\d+\.) .*(?:\n|$)(?:^[ \t]{2,}\S.*(?:\n|$))*)+/gm, mdList);
  x = x.split(/\n{2,}/).map(p => /^\s*<(h\d|ul|ol|pre|table|blockquote)/.test(p) || /^\u0000\d+\u0000$/.test(p.trim()) ? p : `<p>${p.replace(/\n/g, '<br>')}</p>`).join('');
  return x.replace(/\u0000(\d+)\u0000/g, (_, i) => blocks[i]);
}
