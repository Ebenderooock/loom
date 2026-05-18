import { LogViewer } from "@/components/logs/log-viewer";

export function SystemLogsPanel() {
  return (
    <div className="space-y-4">
      <div>
        <h3 className="text-lg font-semibold">System Logs</h3>
        <p className="text-sm text-muted-foreground">
          View real-time application logs and search historical entries.
          Logs are captured independently of the console output level.
        </p>
      </div>
      <LogViewer showConfig showStreamToggle />
    </div>
  );
}
