# Loom Design System - Color Reference Guide

## Quick Reference

### Primary Colors

```
Purple #2D1B69 → Midnight Purple (headers, nav, primary buttons)
Teal   #00BFA6 → Electric Teal (accents, highlights, focus rings)
```

### How to Use in Components

#### Light Mode (Default)

```tsx
// Button with primary purple
<button className="bg-primary text-primary-foreground rounded px-4 py-2">
  Action
</button>

// Card with teal accent
<div className="bg-card border-l-4 border-accent rounded-lg p-4">
  Content
</div>

// Status indicators
<span className="text-semantic-success">✓ Success</span>
<span className="text-semantic-error">✗ Error</span>
```

#### Dark Mode Support

Colors automatically adapt when `data-theme="dark"` is set:

```tsx
// This component looks great in both light and dark modes
<div className="border border-border bg-card p-4 text-card-foreground">
  {content}
</div>
```

### Extended Palette Usage

```tsx
// Use extended palette for specific brand elements
<nav className="bg-purple-midnight">
  <a href="#" className="hover:text-teal-electric">Link</a>
</nav>

// Status colors
<div className="bg-semantic-success/10 text-semantic-success border border-semantic-success">
  Download Complete
</div>
```

## CSS Variables Reference

All colors are available as CSS custom properties:

| Variable        | Light Mode      | Dark Mode       | AMOLED Mode     |
| --------------- | --------------- | --------------- | --------------- |
| `--primary`     | Midnight Purple | Midnight Purple | Midnight Purple |
| `--secondary`   | Rich Violet     | Rich Violet     | Deep Teal       |
| `--accent`      | Electric Teal   | Electric Teal   | Electric Teal   |
| `--background`  | Soft White      | Dark Slate      | Pure Black      |
| `--foreground`  | Dark Slate      | Soft White      | Soft White      |
| `--card`        | White           | Card Grey       | Very Dark Grey  |
| `--destructive` | Error Red       | Error Red       | Error Red       |

### Using CSS Variables

```css
.my-component {
  background-color: hsl(var(--primary));
  color: hsl(var(--primary-foreground));
}

@supports (selector(:focus-visible)) {
  .my-component:focus-visible {
    outline: 2px solid hsl(var(--ring));
    outline-offset: 2px;
  }
}
```

## Component Color Guide

### Navigation

```tsx
// App navigation
<nav className="bg-primary text-primary-foreground">
  <Link to="/" className="hover:bg-secondary/20">Home</Link>
  <Link to="/sources" className="hover:bg-secondary/20">Sources</Link>
</nav>

// Active state
<Link
  to="/current"
  className="border-b-2 border-accent"
>
  Current Page
</Link>
```

### Forms

```tsx
<div>
  <label className="mb-2 block font-medium text-foreground">Label</label>
  <input
    className="rounded border border-input bg-background px-3 py-2 text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-accent focus:ring-offset-2"
    placeholder="Enter text..."
  />
</div>
```

### Status Cards

```tsx
// Success
<div className="bg-semantic-success/10 border border-semantic-success rounded p-4">
  <p className="text-semantic-success font-medium">Success</p>
</div>

// Error
<div className="bg-semantic-error/10 border border-semantic-error rounded p-4">
  <p className="text-semantic-error font-medium">Error</p>
</div>

// Warning
<div className="bg-semantic-warning/10 border border-semantic-warning rounded p-4">
  <p className="text-semantic-warning font-medium">Warning</p>
</div>
```

### Buttons

```tsx
// Primary (Midnight Purple)
<button className="bg-primary text-primary-foreground hover:bg-primary/90 rounded px-4 py-2 font-medium">
  Primary Action
</button>

// Secondary (Rich Violet)
<button className="bg-secondary text-secondary-foreground hover:bg-secondary/90 rounded px-4 py-2 font-medium">
  Secondary Action
</button>

// Accent (Electric Teal)
<button className="bg-accent text-accent-foreground hover:bg-accent/90 rounded px-4 py-2 font-medium">
  Accent Action
</button>

// Destructive (Error Red)
<button className="bg-destructive text-destructive-foreground hover:bg-destructive/90 rounded px-4 py-2 font-medium">
  Delete
</button>
```

### Tables

```tsx
<table>
  <thead className="bg-primary text-primary-foreground">
    <tr>
      <th className="px-4 py-2 text-left">Column</th>
    </tr>
  </thead>
  <tbody>
    <tr className="border-b border-border hover:bg-accent/5">
      <td className="px-4 py-2">Data</td>
    </tr>
  </tbody>
</table>
```

### Badges & Tags

```tsx
// Type badge
<span className="inline-flex items-center bg-indigo-deep/20 text-indigo-deep px-3 py-1 rounded-full text-sm font-medium">
  RSS
</span>

// Status badge
<span className="inline-flex items-center bg-semantic-success/20 text-semantic-success px-3 py-1 rounded-full text-sm font-medium">
  Active
</span>
```

### Modals/Dialogs

```tsx
<DialogContent className="border border-border bg-card text-card-foreground">
  <DialogHeader>
    <DialogTitle>Dialog Title</DialogTitle>
  </DialogHeader>
  <div className="py-4">Content goes here</div>
  <DialogFooter>
    <button className="bg-secondary text-secondary-foreground">Cancel</button>
    <button className="bg-primary text-primary-foreground">Confirm</button>
  </DialogFooter>
</DialogContent>
```

## Responsive & Dark Mode

### Using `dark:` prefix for dark-specific styles

```tsx
<div className="bg-white text-black dark:bg-card dark:text-foreground">
  This adapts to dark mode
</div>
```

### Using Tailwind's dark mode

Since Loom uses custom theme attributes, use this pattern:

```tsx
// Define in CSS
@layer base {
  [data-theme="dark"] .my-component {
    @apply bg-card text-card-foreground;
  }
}

// Or in JSX with conditional classNames
<div className={cn(
  "bg-white text-black",
  theme === "dark" && "bg-card text-card-foreground"
)}>
  Content
</div>
```

## Accessibility Checklist

- [ ] Text has 4.5:1 contrast ratio with background (WCAG AA)
- [ ] UI components have 3:1 contrast with adjacent colors
- [ ] Focus indicators use 2px outline with `--ring` color
- [ ] Error messages use both color and icons (not just red)
- [ ] Disabled states are visually distinct from interactive states

## Color Conversion Utility

If you need to convert between formats:

```tsx
// Hex to HSL (already done in design system)
// Use online tool: https://htmlcolorcodes.com/

// Example: #2D1B69 → HSL(270, 50%, 41%)
// Then in CSS: hsl(270 50% 41%)
```

## Testing Colors

### Manual Testing

1. **Light Mode**: Check all components with light background
2. **Dark Mode**: Toggle with `data-theme="dark"` on body
3. **AMOLED Mode**: Toggle with `data-theme="amoled"` on body
4. **Contrast**: Use DevTools color picker or Stark plugin
5. **Color Blindness**: Use WebAIM Contrast Checker or Deutan simulator

### Automated Testing

```tsx
// Example test for contrast
import { getContrast } from "@/lib/color-utils";

test("button has sufficient contrast", () => {
  const contrast = getContrast("#2D1B69", "#FFFFFF");
  expect(contrast).toBeGreaterThanOrEqual(4.5);
});
```

## Common Patterns

### Hover Effects

```tsx
className = "hover:bg-accent/10 hover:shadow-md transition-all duration-200";
```

### Focus Ring

```tsx
className =
  "focus:outline-none focus:ring-2 focus:ring-accent focus:ring-offset-2";
```

### Disabled State

```tsx
className = "disabled:opacity-50 disabled:cursor-not-allowed";
```

### Loading State

```tsx
className = "bg-accent/50 text-accent-foreground/50";
```
