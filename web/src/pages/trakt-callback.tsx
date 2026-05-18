import { useNavigate } from "@tanstack/react-router";
import { useEffect } from "react";

export function TraktCallbackPage() {
  const navigate = useNavigate();

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const code = params.get("code") ?? "";
    navigate({ to: "/settings", search: { trakt_code: code } });
  }, [navigate]);

  return (
    <div className="flex h-screen items-center justify-center text-zinc-400">
      Completing Trakt authorization…
    </div>
  );
}

export default TraktCallbackPage;
