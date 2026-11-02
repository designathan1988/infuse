// spend — what a session snapshot says about one turn's money and how it ended. DOM-free, so the gate
// executes it against the payloads the server really sends.
export const isLowerBound = snap => (snap?.unpriced || []).length > 0;
export const isUnverified = snap => snap?.outcome === 'unverified';
export const turnCostOf = (snap, turn) => (snap?.turnCosts || [])[turn - 1] || 0;

// The turn a message belongs to. After a compaction several turns start at the same index and the
// last of them is the one that counts, because the costs stay indexed by turn number.
export function turnNumberAt(starts, index) {
  let turn = 0;
  (starts || []).forEach((start, k) => { if (start <= index) turn = k + 1; });
  return turn;
}

// What a "message" event changes in the snapshot's accounts.
export function spendPatch(snap, d) {
  const turnCosts = [...(snap.turnCosts || [])];
  if (d.turnCost != null && snap.turn > 0) turnCosts[snap.turn - 1] = d.turnCost;
  return { turnCost: d.turnCost ?? snap.turnCost, turnCosts, unpriced: d.unpriced ?? snap.unpriced };
}
