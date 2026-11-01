// composer mount — builds the shell, then hands it to wiring and the reactive effect.
import { composerMarkup } from './markup.js';
import { createComposerContext } from './context.js';
import { wireComposer } from './wire.js';
import { mountEffect } from './effect.js';

export function mountComposer(v, el) {
  el.innerHTML = composerMarkup();
  const ctx = createComposerContext(v, el);
  wireComposer(ctx);
  mountEffect(ctx);
}
