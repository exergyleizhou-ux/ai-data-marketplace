import type { Config } from "tailwindcss";

// Design tokens (see DESIGN.md). The site's identity lives here:
// - paper / ink: warm off-white + near-black ink instead of pure white/black,
//   so the page reads like a printed document, not a screen UI.
// - forest: deep emerald accent, used SPARINGLY for the trust narrative.
// - gold: the wax-seal accent — appears only on verified-certificate cues.
const config: Config = {
  content: [
    "./app/**/*.{js,ts,jsx,tsx,mdx}",
    "./components/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  theme: {
    extend: {
      colors: {
        paper: "#fafaf7",
        ink: "#18181b",
        muted: "#71717a",
        rule: "#e7e5e0",
        forest: {
          DEFAULT: "#047857",
          50: "#ecfdf5",
          100: "#d1fae5",
          200: "#a7f3d0",
          600: "#047857",
          700: "#065f46",
          900: "#064e3b",
        },
        gold: {
          DEFAULT: "#b45309",
          50: "#fffbeb",
          100: "#fef3c7",
          600: "#b45309",
          700: "#92400e",
        },
      },
      fontFamily: {
        display: ["var(--font-display)", "ui-serif", "Georgia", "serif"],
        sans: ["var(--font-body)", "ui-sans-serif", "system-ui", "sans-serif"],
        mono: ["var(--font-mono)", "ui-monospace", "Menlo", "monospace"],
      },
      // Modular display scale: hero is dramatic, intentional density at each step.
      fontSize: {
        kicker: ["0.6875rem", { lineHeight: "1", letterSpacing: "0.16em", fontWeight: "500" }],
        "display-sm": ["2rem", { lineHeight: "1.05", letterSpacing: "-0.015em" }],
        "display-md": ["3rem", { lineHeight: "1.02", letterSpacing: "-0.02em" }],
        "display-lg": ["4.25rem", { lineHeight: "0.98", letterSpacing: "-0.025em" }],
        "display-xl": ["5.5rem", { lineHeight: "0.96", letterSpacing: "-0.03em" }],
      },
    },
  },
  plugins: [],
};

export default config;
