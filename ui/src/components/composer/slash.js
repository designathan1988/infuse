// composer slash commands — local commands and session actions; a false result means "send this text".
import { $ } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { api } from '../../core/api.js';
import { liveOf, setSessionOpts, cache, commandsOf, newSessionScreen, saveSettings } from '../../core/store.js';
import { toast } from '../toast.js';
import { messageText } from '../../core/messages.js';
import { vopts } from './options.js';
import { modelMenu, modeMenu } from './menus.js';

export async function handleSlash(ctx, text) {
  const { v, P, el } = ctx;
  const [cmd, ...rest] = text.slice(1).split(/\s+/);
  const arg = rest.join(' ');
  const root = vopts(v).root;
  const cmds = (await commandsOf(root)).filter(c => !c.builtin && c.name === cmd);
  if (cmds[0]) { P.value = cmds[0].prompt.replace(/\$ARGUMENTS/g, arg); return false; }
  const L = v.sid ? liveOf(v.sid) : null;
  switch (cmd) {
    case 'clear': case 'new': newSessionScreen(root); return true;
    case 'compact': if (L?.snap.get()?.turn > 1) {
      const r = await api(`/api/session/${L.id}/summarize`, { body: { turn: L.snap.get().turn, hint: arg }, root: L.root }).catch(e => ({ error: String(e) }));
      toast(r.error || t('chat.compactedToast'));
    } else toast(t('chat.nothingToCompact')); return true;
    case 'model': modelMenu(ctx, $('.composer-under .chip:last-child', el)); return true;
    case 'effort': modelMenu(ctx, $('.composer-under .chip:last-child', el), true); return true;
    case 'mode': case 'permissions': modeMenu(ctx, $('.chip[class*="mode-"]', el)); return true;
    case 'plan': if (v.sid) setSessionOpts(liveOf(v.sid), { mode: 'plan' }); else { v.draft.mode = 'plan'; ctx.renderUnder(); } if (arg) { P.value = arg; return false; } return true;
    case 'rewind': case 'undo': case 'checkpoint': import('../dialogs.js').then(d => d.rewindDialog(v)); return true;
    case 'diff': import('../../panes/index.js').then(p => p.togglePane('changes', true)); return true;
    case 'cost': case 'usage': case 'context': import('../dialogs.js').then(d => d.contextDialog()); return true;
    case 'btw': import('../view.js').then(m => m.openSideChat(v, arg)); return true;
    case 'config': case 'settings': import('../../pages/settings.js').then(p => p.openSettings()); return true;
    case 'resume': import('../dialogs.js').then(d => d.palette('sessions')); return true;
    case 'rename': if (L && arg) setSessionOpts(L, { title: arg }); return true;
    case 'review': case 'code-review': import('../dialogs.js').then(d => d.runReview(v, arg)); return true;
    case 'mission': case 'missao': if (arg) P.value = arg; import('../dialogs.js').then(d => d.missionDialog(v)); return true;
    case 'init': P.value = t('slash.init'); return false;
    case 'memory': import('../../panes/index.js').then(p => p.togglePane('memory', true)); return true;
    case 'terminal': import('../../panes/index.js').then(p => p.togglePane('terminal', true)); return true;
    case 'vim': await saveSettings({ vim: !cache.settings.get().vim }); toast(cache.settings.get().vim ? t('vim.on') : t('vim.off')); return true;
    case 'help': import('../dialogs.js').then(d => d.shortcutsDialog()); return true;
    case 'copy': if (L) {
      const n = parseInt(arg) || 1;
      const ms = L.snap.get().messages.filter(x => x.role === 'assistant' && (x.content || []).some(b => b.type === 'text' && b.text?.trim()));
      const m = ms[ms.length - n];
      if (m) navigator.clipboard.writeText(messageText(m));
    } return true;
    case 'export': if (L) import('../../components/transcript.js').then(m => m.downloadConversation(L)); return true;
  }
  return false;
}


