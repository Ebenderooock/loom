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
        },
        secondary: {
          DEFAULT: "hsl(var(--secondary))",
          foreground: "hsl(var(--secondary-foreground))",
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
        "purple": {
          "midnight": "#2D1B69",
          "rich": "#4A2C8F",
          "soft": "#6B4BA1",
        },
        "teal": {
          "electric": "#00BFA6",
          "ocean": "#00897B",
          "deep": "#00695C",
        },
        "indigo": {
          "periwinkle": "#7986CB",
          "soft": "#5C6BC0",
          "deep": "#3949AB",
        },
        "neutral": {
          "dark": "#1A1A2E",
          "card": "#2A2A3E",
          "light": "#F4F4F8",
          "white": "#FFFFFF",
          "muted": "#9E9E9E",
        },
        "semantic": {
          "success": "#00C853",
          "error": "#FF5252",
          "warning": "#FFD740",
          "info": "#40C4FF",
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
      },
      animation: {
        "accordion-down": "accordion-down 0.2s ease-out",
        "accordion-up": "accordion-up 0.2s ease-out",
      },
    },
  },
  plugins: [animate],
};

export default config;
