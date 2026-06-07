import { createContext, useContext, useEffect, useState } from "react";
import { apiFetch } from "@/lib/fetch";

interface AuthContextType {
  isSetupComplete: boolean;
  isAuthenticated: boolean;
  isLoading: boolean;
  user: { id: number; username: string; email?: string; role: string } | null;
  logout: () => Promise<void>;
  refreshAuth: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [isSetupComplete, setIsSetupComplete] = useState(false);
  const [isAuthenticated, setIsAuthenticated] = useState(false);
  const [user, setUser] = useState<{
    id: number;
    username: string;
    email?: string;
    role: string;
  } | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  // Check auth status on mount and poll until authenticated (after setup)
  useEffect(() => {
    let mounted = true;
    let pollInterval: ReturnType<typeof setInterval> | null = null;

    const checkStatus = async () => {
      try {
        const response = await apiFetch("/api/v1/auth/status");

        if (response.ok && mounted) {
          const data = await response.json();
          setIsSetupComplete(!data.setup_required);
          setIsAuthenticated(data.is_authenticated);
          if (data.user) {
            setUser(data.user);
          }

          // If setup is complete but not authenticated, poll for auth
          // (happens when user just completed setup on this browser)
          if (
            !data.setup_required &&
            !data.is_authenticated &&
            pollInterval === null
          ) {
            pollInterval = setInterval(checkStatus, 1000);
          } else if (pollInterval) {
            clearInterval(pollInterval);
            pollInterval = null;
          }
        }
      } catch (err) {
        console.error("Failed to check auth status:", err);
      }
    };

    checkStatus().then(() => {
      if (mounted) {
        setIsLoading(false);
      }
    });

    return () => {
      mounted = false;
      if (pollInterval) {
        clearInterval(pollInterval);
      }
    };
  }, []);

  const logout = async () => {
    try {
      await apiFetch("/api/v1/auth/logout", {
        method: "POST",
      });
    } catch (err) {
      console.error("Logout failed:", err);
    } finally {
      setIsAuthenticated(false);
      setUser(null);
      try {
        const response = await apiFetch("/api/v1/auth/status");
        if (response.ok) {
          const data = await response.json();
          setIsSetupComplete(!data.setup_required);
        }
      } catch {
        // Offline or network error — already logged out locally
      }
    }
  };

  const refreshAuth = async () => {
    try {
      const response = await apiFetch("/api/v1/auth/status");

      if (response.ok) {
        const data = await response.json();
        setIsSetupComplete(!data.setup_required);
        setIsAuthenticated(data.is_authenticated);
        if (data.user) {
          setUser(data.user);
        }
      }
    } catch (err) {
      console.error("Failed to refresh auth status:", err);
    }
  };

  return (
    <AuthContext.Provider
      value={{
        isSetupComplete,
        isAuthenticated,
        isLoading,
        user,
        logout,
        refreshAuth,
      }}
    >
      {isLoading ? (
        <div className="h-screen w-screen bg-neutral-dark" />
      ) : (
        children
      )}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error("useAuth must be used within AuthProvider");
  }
  return context;
}
