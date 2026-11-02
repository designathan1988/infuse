// writePolicy — the write-policy switches and how they read the settings payload. No DOM here, so
// scripts/check-ui.mjs can exercise it.

// In display order: settings key, label, description.
export const WRITE_POLICY = [
  ['requireContract', 'settings.requireContract', 'settings.requireContractDesc'],
  ['requireDiagnosis', 'settings.requireDiagnosis', 'settings.requireDiagnosisDesc'],
  ['enforceConventions', 'settings.enforceConventions', 'settings.enforceConventionsDesc'],
  ['enforceArchitecture', 'settings.enforceArchitecture', 'settings.enforceArchitectureDesc'],
  ['requireReview', 'settings.requireReview', 'settings.requireReviewDesc'],
  ['diagnostics', 'settings.diagnostics', 'settings.diagnosticsDesc'],
  ['userProhibitions', 'settings.userProhibitions', 'settings.userProhibitionsDesc'],
];

// Enforced by the agent loop with no setting behind them: shown, never offered as a switch.
export const ALWAYS_ON = [['settings.requirePlan', 'settings.requirePlanDesc']];

// /api/settings/full answers {settings: {...}}. An unset switch is on. Returns null when the payload
// has no settings, so a failed read can never be shown as defaults and saved over the real values.
export function writePolicyValues(payload) {
  const s = payload?.settings;
  if (!s) return null;
  return {
    toggles: Object.fromEntries(WRITE_POLICY.map(([key]) => [key, s[key] !== false])),
    maxFileLines: s.maxFileLines || 400,
    protectedPaths: (s.protectedPaths || []).join(', '),
  };
}
