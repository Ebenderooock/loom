import { Toaster as Sonner, type ToasterProps } from "sonner";
import { useTheme } from "@/hooks/use-theme";

export function Toaster(props: ToasterProps) {
  const { resolvedTheme } = useTheme();
  return (
    <Sonner
      theme={resolvedTheme === "light" ? "light" : "dark"}
      className="toaster group"
      {...props}
    />
  );
}
