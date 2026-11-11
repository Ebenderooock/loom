import type * as React from "react";
import { cn } from "@/lib/utils";

const LAYERS = [
  {
    mask: "/brand/loom-logo-accent.png",
    color: "hsl(var(--loom-logo-accent, var(--accent)))",
  },
  {
    mask: "/brand/loom-logo-primary.png",
    color: "hsl(var(--loom-logo-primary, var(--primary)))",
  },
  { mask: "/brand/loom-logo-shade.png", color: "rgb(0 0 0)" },
];

export function LoomLogo({
  className,
  label,
}: {
  className?: string;
  label?: string;
}) {
  return (
    <span
      role={label ? "img" : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      data-testid="loom-logo"
      className={cn(
        "relative inline-block aspect-square shrink-0 [forced-color-adjust:none]",
        className,
      )}
    >
      {LAYERS.map(({ mask, color }) => {
        const image = `url(${mask})`;
        const style: React.CSSProperties = {
          backgroundColor: color,
          maskImage: image,
          WebkitMaskImage: image,
          maskSize: "contain",
          WebkitMaskSize: "contain",
          maskRepeat: "no-repeat",
          WebkitMaskRepeat: "no-repeat",
          maskPosition: "center",
          WebkitMaskPosition: "center",
        };
        return <span key={mask} className="absolute inset-0" style={style} />;
      })}
    </span>
  );
}
