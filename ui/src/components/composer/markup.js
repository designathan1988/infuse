// composer markup — the empty shell; reactive regions are filled by effects and menus.
import { esc } from '../../core/dom.js';
import { t } from '../../core/i18n.js';
import { I } from '../../core/icons.js';
import { PROMPT_PH } from './options.js';

export function composerMarkup() {
  return `
    <div class="home-title"><b>${t('home.title')}</b><span class="hsub"></span></div>
    <div class="queue" aria-live="polite"></div>
    <div class="composer-box">
      <div class="atts"></div>
      <textarea class="prompt-input" rows="1" placeholder="${esc(t(PROMPT_PH))}" aria-label="${esc(t('composer.ariaLabel'))}"></textarea>
      <div class="composer-actions">
        <button class="icon-btn plusbtn" title="${esc(t('composer.attachAndMore'))}">${I.plus}</button>
        <span class="sp"></span>
        <span class="ctx-ring" role="button" tabindex="0" title="${esc(t('composer.context'))}"></span>
        <button class="send-btn" title="${esc(t('composer.send'))}">${I.up}</button>
      </div>
    </div>
    <div class="composer-under"></div>`;
}
