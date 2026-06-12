import nextCoreWebVitals from "eslint-config-next/core-web-vitals";

// eslint 9 flat config. eslint-config-next 16 ships native flat-config arrays,
// so they are spread directly (FlatCompat is no longer needed / is incompatible).
const config = [
  ...nextCoreWebVitals,
  {
    ignores: [
      ".next/**",
      "node_modules/**",
      "test-results/**",
      "playwright-report/**",
      "next-env.d.ts",
    ],
  },
  {
    rules: {
      // react-hooks v6 (pulled in by eslint-config-next 16) adds this advisory.
      // It flags ~30 pre-existing init/sync effects (i18n locale, auth refresh)
      // that are functionally correct. Demoted to a warning so the Next 16
      // dependency migration stays focused — the effect cleanups are tracked
      // as separate follow-up, not a blocker.
      "react-hooks/set-state-in-effect": "warn",
    },
  },
];

export default config;
