import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { AppearanceProvider } from "@/hooks/use-appearance";
import { AuthProvider } from "@/hooks/use-auth";
import { Toaster } from "@/components/ui/sonner";
import { AcquisitionRecovery } from "@/components/downloads/acquisition-recovery";
import { router } from "@/routes/router";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      refetchOnWindowFocus: false,
    },
  },
});

export function App() {
  return (
    <AuthProvider>
      <QueryClientProvider client={queryClient}>
        <AppearanceProvider>
          <RouterProvider router={router} />
          <Toaster />
          <AcquisitionRecovery />
        </AppearanceProvider>
      </QueryClientProvider>
    </AuthProvider>
  );
}
