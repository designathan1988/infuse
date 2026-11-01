// settings/code — worktree location and branch prefix.
import { h } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { setRow, inputEl } from '../shell.js';

export function renderCode({ el, s, save }) {
  el.append(h(`<h1>${t('settings.codeTitle')}</h1>`));
  el.append(setRow('settings.worktreeDir', 'settings.worktreeDirDesc', inputEl(s.worktreeDir, v => save({ worktreeDir: v }))));
  el.append(setRow('settings.branchPrefix', 'settings.branchPrefixDesc', inputEl(s.branchPrefix, v => save({ branchPrefix: v }))));
}
