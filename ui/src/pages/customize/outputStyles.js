// customize/outputStyles — the output style picker: the built-in styles and the user's own files. The
// choice goes to the local settings, so it is personal and never versioned.
import { h, esc } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { api } from '../../core/api.js';
import { toast } from '../../components/toast.js';
import { patchLocalSettings } from './localSettings.js';

const styleName = st => (st.label ? t(st.label) : st.name);
// a built-in's prompt speaks to the model, in English: the screen shows its catalog line instead
const styleHint = st => (st.label ? t(st.label + 'Hint') : st.description || st.prompt);

const option = (st, on) => h(`<button class="style-option${on ? ' on' : ''}" role="radio" aria-checked="${on}"><span class="dot"></span><span class="grow"><b>${esc(styleName(st))}</b><span class="faint clamp-2">${esc(styleHint(st))}</span></span>${st.source === 'builtin' ? '' : `<span class="tag mono" title="${esc(st.source)}">${esc(st.name)}.md</span>`}</button>`);

export async function stylesSection(el, root) {
  el.append(h(`<h1>${t('cust.styles')}</h1>`), h(`<p class="lead">${t('cust.stylesDesc')}</p>`));
  const list = h(`<div class="connector-list" role="radiogroup" aria-label="${esc(t('cust.styles'))}"></div>`);
  el.append(list);
  const pick = async st => {
    const res = await patchLocalSettings(root, { outputStyle: st.name }).catch(e => ({ error: String(e) }));
    if (res.error) return toast(res.error);
    const current = await draw();
    toast(t(current === st.name ? 'style.saved' : 'style.overridden'));
  };
  const draw = async () => {
    const r = await api('/api/output-styles', { root }).catch(e => ({ error: String(e) }));
    if (r.error) { toast(r.error); return ''; }
    const current = r.current || 'default';
    list.innerHTML = '';
    for (const st of r.styles || []) {
      const b = option(st, st.name === current);
      b.onclick = () => pick(st);
      list.append(b);
    }
    return current;
  };
  await draw();
}
