// components/policy/filterRules — the user's filter rules editor: one table of when/match/action/value
// rules, the heredoc switch and a dry-run box. The Rules pane and the Rules page render the same
// component, so both always edit the same file the same way.
import { h, esc, debounce } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { I } from '../../core/icons.js';
import { api } from '../../core/api.js';
import { pref } from '../../core/store.js';
import { menu } from '../menu.js';
import { toast } from '../toast.js';

const actOf = x => x.replace != null ? 'replace' : x.say ? 'say' : x.infuse ? 'infuse' : x.infuseFile ? 'infuseFile' : x.vice ? 'vice' : 'say';

function shellHTML(scope, r, st) {
  return `<div class="pane-list">
      <div class="row mb-4"><span class="chip sc" role="button" tabindex="0">${t(scope === 'project' ? 'rules.thisProject' : 'rules.allProjects')} ${I.caret}</span><span class="faint mono trunc grow">${esc(r.file || '')}</span></div>
      <label class="row rules-toggle"><span class="switch ${st.disable.includes('heredoc') ? 'on' : ''} hd" role="switch" aria-checked="${st.disable.includes('heredoc')}" tabindex="0"></span>${t('rules.disableHeredoc')}</label>
      <div class="rule-row faint rules-header"><span></span><span>${t('rules.when')}</span><span>${t('rules.matchRegex')}</span><span>${t('rules.action')}</span><span>${t('rules.value')}</span><span></span></div>
      <div class="rows"></div>
      <button class="btn sm add mt-3">${I.plus} ${t('rules.addRule')}</button>
      <div class="faint rules-help-title">${t('rules.testCommand')}</div>
      <input class="tc mono rules-input" placeholder="cat > a.ts <<EOF …" aria-label="${esc(t('rules.testCommand'))}">
      <pre class="tout rules-out" hidden aria-live="polite"></pre></div>`;
}

function bindRow(s, row, x, i) {
  row.oninput = row.onchange = () => {
    x.when = row.querySelector('[data-k=when]').value;
    x.match = row.querySelector('[data-k=match]').value;
    const act = row.querySelector('[data-k=act]').value, val = row.querySelector('[data-k=val]').value;
    delete x.replace; delete x.say; delete x.infuse; delete x.infuseFile; delete x.vice;
    x[act] = val;
    s.save();
  };
  row.querySelector('[data-k=en]').onclick = e => {
    x.enabled = x.enabled === false;
    e.target.classList.toggle('on', x.enabled !== false);
    s.save();
  };
  row.querySelector('[data-k=del]').onclick = () => { s.st.rules.splice(i, 1); s.drawRows(); s.save(); };
}

function ruleRow(s, x, i) {
  const row = h(`<div class="rule-row"><span class="switch ${x.enabled === false ? '' : 'on'}" data-k="en" role="switch" aria-checked="${x.enabled !== false}" tabindex="0"></span><select data-k="when" aria-label="${esc(t('rules.when'))}">${['command', 'write', 'list', 'always'].map(w => `<option ${x.when === w ? 'selected' : ''}>${w}</option>`).join('')}</select><input data-k="match" class="mono" value="${esc(x.match || '')}" aria-label="${esc(t('rules.matchRegex'))}"><select data-k="act" aria-label="${esc(t('rules.action'))}">${['say', 'infuse', 'infuseFile', 'replace', 'vice'].map(a => `<option ${actOf(x) === a ? 'selected' : ''}>${a}</option>`).join('')}</select><input data-k="val" value="${esc(x.replace ?? x.say ?? x.infuse ?? x.infuseFile ?? x.vice ?? '')}" aria-label="${esc(t('rules.value'))}"><button class="icon-btn icon-22" data-k="del" aria-label="${esc(t('common.remove'))}">${I.close}</button></div>`);
  bindRow(s, row, x, i);
  return row;
}

function drawRows(s) {
  const rows = s.box.querySelector('.rows');
  rows.innerHTML = '';
  s.st.rules.forEach((x, i) => rows.appendChild(ruleRow(s, x, i)));
}

function saveRules(s) {
  return debounce(async () => {
    const res = await api(`/api/rules?scope=${s.scope}`, { method: 'PUT', body: { rules: s.st.rules, disable: s.st.disable }, root: s.root }).catch(e => ({ error: String(e) }));
    if (res.error) toast(res.error);
  }, 400);
}

async function testCommand(s, e) {
  if (e.key !== 'Enter') return;
  const res = await api('/api/rules/test', { body: { command: e.target.value }, root: s.root }).catch(e2 => ({ error: String(e2) }));
  const o = s.box.querySelector('.tout');
  o.hidden = false;
  o.textContent = `${t('rules.finalCommand')}:\n${res.finalCommand || ''}\n\n${t('rules.agentReceives')}:\n${(res.lines || []).join('\n') || t('rules.justNormalOutput')}`;
}

function wireRules(s) {
  s.box.querySelector('.add').onclick = () => { s.st.rules.push({ id: 'r' + Date.now(), when: 'command', match: '', say: '' }); s.drawRows(); s.save(); };
  s.box.querySelector('.hd').onclick = e => {
    const on = !e.target.classList.contains('on');
    e.target.classList.toggle('on', on);
    e.target.setAttribute('aria-checked', String(on));
    s.st.disable = on ? ['heredoc'] : [];
    s.save();
  };
  s.box.querySelector('.sc').onclick = e => menu(e.currentTarget, [
    { label: t('rules.thisProject'), check: s.scope === 'project', f: () => { pref('rulesScope', 'project'); drawRules(s); } },
    { label: t('rules.allProjects'), check: s.scope === 'global', f: () => { pref('rulesScope', 'global'); drawRules(s); } },
  ]);
  s.box.querySelector('.tc').onkeydown = e => testCommand(s, e);
}

async function drawRules(s) {
  s.scope = pref('rulesScope') === 'global' ? 'global' : 'project';
  const r = await api(`/api/rules?scope=${s.scope}`, { root: s.root }).catch(() => ({}));
  s.st = { rules: r.rules || [], disable: r.disable || [] };
  s.box.innerHTML = shellHTML(s.scope, r, s.st);
  s.save = saveRules(s);
  s.drawRows = () => drawRows(s);
  wireRules(s);
  drawRows(s);
}

// mountFilterRules renders the editor into box and keeps it live. root scopes the project file.
export function mountFilterRules(box, { root }) {
  const s = { box, root, scope: 'project', st: { rules: [], disable: [] } };
  drawRules(s);
  return { refresh: () => drawRules(s) };
}
