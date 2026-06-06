// post-import.js — minimal example Loom plugin (JavaScript).
//
// Logs the event it receives and, if env.WEBHOOK_URL is set, POSTs a small
// JSON notification to it. Configure WEBHOOK_URL under the plugin's
// "Environment variables" (one per line, e.g. WEBHOOK_URL=https://...).
//
// Globals available to every plugin: event, env, console, fetch.
// See docs/plugins.md for the full contract.

console.log("event:", event.event, "topic:", event.topic);
console.log("title:", event.title);
console.log("data:", event.data); // objects are rendered as JSON

if (env.WEBHOOK_URL) {
  var res = fetch(env.WEBHOOK_URL, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      event: event.event,
      title: event.title,
      timestamp: event.timestamp,
    }),
  });
  console.log("webhook status:", res.status, "ok:", res.ok);
  if (!res.ok) {
    // Throwing marks the run as failed (visible under History).
    throw new Error("webhook returned " + res.status + ": " + res.body);
  }
} else {
  console.log("WEBHOOK_URL not set; skipping webhook");
}
