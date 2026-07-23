import * as React from "react";

interface EmptyStateProps {
  icon?: React.ReactNode;
  title: string;
  description?: string;
  action?: React.ReactNode;
}

export function EmptyState({
  icon,
  title,
  description,
  action,
}: EmptyStateProps) {
  return (
    <div className="animate-fade-in-up flex flex-col items-center justify-center py-12 text-center">
      {icon && (
        <div className="text-muted-foreground mb-3 [&>svg]:h-10 [&>svg]:w-10">
          {icon}
        </div>
      )}
      <p className="text-sm font-medium">{title}</p>
      {description && (
        <p className="text-muted-foreground mt-1 max-w-sm text-xs">
          {description}
        </p>
      )}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}
