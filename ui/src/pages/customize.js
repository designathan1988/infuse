// customize — per-project tailoring: bar commands, skills, MCP connectors, hooks, pinned docs,
// conventions, project instructions. Same format as Claude Code where it overlaps.
import { $, $$, h, esc, debounce } from '../core/dom.js';
import { t } from '../core/i18n.js';
import { fmtDate } from '../core/i18n.js';
import { I } from '../core/icons.js';
import { api } from '../core/api.js';
import { cache, projectLabel, curRoot } from '../core/store.js';
import { pageShell } from './shell.js';
import { menu } from '../components/menu.js';
import { toast } from '../components/toast.js';
import { md } from '../core/markdown.js';

const MCP_PRESETS = [
  { name: 'playwright', title: 'mcp.playwright.title', what: 'mcp.playwright.what', cfg: { command: 'npx', args: ['-y', '@playwright/mcp@latest'] }, needs: 'Node.js' },
  { name: 'context7', title: 'mcp.context7.title', what: 'mcp.context7.what', cfg: { command: 'npx', args: ['-y', '@upstash/context7-mcp'] }, needs: 'Node.js' },
  { name: 'github', title: 'mcp.github.title', what: 'mcp.github.what', cfg: { type: 'http', url: 'https://api.githubcopilot.com/mcp/', headers: { Authorization: 'Bearer ${GITHUB_TOKEN}' } }, needs: 'GITHUB_TOKEN' },
  { name: 'fetch', title: 'mcp.fetch.title', what: 'mcp.fetch.what', cfg: { command: 'uvx', args: ['mcp-server-fetch'] }, needs: 'uv' },
  { name: 'sequential-thinking', title: 'mcp.seq.title', what: 'mcp.seq.what', cfg: { command: 'npx', args: ['-y', '@modelcontextprotocol/server-sequential-thinking'] }, needs: 'Node.js' },
  { name: 'knowledge-graph', title: 'mcp.graph.title', what: 'mcp.graph.what', cfg: { command: 'npx', args: ['-y', '@modelcontextprotocol/server-memory'] }, needs: 'Node.js' },
];
const MCP_STATE = { running: ['ok', 'mcp.state.connected'], starting: ['warn', 'mcp.state.starting'], error: ['bad', 'mcp.state.error'], stopped: ['', 'mcp.state.stopped'] };
const HOOK_EVENTS = {
  PreToolUse: ['hooks.preToolUse', 'hooks.preToolUseDesc'],
  PostToolUse: ['hooks.postToolUse', 'hooks.postToolUseDesc'],
  UserPromptSubmit: ['hooks.promptSubmit', 'hooks.promptSubmitDesc'],
  Stop: ['hooks.stop', 'hooks.stopDesc'],
};
const HOOK_PRESETS = [
  { title: 'hooks.preset.tests', what: 'hooks.preset.testsWhat', event: 'Stop', matcher: '', command: "npm test --silent >/dev/null 2>&1 || { echo 'Os testes falharam. Rode npm test, corrija e só então termine.' >&2; exit 2; }" },
  { title: 'hooks.preset.noPush', what: 'hooks.preset.noPushWhat', event: 'PreToolUse', matcher: 'Bash', command: "grep -q 'git push' && { echo 'git push é feito pelo usuário, não pelo agente.' >&2; exit 2; } || exit 0" },
  { title: 'hooks.preset.prettier', what: 'hooks.preset.prettierWhat', event: 'PostToolUse', matcher: 'Write|Edit|MultiEdit', command: 'npx --no-install prettier --write . --log-level silent >/dev/null 2>&1 || true' },
  { title: 'hooks.preset.gitStatus', what: 'hooks.preset.gitStatusWhat', event: 'UserPromptSubmit', matcher: '', command: 'git status --short 2>/dev/null | head -30' },
];

export async function openCustomize(section = 'commands') {
  const sections = [
    ['commands', 'cust.commands'], ['skills', 'cust.skills'], ['connectors', 'cust.connectors'],
    ['hooks', 'cust.hooks'], ['docs', 'cust.docs'], ['conventions', 'cust.conventions'], ['instructions', 'cust.instructions']
  ];
  const el = pageShell('customize', sections, section, openCustomize);
  const root = curRoot() || cache.projects.get()[0]?.root || '';
  if (!root) { el.append(h(`<div class="empty">${t('cust.needProject')}</div>`)); return; }
  el.append(h(`<p class="faint field-note">${esc(projectLabel(root))} · ${esc(root)}</p>`));

  if (section === 'commands') return commandsSection(el, root);
  if (section === 'connectors') return connectorsSection(el, root);
  if (section === 'hooks') return hooksSection(el, root);
  if (section === 'docs') return docsSection(el, root);
  if (section === 'conventions') return conventionsSection(el, root);
  if (section === 'skills') return skillsSection(el, root);
  if (section === 'instructions') return instructionsSection(el, root);
}

async function commandsSection(el, root) {
  el.append(
    h(`<div class="row"><h1 class="grow">${t('cust.commands')}</h1><button class="btn line nc">${I.plus} ${t('cust.newCommand')}</button></div>`),
    h(`<p class="lead">${t('cust.commandsDesc')}</p>`)
  );
  const cmds = (await api('/api/commands', { root }).catch(() => [])).filter(c => !c.builtin);
  const editor = (c = { name: '', description: '', prompt: '' }) => {
    const d = h(`<div class="doc-editor-box field-my"><input class="name" placeholder="${esc(t('cust.cmdName'))}" value="${esc(c.name)}"><input class="desc field-select" placeholder="${esc(t('cust.cmdDesc'))}" value="${esc(c.description)}"><textarea placeholder="${esc(t('cust.cmdPrompt'))}">${esc(c.prompt)}</textarea><div class="row mt-4"><span class="chip scope" role="button" tabindex="0">${t('rules.thisProject')} ${I.caret}</span><span class="grow"></span>${c.name ? `<button class="btn danger del">${t('common.delete')}</button>` : ''}<button class="btn pri save">${t('common.save')}</button></div></div>`);
    let scope = 'project';
    d.querySelector('.scope').onclick = e => menu(e.currentTarget, [
      { label: t('rules.thisProject'), check: scope === 'project', f: () => { scope = 'project'; d.querySelector('.scope').firstChild.textContent = t('rules.thisProject') + ' '; } },
      { label: t('rules.allProjects'), check: scope === 'global', f: () => { scope = 'global'; d.querySelector('.scope').firstChild.textContent = t('rules.allProjects') + ' '; } },
    ]);
    d.querySelector('.save').onclick = async () => {
      const r = await api('/api/commands/save', {
        body: { name: d.querySelector('.name').value, description: d.querySelector('.desc').value, prompt: d.querySelector('textarea').value, scope, previousName: c.name },
        root
      }).catch(e => ({ error: String(e) }));
      if (r.error) return toast(r.error);
      delete cache.commandsByRoot[root];
      toast(t('cust.commandSaved'));
      openCustomize('commands');
    };
    d.querySelector('.del')?.addEventListener('click', async () => {
      await api('/api/commands/delete', { body: { name: c.name }, root }).catch(() => { });
      delete cache.commandsByRoot[root];
      openCustomize('commands');
    });
    return d;
  };
  el.querySelector('.nc').onclick = () => { const d = editor(); el.insertBefore(d, el.children[2]); d.querySelector('.name').focus(); };
  if (!cmds.length) el.append(h(`<div class="empty">${t('cust.noCommands')}</div>`));
  for (const c of cmds) {
    const r = h(`<div class="task-row" tabindex="0" role="button"><span class="mono">/${esc(c.name)}</span><span class="faint grow trunc">${esc(c.description)}</span></div>`);
    r.onclick = () => r.replaceWith(editor(c));
    el.append(r);
  }
}

async function connectorsSection(el, root) {
  el.append(
    h(`<div class="row"><h1 class="grow">${t('cust.connectors')}</h1><button class="icon-btn help" aria-label="${esc(t('common.howItWorks'))}">?</button></div>`),
    h(`<p class="lead">${t('cust.connectorsDesc')}</p>`)
  );
  const help = h(`<div class="howto field-help" hidden><ol>
    <li><b>${t('mcp.step1a')}</b> ${t('mcp.step1b')}</li>
    <li><b>${t('mcp.step2a')}</b> ${t('mcp.step2b')}</li>
    <li><b>${t('mcp.step3a')}</b> ${t('mcp.step3b')}</li></ol></div>`);
  el.append(help);
  el.querySelector('.help').onclick = () => { help.hidden = !help.hidden; };
  const list = h('<div class="connector-list"></div>');
  el.append(list);
  const act = async (path, body) => {
    const r = await api('/api/mcp/' + path, { body, root }).catch(e => ({ error: String(e) }));
    if (r.error) toast(r.error);
    draw(r.servers);
  };
  const draw = async servers => {
    servers = servers || await api('/api/mcp', { root }).catch(() => []);
    list.innerHTML = servers.length ? '' : `<div class="empty">${t('mcp.noneYet')}</div>`;
    for (const s of servers) {
      const [cls, label] = !s.approved ? ['warn', 'mcp.awaiting'] : s.disabled ? ['', 'mcp.off'] : MCP_STATE[s.state] || ['', s.state];
      const card = h(`<div class="connector-card"><div class="row"><span class="conn-dot ${cls}"></span><b>${esc(s.name)}</b><span class="tag">${t(s.scope === 'global' ? 'rules.allProjects' : 'rules.thisProject')}</span><span class="faint">${t(label)}${s.state === 'running' && !s.disabled ? ' · ' + t('mcp.toolsCount', { count: s.tools.length }) : ''}</span><span class="grow"></span>${s.approved ? `<span class="switch${s.disabled ? '' : ' on'} tg" role="switch" tabindex="0" aria-checked="${!s.disabled}" title="${esc(t('mcp.toggle'))}"></span>` : ''}<button class="icon-btn more" aria-label="${esc(t('common.more'))}">${I.more}</button></div>
        <div class="mono faint connector-target">${esc(s.target)}</div>
        ${!s.approved ? `<div class="connector-approve"><span>${t('mcp.approveWarning')}</span><button class="btn sm pri ap">${t('mcp.approveStart')}</button></div>` : ''}
        ${s.error ? `<div class="errbox mt-4">${esc(s.error)}${s.stderr ? '\n\n' + esc(s.stderr) : ''}</div>` : ''}
        ${s.tools.length ? `<details class="details-fold"><summary>${t('settings.toolsTitle')} (${s.tools.length})</summary>${s.tools.map(x => `<div class="conn-tool"><span class="mono">${esc(x.name)}</span>${x.readOnly ? `<span class="tag ok">${t('mcp.readOnly')}</span>` : ''}<div class="faint">${esc(x.description || '')}</div></div>`).join('')}</details>` : ''}</div>`);
      const ap = card.querySelector('.ap');
      if (ap) ap.onclick = () => { ap.textContent = t('mcp.state.starting'); act('approve', { scope: s.scope, name: s.name }); };
      const tg = card.querySelector('.tg');
      if (tg) tg.onclick = () => act('toggle', { scope: s.scope, name: s.name, disabled: !s.disabled });
      card.querySelector('.more').onclick = e => menu(e.currentTarget, [
        { label: t('common.restart'), f: () => act('restart', { scope: s.scope, name: s.name }) },
        { sep: 1 },
        { label: t('common.remove'), danger: 1, f: () => act('remove', { scope: s.scope, name: s.name }) },
      ], null, { alignRight: true });
      list.append(card);
    }
    drawPresets(servers);
  };
  const presets = h('<div></div>');
  el.append(h(`<h2>${t('mcp.addConnector')}</h2>`), presets);
  const addCfg = (name, cfg, scope) => act('add', { name, scope, config: cfg, approve: true });
  const drawPresets = servers => {
    const have = new Set(servers.map(s => s.name));
    presets.innerHTML = `<div class="connector-grid">${MCP_PRESETS.map((p, i) => `<div class="connector-preset${have.has(p.name) ? ' added' : ''}"><b>${t(p.title)}</b><span class="faint">${t(p.what)}</span><span class="mono faint">${esc(p.cfg.url || [p.cfg.command, ...p.cfg.args].join(' '))}</span><span class="faint">${t('mcp.needs')} ${esc(p.needs)}</span><div class="row">${have.has(p.name) ? `<span class="tag ok">${t('mcp.added')}</span>` : `<button class="btn sm" data-p="${i}" data-sc="project">${I.plus} ${t('rules.thisProject')}</button><button class="btn sm" data-p="${i}" data-sc="global">${t('rules.allProjects')}</button>`}</div></div>`).join('')}</div>
      <div class="connector-custom"><b>${t('mcp.otherConnector')}</b><div class="crit-sentence">${t('cust.name')} <input class="slot mono cn" placeholder="meu-servidor" spellcheck="false"> · ${t('mcp.run')} <input class="slot mono cc" placeholder="npx -y pacote-mcp" spellcheck="false"> ${t('mcp.orConnect')} <input class="slot mono cu" placeholder="https://exemplo.com/mcp" spellcheck="false"></div><div class="row"><button class="btn sm pri cadd">${t('mcp.addToProject')}</button><span class="faint">${t('mcp.splitHint')}</span></div></div>`;
    $$('[data-p]', presets).forEach(b => b.onclick = () => { const p = MCP_PRESETS[+b.dataset.p]; addCfg(p.name, p.cfg, b.dataset.sc); });
    presets.querySelector('.cadd').onclick = () => {
      const name = presets.querySelector('.cn').value.trim(), cmd = presets.querySelector('.cc').value.trim(), url = presets.querySelector('.cu').value.trim();
      if (!name || (!cmd && !url)) return toast(t('mcp.nameAndCommand'));
      const parts = cmd.split(/\s+/);
      addCfg(name, url && !cmd ? { type: 'http', url } : { command: parts[0], args: parts.slice(1) }, 'project');
    };
  };
  draw();
}

async function hooksSection(el, root) {
  el.append(h(`<div class="row"><h1 class="grow">${t('cust.hooks')}</h1></div>`), h(`<p class="lead">${t('cust.hooksDesc')}</p>`));
  const list = h('<div class="connector-list"></div>');
  el.append(list);
  const draw = async () => {
    const hooks = await api('/api/hooks', { root }).catch(() => []);
    list.innerHTML = hooks.length ? '' : `<div class="empty">${t('hooks.noneYet')}</div>`;
    for (const x of hooks) {
      const card = h(`<div class="connector-card"><div class="row"><span class="conn-dot ${x.approved ? 'ok' : 'warn'}"></span><b>${t(HOOK_EVENTS[x.event]?.[0] || x.event)}</b>${x.matcher ? `<span class="tag mono">${esc(x.matcher)}</span>` : ''}<span class="faint">${t(x.approved ? 'hooks.active' : 'mcp.awaiting')}</span><span class="grow"></span><span class="tag">${t(x.scope === 'global' ? 'rules.allProjects' : 'rules.thisProject')}</span><button class="icon-btn del" aria-label="${esc(t('common.remove'))}">${I.trash}</button></div><pre class="mono hook-cmd">${esc(x.command)}</pre><div class="faint text-sm">${esc(x.source)}</div>${x.approved ? '' : `<div class="connector-approve"><span>${t('hooks.approveWarning')}</span><button class="btn sm pri ap">${t('mcp.approve')}</button></div>`}</div>`);
      const ap = card.querySelector('.ap');
      if (ap) ap.onclick = async () => { await api('/api/hooks/approve', { body: { key: x.key }, root }).catch(() => { }); draw(); };
      card.querySelector('.del').onclick = async () => {
        const r = await api('/api/hooks/remove', { body: { key: x.key }, root }).catch(e => ({ error: String(e) }));
        if (r.error) toast(r.error);
        draw();
      };
      list.append(card);
    }
  };
  const add = async (event, matcher, command, scope = 'project') => {
    const r = await api('/api/hooks/add', { body: { event, matcher, command, scope }, root }).catch(e => ({ error: String(e) }));
    if (r.error) return toast(r.error);
    toast(t('hooks.added'));
    draw();
  };
  el.append(h(`<h2>${t('hooks.readyMade')}</h2>`));
  const grid = h(`<div class="connector-grid">${HOOK_PRESETS.map((p, i) => `<div class="connector-preset"><b>${t(p.title)}</b><span class="faint">${t(p.what)}</span><span class="faint">${t('hooks.when')}: ${t(HOOK_EVENTS[p.event][0])}${p.matcher ? ' · ' + esc(p.matcher) : ''}</span><span class="mono faint">${esc(p.command)}</span><div class="row"><button class="btn sm" data-h="${i}">${I.plus} ${t('rules.thisProject')}</button><button class="btn sm" data-h="${i}" data-sc="global">${t('rules.allProjects')}</button></div></div>`).join('')}</div>`);
  el.append(grid);
  $$('[data-h]', grid).forEach(b => b.onclick = () => { const p = HOOK_PRESETS[+b.dataset.h]; add(p.event, p.matcher, p.command, b.dataset.sc || 'project'); });
  let ev = 'PreToolUse';
  const custom = h(`<div class="connector-custom"><b>${t('hooks.other')}</b><div class="crit-sentence"><span class="chip hev" role="button" tabindex="0">${t(HOOK_EVENTS[ev][0])} ${I.caret}</span> ${t('hooks.forTools')} <input class="slot mono hm" placeholder="Bash|Write" spellcheck="false"> ${t('mcp.run')} <input class="slot mono hc field-wide" placeholder="./scripts/checar.sh" spellcheck="false"></div><div class="faint hevd text-md">${t(HOOK_EVENTS[ev][1])} ${t('hooks.jsonStdin')}</div><div class="row"><button class="btn sm pri hadd">${t('mcp.addToProject')}</button></div></div>`);
  el.append(custom);
  custom.querySelector('.hev').onclick = e => menu(e.currentTarget, Object.entries(HOOK_EVENTS).map(([k, [l, d]]) => ({
    label: t(l), d: t(d), check: ev === k,
    f: () => { ev = k; custom.querySelector('.hev').firstChild.textContent = t(l) + ' '; custom.querySelector('.hevd').textContent = t(d) + ' ' + t('hooks.jsonStdin'); }
  })));
  custom.querySelector('.hadd').onclick = () => add(ev, custom.querySelector('.hm').value.trim(), custom.querySelector('.hc').value.trim());
  draw();
}

const SKILL_EXAMPLE_KEY = 'skills.example';
async function skillsSection(el, root) {
  el.append(
    h(`<div class="row"><h1 class="grow">${t('cust.skills')}</h1><button class="btn line ns">${I.plus} ${t('skills.new')}</button></div>`),
    h(`<p class="lead">${t('cust.skillsDesc')}</p>`)
  );
  const list = h('<div class="connector-list"></div>');
  el.append(list);
  const editor = (sk = { name: '', description: '', body: '' }) => {
    const d = h(`<div class="connector-card"><div class="crit-sentence">${t('cust.name')} <input class="slot mono sn" placeholder="publicar-versao" value="${esc(sk.name)}" spellcheck="false"> · ${t('skills.useWhen')} <input class="slot sd field-wide-2" placeholder="${esc(t('skills.useWhenPlaceholder'))}" value="${esc(sk.description)}"></div><textarea class="doc-editor sb field-textarea" placeholder="${esc(t('skills.instructions'))}">${esc(sk.body)}</textarea><div class="row mt-3"><button class="btn sm pri sv">${t('skills.saveProject')}</button><button class="btn sm svg">${t('skills.saveGlobal')}</button><button class="btn sm ex">${t('oracle.insertExample')}</button><span class="grow"></span><button class="btn sm cancel">${t('common.cancel')}</button></div></div>`);
    const save = async scope => {
      const r = await api('/api/skills/save', { body: { scope, name: d.querySelector('.sn').value.trim(), description: d.querySelector('.sd').value.trim(), body: d.querySelector('.sb').value }, root }).catch(e => ({ error: String(e) }));
      if (r.error) return toast(r.error);
      toast(t('skills.saved'));
      draw(); d.remove();
    };
    d.querySelector('.sv').onclick = () => save('project');
    d.querySelector('.svg').onclick = () => save('global');
    d.querySelector('.ex').onclick = () => {
      if (!d.querySelector('.sn').value) d.querySelector('.sn').value = 'publicar-versao';
      if (!d.querySelector('.sd').value) d.querySelector('.sd').value = t('skills.useWhenPlaceholder');
      d.querySelector('.sb').value = t(SKILL_EXAMPLE_KEY);
    };
    d.querySelector('.cancel').onclick = () => d.remove();
    return d;
  };
  el.querySelector('.ns').onclick = () => { const d = editor(); list.prepend(d); d.querySelector('.sn').focus(); };
  const draw = async () => {
    const skills = await api('/api/skills', { root }).catch(() => []);
    list.innerHTML = skills.length ? '' : `<div class="empty">${t('skills.noneYet')}</div>`;
    for (const sk of skills) {
      const card = h(`<div class="connector-card"><div class="row"><b class="mono">${esc(sk.name)}</b><span class="tag">${t(sk.scope === 'global' ? 'rules.allProjects' : 'rules.thisProject')}</span><span class="faint grow trunc">${esc(sk.description)}</span><button class="icon-btn ed" aria-label="${esc(t('common.edit'))}">✎</button><button class="icon-btn del" aria-label="${esc(t('common.remove'))}">${I.trash}</button></div><details class="details-fold"><summary>${t('skills.instructions')}</summary><div class="markdown sel">${md(sk.body || '')}</div></details><div class="faint text-sm">${esc(sk.dir)}</div></div>`);
      card.querySelector('.ed').onclick = () => card.replaceWith(editor(sk));
      card.querySelector('.del').onclick = async () => { await api('/api/skills/remove', { body: { name: sk.name }, root }).catch(() => { }); draw(); };
      list.append(card);
    }
  };
  draw();
}

async function docsSection(el, root) {
  el.append(h(`<h1>${t('cust.docs')}</h1>`), h(`<p class="lead">${t('cust.docsDesc')}</p>`));
  const add = h(`<div class="connector-custom"><div class="crit-sentence">${t('docs.pinPage')} <input class="slot mono du field-wide-3" placeholder="https://react.dev/reference/react/useState" spellcheck="false"> <button class="btn sm pri dadd">${t('docs.pin')}</button></div></div>`);
  const list = h('<div class="connector-list mt-6"></div>');
  const sugg = h('<div></div>');
  el.append(add, list, sugg);
  const pin = async (url, btn) => {
    if (btn) { btn.disabled = true; btn.textContent = t('docs.downloading'); }
    const r = await api('/api/docs/pin', { body: { url }, root }).catch(e => ({ error: String(e) }));
    if (r.error) toast(r.error); else toast(t('docs.pinned'));
    draw();
  };
  add.querySelector('.dadd').onclick = e => { const u = add.querySelector('.du').value.trim(); if (u) pin(u, e.currentTarget); };
  add.querySelector('.du').onkeydown = e => { if (e.key === 'Enter' && e.target.value.trim()) pin(e.target.value.trim(), add.querySelector('.dadd')); };
  const draw = async () => {
    const r = await api('/api/docs', { root }).catch(() => ({}));
    list.innerHTML = (r.docs || []).length ? '' : `<div class="empty">${t('docs.noneYet')}</div>`;
    for (const d of r.docs || []) {
      const card = h(`<div class="connector-card"><div class="row"><b class="grow trunc">${esc(d.title)}</b><span class="faint">${Math.round(d.bytes / 1024)} KB · ${fmtDate(d.fetched)}</span><button class="icon-btn rf" aria-label="${esc(t('docs.redownload'))}">${I.reload}</button><button class="icon-btn del" aria-label="${esc(t('common.remove'))}">${I.trash}</button></div><div class="mono faint connector-target ml-0">${esc(d.url)} → ${esc(d.file)}</div></div>`);
      card.querySelector('.rf').onclick = () => pin(d.url, null);
      card.querySelector('.del').onclick = async () => { await api('/api/docs/unpin', { body: { url: d.url }, root }).catch(() => { }); draw(); };
      list.append(card);
    }
    const s = r.suggestions || [];
    sugg.innerHTML = s.length ? `<h2>${t('oracle.suggestions')}</h2><div class="connector-grid">${s.map((x, i) => `<div class="connector-preset"><b>${esc(x.name)}</b><span class="faint">${esc(x.why)}</span><span class="mono faint">${esc(x.url)}</span><div class="row"><button class="btn sm" data-s="${i}">${I.plus} ${t('docs.pin')}</button></div></div>`).join('')}</div>` : '';
    $$('[data-s]', sugg).forEach(b => b.onclick = () => pin(s[+b.dataset.s].url, b));
  };
  draw();
}

async function conventionsSection(el, root) {
  el.append(
    h(`<div class="row"><h1 class="grow">${t('cust.conventions')}</h1><button class="btn line rl">${I.reload} ${t('conv.reread')}</button></div>`),
    h(`<p class="lead">${t('cust.conventionsDesc')}</p>`)
  );
  const r = await api('/api/conventions', { root }).catch(() => ({}));
  const ta = h(`<textarea class="doc-editor preview-min" placeholder="${esc(t('cust.conventionsPlaceholder'))}">${esc(r.text || '')}</textarea>`);
  const st = h('<span class="faint" aria-live="polite"></span>');
  ta.oninput = debounce(async () => {
    await api('/api/conventions', { body: { text: ta.value }, root }).catch(() => { });
    st.textContent = t('common.saved');
    setTimeout(() => st.textContent = '', 1200);
  }, 600);
  el.querySelector('.rl').onclick = async () => {
    const x = await api('/api/conventions', { body: { relearn: true }, root }).catch(() => ({}));
    ta.value = x.text || '';
    toast(t('conv.rereadToast'));
  };
  el.append(ta, st);
}

async function instructionsSection(el, root) {
  el.append(h(`<h1>${t('cust.instructions')}</h1>`), h(`<p class="lead">${t('cust.instructionsDesc')}</p>`));
  const r = await api('/api/memory', { root }).catch(() => ({}));
  const ta = h(`<textarea class="doc-editor instructions-min" placeholder="${esc(t('cust.instructionsPlaceholder'))}">${esc(r.content || '')}</textarea>`);
  const st = h('<span class="faint" aria-live="polite"></span>');
  ta.oninput = debounce(async () => {
    await api('/api/memory', { method: 'PUT', body: { content: ta.value }, root }).catch(() => { });
    st.textContent = t('common.saved');
    setTimeout(() => st.textContent = '', 1200);
  }, 600);
  el.append(ta, st);
  if (r.instructions) el.append(h(`<h2>${t('cust.foundInstructions')}</h2>`), h(`<pre class="doc-editor sel source-preview">${esc(r.instructions)}</pre>`));
}
