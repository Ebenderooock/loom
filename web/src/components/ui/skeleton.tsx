import { cn } from "@/lib/utils";

function Skeleton({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("rounded-md bg-gradient-to-r from-primary/5 via-primary/10 to-primary/5 bg-[length:200%_100%] animate-shimmer", className)}
      {...props}
    />
  );
}

export { Skeleton };
