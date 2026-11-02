// rules — the Rules pane: the same filter-rules editor the Rules page hosts, in a side pane.
import { mountFilterRules } from '../components/policy/filterRules.js';

export function mount(pb, extra, c) {
  return mountFilterRules(pb, { root: c.root });
}
