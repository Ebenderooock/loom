import * as React from "react";
import { cn } from "@/lib/utils";

export function AnimatedNav({
  activeKey,
  children,
  className,
}: {
  activeKey?: string;
  children: React.ReactNode;
  className?: string;
}) {
  const containerRef = React.useRef<HTMLDivElement>(null);
  const highlightRef = React.useRef<HTMLDivElement>(null);

  React.useLayoutEffect(() => {
    const container = containerRef.current;
    const highlight = highlightRef.current;
    if (!container || !highlight) return;

    const update = () => {
      const active = container.querySelector<HTMLElement>(
        '[aria-current="page"]',
      );
      if (!active) {
        highlight.hidden = true;
        return;
      }

      const top =
        active.getBoundingClientRect().top -
        container.getBoundingClientRect().top;
      highlight.style.height = `${active.offsetHeight}px`;
      highlight.style.transform = `translateY(${top}px)`;
      highlight.hidden = false;
    };

    update();
    const observer =
      typeof ResizeObserver !== "undefined"
        ? new ResizeObserver(update)
        : undefined;
    observer?.observe(container);
    window.addEventListener("resize", update);
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", update);
    };
  }, [activeKey]);

  return (
    <div ref={containerRef} className={cn("relative isolate", className)}>
      <div
        ref={highlightRef}
        aria-hidden="true"
        hidden
        data-nav-highlight=""
        className="pointer-events-none absolute inset-x-0 top-0 -z-10 rounded-md border-l-2 border-accent bg-accent/15 transition-transform duration-200 ease-out motion-reduce:transition-none"
      />
      {children}
    </div>
  );
}
