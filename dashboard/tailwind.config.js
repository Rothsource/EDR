/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{js,jsx}"],
  theme: {
    extend: {
      colors: {
        // Carried over from the Stitch design — calm, restrained palette.
        background: "#f9f9ff",
        surface: "#ffffff",
        "surface-container": "#eff3ff",
        "surface-container-low": "#f5f7fe",
        "on-surface": "#151c27",
        "on-surface-variant": "#434751",
        outline: "#737783",
        "outline-variant": "#c3c6d3",
        primary: "#2e5aac",
        "on-primary": "#ffffff",
        "primary-container": "#2e5aac",
        success: "#2e9e5b",
        "success-container": "#e3f5ea",
        warning: "#d89614",
        "warning-container": "#fdf1dc",
        error: "#d64545",
        "error-container": "#ffdad6",
        "on-error-container": "#93000a",
      },
      fontFamily: {
        sans: ["Inter", "sans-serif"],
        mono: ["JetBrains Mono", "monospace"],
      },
      borderRadius: {
        xl: "0.75rem",
      },
    },
  },
  plugins: [],
};
