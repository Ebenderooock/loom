# Loom Design System

## Overview

The Loom UI uses a sophisticated color palette combining Deep Purples and Vibrant Teals to create a modern, focused interface. The design system supports three theme modes: Light (default), Dark, and AMOLED.

## Color Palette

### Deep Purples

| Name | Hex | HSL | Usage |
|------|-----|-----|-------|
| Midnight Purple | #2D1B69 | 270°, 50%, 41% | Primary brand, headers, nav backgrounds |
| Rich Violet | #4A2C8F | 266°, 36%, 53% | Buttons, CTAs, active states |
| Soft Plum | #6B4BA1 | 268°, 28%, 52% | Hover states, secondary elements |

### Vibrant Teals

| Name | Hex | HSL | Usage |
|------|-----|-----|-------|
| Electric Teal | #00BFA6 | 173°, 100%, 39% | Accent, highlights, progress bars, focus rings |
| Ocean Teal | #00897B | 174°, 100%, 30% | Icons, badges, success states |
| Deep Teal | #00695C | 173°, 100%, 21% | Dark mode accents, borders |

### Soft Indigo

| Name | Hex | HSL | Usage |
|------|-----|-----|-------|
| Periwinkle | #7986CB | 230°, 54%, 60% | Secondary UI, tags, chips |
| Soft Indigo | #5C6BC0 | 228°, 52%, 53% | Links, secondary buttons |
| Deep Indigo | #3949AB | 226°, 58%, 42% | Dark accents, sidebar elements |

### Neutrals

| Name | Hex | HSL | Usage |
|------|-----|-----|-------|
| Dark Slate | #1A1A2E | 240°, 23%, 18% | Dark mode background |
| Card Grey | #2A2A3E | 240°, 22%, 23% | Dark mode cards/surfaces |
| Soft White | #F4F4F8 | 220°, 20%, 97.6% | Light mode background |
| Card White | #FFFFFF | 0°, 0%, 100% | Light mode cards/surfaces |
| Muted Grey | #9E9E9E | 0°, 0%, 62% | Placeholder text, disabled states |

### Semantic Colors

| Name | Hex | HSL | Usage |
|------|-----|-----|-------|
| Success | #00C853 | 145°, 100%, 40% | Download complete, healthy status |
| Error | #FF5252 | 0°, 100%, 48% | Failed tasks, errors |
| Warning | #FFD740 | 45°, 100%, 50% | Indexer warnings, queue alerts |
| Info | #40C4FF | 199°, 100%, 50% | Tooltips, informational banners |

## Theme Modes

### Light Mode (Default)
- **Background**: Soft White (#F4F4F8)
- **Surface/Cards**: White (#FFFFFF)
- **Primary Text**: Dark Slate (#1A1A2E)
- **Accent/Highlight**: Rich Violet (#4A2C8F) for primary, Electric Teal (#00BFA6) for accents

### Dark Mode
- **Background**: Dark Slate (#1A1A2E)
- **Surface/Cards**: Card Grey (#2A2A3E)
- **Primary Text**: Soft White (#F4F4F8)
- **Accent/Highlight**: Midnight Purple (#2D1B69) for primary, Electric Teal (#00BFA6) for accents

### AMOLED Mode
- **Background**: Pure Black (#000000)
- **Surface/Cards**: Very Dark Grey (#0D0D0D)
- **Primary Text**: Soft White (#F4F4F8)
- **Accent/Highlight**: Midnight Purple (#2D1B69) for primary, Electric Teal (#00BFA6) for accents
- Optimized for OLED displays with perfect blacks to reduce power consumption

## Typography

| Category | Font | Weight | Size | Usage |
|----------|------|--------|------|-------|
| Headings | Inter / Poppins | Bold (700) | 24-48px | Page titles, major sections |
| Subheadings | Inter | Semi-Bold (600) | 16-20px | Section titles, list headers |
| Body | Inter | Regular (400) | 14-16px | Default text content |
| UI Labels | Inter | Medium (500) | 12-14px | Button text, form labels |
| Code/Logs | JetBrains Mono | Regular (400) | 12-13px | Code blocks, terminal output |
| Small Text | Inter | Regular (400) | 12px | Captions, helper text |

## CSS Variables (index.css)

The theme system uses CSS custom properties that automatically adapt based on the `data-theme` attribute:

### Root (Light Mode)
```css
--primary: 270 50% 41%;           /* Midnight Purple */
--secondary: 266 36% 53%;         /* Rich Violet */
--accent: 173 100% 39%;           /* Electric Teal */
--destructive: 0 84.2% 60.2%;     /* Error */
--background: 220 20% 97.6%;      /* Soft White */
--foreground: 240 17% 18%;        /* Dark Slate */
```

### Dark Mode ([data-theme="dark"])
```css
--primary: 270 50% 41%;           /* Midnight Purple */
--secondary: 266 36% 53%;         /* Rich Violet */
--accent: 173 100% 39%;           /* Electric Teal */
--background: 240 23% 18%;        /* Dark Slate */
--foreground: 220 20% 97.6%;      /* Soft White */
--card: 240 22% 23%;              /* Card Grey */
```

### AMOLED Mode ([data-theme="amoled"])
```css
--primary: 270 50% 41%;           /* Midnight Purple */
--secondary: 173 100% 35%;        /* Deep Teal */
--accent: 173 100% 39%;           /* Electric Teal */
--background: 0 0% 0%;            /* Pure Black */
--foreground: 220 20% 97.6%;      /* Soft White */
```

## Tailwind Color Utilities

### Standard Colors (Semantic)
```tailwind
bg-primary          /* Primary background (Midnight Purple) */
bg-secondary        /* Secondary background (Rich Violet) */
bg-accent           /* Accent background (Electric Teal) */
bg-destructive      /* Error state */
bg-muted            /* Muted/disabled state */
text-primary        /* Primary text */
text-accent         /* Accent text (Electric Teal in light, teal in dark) */
```

### Extended Palette
```tailwind
bg-purple-midnight  /* #2D1B69 */
bg-purple-rich      /* #4A2C8F */
bg-purple-soft      /* #6B4BA1 */

bg-teal-electric    /* #00BFA6 */
bg-teal-ocean       /* #00897B */
bg-teal-deep        /* #00695C */

bg-indigo-periwinkle /* #7986CB */
bg-indigo-soft       /* #5C6BC0 */
bg-indigo-deep       /* #3949AB */

bg-neutral-dark     /* #1A1A2E */
bg-neutral-card     /* #2A2A3E */
bg-neutral-light    /* #F4F4F8 */
bg-neutral-white    /* #FFFFFF */
bg-neutral-muted    /* #9E9E9E */

bg-semantic-success /* #00C853 */
bg-semantic-error   /* #FF5252 */
bg-semantic-warning /* #FFD740 */
bg-semantic-info    /* #40C4FF */
```

## Component Color Usage

### Buttons
- **Primary Button**: `bg-primary text-primary-foreground` (Midnight Purple)
- **Secondary Button**: `bg-secondary text-secondary-foreground` (Rich Violet)
- **Accent Button**: `bg-accent text-accent-foreground` (Electric Teal)
- **Destructive Button**: `bg-destructive text-destructive-foreground` (Error Red)

### Navigation
- **Active Link**: `text-accent` or `border-b-2 border-accent` (Electric Teal)
- **Nav Background**: `bg-primary` (Midnight Purple, dark mode respects theme)
- **Icon Hover**: `text-secondary` (Rich Violet)

### Status Indicators
- **Success**: `bg-semantic-success` or `text-semantic-success` (#00C853)
- **Error**: `bg-semantic-error` or `text-semantic-error` (#FF5252)
- **Warning**: `bg-semantic-warning` or `text-semantic-warning` (#FFD740)
- **Info**: `bg-semantic-info` or `text-semantic-info` (#40C4FF)

### Form Elements
- **Focus Ring**: `ring-offset-background ring-2 ring-accent` (Electric Teal ring)
- **Input Border**: `border border-input` (adapts to theme)
- **Placeholder**: `placeholder-muted-foreground` (Muted Grey)
- **Label**: `text-foreground font-medium` (Dark Slate in light, Soft White in dark)

### Cards & Surfaces
- **Background**: `bg-card text-card-foreground`
- **Border**: `border border-border`
- **Hover**: `hover:bg-accent/10` or `hover:shadow-lg`

## Best Practices

1. **Use semantic color names** (`primary`, `secondary`, `accent`, `destructive`) for components that should adapt to theme changes
2. **Use extended palette** for specific brand elements that should remain consistent across themes
3. **Respect contrast ratios** - Always ensure WCAG AA compliance (4.5:1 for text, 3:1 for UI components)
4. **Avoid hardcoding colors** - Use CSS variables or Tailwind utilities instead
5. **Test in all themes** - Light, Dark, and AMOLED modes should all be tested
6. **Use semantic colors** for status states (success, error, warning, info)

## Implementation Example

### Using React with Tailwind
```tsx
import { useTheme } from "@/hooks/use-theme";

export function MyComponent() {
  return (
    <div className="rounded-lg border border-border bg-card p-4 text-card-foreground shadow">
      <h2 className="text-lg font-bold text-primary">Title</h2>
      <button className="mt-4 rounded bg-accent px-4 py-2 text-accent-foreground hover:bg-accent/90">
        Action
      </button>
    </div>
  );
}
```

### Custom CSS
```css
.my-component {
  background-color: hsl(var(--card));
  color: hsl(var(--card-foreground));
  border: 1px solid hsl(var(--border));
}

.my-component:focus {
  outline: 2px solid hsl(var(--ring));
  outline-offset: 2px;
}
```

## Accessibility

The color palette meets WCAG AA standards for contrast:
- **Dark text on light backgrounds**: 7+ contrast ratio
- **Light text on dark backgrounds**: 7+ contrast ratio
- **UI components**: 3+ contrast ratio with adjacent colors

Always verify contrast ratios when creating custom color combinations.

## Dark Mode Detection

The theme automatically respects system preferences via `prefers-color-scheme`:
- If user hasn't set a preference, light mode is default
- Set `data-theme="dark"` or `data-theme="amoled"` on document root to override
- Use `useTheme()` hook in React components to manage theme switching

## Future Enhancements

- Custom color picker for user-defined themes
- Per-component color overrides
- High contrast mode for accessibility
- Color blindness-friendly palettes
- Animated theme transitions
