// composer attachments — the attachment chips and the native/file-system picker bridge.
import { $, h, esc } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { api } from '../../core/api.js';

export async function attachFiles(ctx) {
  const list = window.infuse ? await infuse.call('pickFiles') : (await api('/api/dialog/files', { body: {}, root: '' }).catch(() => null));
  if (Array.isArray(list)) {
    for (const a of list) if (!ctx.v.atts.find(x => x.path === a.path)) ctx.v.atts.push(a);
    ctx.renderAtts();
  }
}

export function renderAttachments(ctx) {
  const box = $('.atts', ctx.el);
  box.innerHTML = '';
  ctx.v.atts.forEach((a, i) => {
    const c = h(`<span class="chipx" title="${esc(a.path)}">📎 <span>${esc(a.name)}</span><b aria-label="${esc(t('action.remove'))}">×</b></span>`);
    c.querySelector('b').onclick = () => { ctx.v.atts.splice(i, 1); renderAttachments(ctx); };
    box.appendChild(c);
  });
  ctx.v.images.forEach((im, i) => {
    const c = h(`<span class="chipx"><img src="data:${im.mediaType};base64,${im.data}" alt=""><span>${t('common.image')}</span><b aria-label="${esc(t('action.remove'))}">×</b></span>`);
    c.querySelector('b').onclick = () => { ctx.v.images.splice(i, 1); renderAttachments(ctx); };
    box.appendChild(c);
  });
}
