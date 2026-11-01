// composer — the prompt box and everything under/around it. The implementation lives in
// ./composer/: options, markup, context, wiring, autocomplete, keys, menus, slash commands and effects.
// This module keeps the stable public surface consumed by views, dialogs and pages.
export { MODE_INFO } from './composer/options.js';
export { mountComposer } from './composer/mount.js';
