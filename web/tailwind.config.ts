import type { Config } from "tailwindcss";
import animate from "tailwindcss-animate";

const config: Config = {
  darkMode: [
    "variant",
    [
      '&:is([data-theme="dark"] *)',
      '&:is([data-theme="amoled"] *)',
      '&[data-theme="dark"]',
      '&[data-theme="amoled"]',
    ],
  ],
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    container: {
      center: true,
      padding: "2rem",
      screens: { "2xl": "1400px" },
    },
    extend: {
      colors: {
        border: "hsl(var(--border))",
        input: "hsl(var(--input))",
        ring: "hsl(var(--ring))",
        background: "hsl(var(--background))",
        foreground: "hsl(var(--foreground))",
        primary: {
          DEFAULT: "hsl(var(--primary))",
          foreground: "hsl(var(--primary-foreground))",
          readable: "hsl(var(--primary-readable, var(--primary)))",
        },
        secondary: {
          DEFAULT: "hsl(var(--secondary))",
          foreground: "hsl(var(--secondary-foreground))",
          readable: "hsl(var(--secondary-readable, var(--secondary)))",
        },
        destructive: {
          DEFAULT: "hsl(var(--destructive))",
          foreground: "hsl(var(--destructive-foreground))",
        },
        muted: {
          DEFAULT: "hsl(var(--muted))",
          foreground: "hsl(var(--muted-foreground))",
        },
        accent: {
          DEFAULT: "hsl(var(--accent))",
          foreground: "hsl(var(--accent-foreground))",
        },
        popover: {
          DEFAULT: "hsl(var(--popover))",
          foreground: "hsl(var(--popover-foreground))",
        },
        card: {
          DEFAULT: "hsl(var(--card))",
          foreground: "hsl(var(--card-foreground))",
        },
        /* Extended color palette */
        purple: {
          midnight: "hsl(var(--primary) / <alpha-value>)",
          rich: "hsl(var(--secondary) / <alpha-value>)",
          soft: "hsl(var(--secondary) / <alpha-value>)",
        },
        teal: {
          electric: "hsl(var(--accent) / <alpha-value>)",
          ocean: "hsl(var(--accent) / <alpha-value>)",
          deep: "hsl(var(--accent) / <alpha-value>)",
        },
        indigo: {
          periwinkle: "hsl(var(--secondary) / <alpha-value>)",
          soft: "hsl(var(--secondary) / <alpha-value>)",
          deep: "hsl(var(--secondary) / <alpha-value>)",
        },
        neutral: {
          dark: "hsl(var(--background) / <alpha-value>)",
          card: "hsl(var(--card) / <alpha-value>)",
          light: "hsl(var(--foreground) / <alpha-value>)",
          white: "hsl(var(--card) / <alpha-value>)",
          muted: "hsl(var(--muted-foreground) / <alpha-value>)",
        },
        semantic: {
          success: "#00C853",
          error: "#FF5252",
          warning: "#FFD740",
          info: "#40C4FF",
        },
      },
      borderRadius: {
        lg: "var(--radius)",
        md: "calc(var(--radius) - 2px)",
        sm: "calc(var(--radius) - 4px)",
      },
      keyframes: {
        "accordion-down": {
          from: { height: "0" },
          to: { height: "var(--radix-accordion-content-height)" },
        },
        "accordion-up": {
          from: { height: "var(--radix-accordion-content-height)" },
          to: { height: "0" },
        },
        shimmer: {
          "0%": { backgroundPosition: "-200% 0" },
          "100%": { backgroundPosition: "200% 0" },
        },
        "fade-in-up": {
          from: { opacity: "0", transform: "translateY(8px)" },
          to: { opacity: "1", transform: "translateY(0)" },
        },
        "slide-in-left": {
          from: { opacity: "0", transform: "translateX(-8px)" },
          to: { opacity: "1", transform: "translateX(0)" },
        },
        "glow-pulse": {
          "0%, 100%": { boxShadow: "0 0 5px hsl(var(--accent) / 0.2)" },
          "50%": { boxShadow: "0 0 20px hsl(var(--accent) / 0.4)" },
        },
        "gradient-flow": {
          "0%": { backgroundPosition: "0% 50%" },
          "100%": { backgroundPosition: "200% 50%" },
        },
      },
      animation: {
        "accordion-down": "accordion-down 0.2s ease-out",
        "accordion-up": "accordion-up 0.2s ease-out",
        shimmer: "shimmer 2s infinite linear",
        "fade-in-up": "fade-in-up 0.3s ease-out",
        "slide-in-left": "slide-in-left 0.25s ease-out",
        "glow-pulse": "glow-pulse 2s ease-in-out infinite",
        "gradient-flow": "gradient-flow 3s linear infinite",
      },
    },
  },
  plugins: [animate],
};

export default config;
