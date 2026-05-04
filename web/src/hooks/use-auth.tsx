import { createContext, useContext, useEffect, useState } from "react";

interface AuthContextType {
  isSetupComplete: boolean;
  isAuthenticated: boolean;
  isLoading: boolean;
  user: { id: number; username: string; email?: string; role: string } | null;
  logout: () => Promise<void>;
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

  // Check auth status on mount
  useEffect(() => {
    const checkStatus = async () => {
      try {
        const response = await fetch("http://localhost:8989/api/v1/auth/status", {
          credentials: "include",
        });

        if (response.ok) {
          const data = await response.json();
          setIsSetupComplete(!data.setup_required);
          setIsAuthenticated(data.is_authenticated);
          if (data.user) {
            setUser(data.user);
          }
        }
      } catch (err) {
        console.error("Failed to check auth status:", err);
      } finally {
        setIsLoading(false);
      }
    };

    checkStatus();
  }, []);

  const logout = async () => {
    try {
      await fetch("http://localhost:8989/api/v1/auth/logout", {
        method: "POST",
        credentials: "include",
      });
    } catch (err) {
      console.error("Logout failed:", err);
    } finally {
      setIsAuthenticated(false);
      setUser(null);
      // Refresh status from server
      const response = await fetch("http://localhost:8989/api/v1/auth/status", {
        credentials: "include",
      });
      if (response.ok) {
        const data = await response.json();
        setIsSetupComplete(!data.setup_required);
      }
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
      }}
    >
      {isLoading ? (
        <div className="w-screen h-screen bg-neutral-dark" />
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
