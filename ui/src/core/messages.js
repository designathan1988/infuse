// messages — the plain text of a stored message; one definition for copy, speech and export.
export const messageText = m => (m.content || [])
  .filter(b => b.type === 'text')
  .map(b => b.display || b.text)
  .join('\n\n')
  .trim();
