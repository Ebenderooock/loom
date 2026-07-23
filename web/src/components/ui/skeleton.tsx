import { cn } from "@/lib/utils";

function Skeleton({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        "animate-shimmer from-primary/5 via-primary/10 to-primary/5 rounded-md bg-gradient-to-r bg-[length:200%_100%]",
        className,
      )}
      {...props}
    />
  );
}

export { Skeleton };
