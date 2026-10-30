package dev.loom.tv;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.SocketTimeoutException;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import javax.net.ssl.SSLException;
import org.json.JSONException;
import org.json.JSONObject;

/** Unauthenticated pairing transport. Never sends a saved API key or follows redirects. */
final class PairingClient implements PairingSession.Protocol {
    static final class Retry extends IOException {
        final int seconds;
        Retry(int seconds) {
            super("Pairing rate limited. Wait " + seconds + " seconds, then try again.");
            this.seconds = seconds;
        }
    }

    static final class Offer {
        final String secret, code, verificationUrl;
        final int expiresIn, interval;
        Offer(String secret, String code, String verificationUrl, int expiresIn, int interval) {
            this.secret = secret;
            this.code = code;
            this.verificationUrl = verificationUrl;
            this.expiresIn = expiresIn;
            this.interval = interval;
        }
    }

    static final class Poll {
        final String status, key;
        Poll(String status, String key) { this.status = status; this.key = key; }
    }

    @Override public Offer start(ServerConfig config) throws IOException {
        try {
            return parseOffer(config, post(config, "start",
                    new JSONObject().put("device_name", "Loom TV"), 201));
        } catch (JSONException | IllegalArgumentException e) {
            throw malformed();
        }
    }

    @Override public Poll poll(ServerConfig config, String secret) throws IOException {
        try {
            JSONObject json = post(config, "poll", new JSONObject().put("device_secret", secret), 200);
            String status = requiredString(json, "status");
            if (status.equals("pending") || status.equals("denied")) return new Poll(status, "");
            if (status.equals("approved")) {
                String key = requiredString(json, "api_key");
                if (!key.matches("loom_[A-Za-z0-9]{24}")) throw malformed();
                ServerConfig approved = new ServerConfig(config.baseUrl, key, config.allowHttp);
                if (!approved.apiKey.equals(key)) throw malformed();
                return new Poll(status, key);
            }
            throw malformed();
        } catch (JSONException | IllegalArgumentException e) {
            throw malformed();
        }
    }

    @Override public void cancel(ServerConfig config, String secret) throws IOException {
        try {
            post(config, "cancel", new JSONObject().put("device_secret", secret), 204);
        } catch (JSONException e) {
            throw malformed();
        }
    }

    static Offer parseOffer(ServerConfig config, JSONObject json) throws IOException, JSONException {
        String secret = requiredString(json, "device_secret");
        String code = requiredString(json, "user_code");
        String path = requiredString(json, "verification_path");
        int expiry = seconds(json, "expires_in");
        int interval = seconds(json, "interval");
        if (!code.matches("[A-Z0-9]{4}-[A-Z0-9]{4}")
                || !path.equals("/pair?code=" + code)) throw malformed();
        // URI.resolve("/pair") would discard /loom. The contract is base-prefix relative.
        return new Offer(secret, code, config.baseUrl + path, expiry, Math.max(5, interval));
    }

    private static int seconds(JSONObject json, String field) throws IOException, JSONException {
        Object value = json.get(field);
        if (!(value instanceof Number)) throw malformed();
        double number = ((Number) value).doubleValue();
        if (number < 1 || number > 3600 || number != Math.floor(number)) throw malformed();
        return (int) number;
    }

    private static String requiredString(JSONObject json, String field) throws IOException, JSONException {
        Object value = json.get(field);
        if (!(value instanceof String) || ((String) value).isEmpty()) throw malformed();
        return (String) value;
    }

    private JSONObject post(ServerConfig config, String action, JSONObject body, int expected) throws IOException {
        HttpURLConnection connection = null;
        long started = System.nanoTime();
        try {
            connection = (HttpURLConnection)
                    new URL(config.baseUrl + "/api/v1/auth/pairing/" + action).openConnection();
            connection.setConnectTimeout(8000);
            connection.setReadTimeout(12000);
            connection.setInstanceFollowRedirects(false);
            connection.setRequestMethod("POST");
            connection.setRequestProperty("Accept", "application/json");
            connection.setRequestProperty("Content-Type", "application/json; charset=utf-8");
            connection.setDoOutput(true);
            byte[] bytes = body.toString().getBytes(StandardCharsets.UTF_8);
            connection.setFixedLengthStreamingMode(bytes.length);
            try (java.io.OutputStream stream = connection.getOutputStream()) { stream.write(bytes); }
            int status = connection.getResponseCode();
            if (status == 429) throw new Retry(retrySeconds(connection.getHeaderField("Retry-After")));
            if (status != expected) throw new SafeFailure(httpError(status));
            if (expected == 204) return null;
            try (InputStream stream = connection.getInputStream();
                 ByteArrayOutputStream output = new ByteArrayOutputStream()) {
                byte[] buffer = new byte[4096];
                int count;
                while ((count = stream.read(buffer)) != -1) {
                    if (output.size() + count > 65536 || System.nanoTime() - started > 30_000_000_000L) {
                        throw malformed();
                    }
                    output.write(buffer, 0, count);
                }
                try {
                    return new JSONObject(output.toString(StandardCharsets.UTF_8.name()));
                } catch (JSONException e) { throw malformed(); }
            }
        } catch (SocketTimeoutException e) {
            throw new SafeFailure("Pairing timed out. Check the network and start again.");
        } catch (SSLException e) {
            throw new SafeFailure("Pairing TLS failed. Check the certificate and TV date; certificates are never bypassed.");
        } catch (IOException e) {
            // Only our own fixed messages may reach the screen.
            if (e instanceof Retry || e instanceof SafeFailure) throw e;
            throw new SafeFailure("Pairing connection failed. Check the server and network, then start again.");
        } finally {
            if (connection != null) connection.disconnect();
        }
    }

    private static final class SafeFailure extends IOException {
        SafeFailure(String message) { super(message); }
    }

    private static IOException malformed() {
        return new SafeFailure("Unexpected pairing response. Check the server address and update Loom, then start again.");
    }

    private static String httpError(int status) {
        if (status == 404 || status == 405) return "Pairing unavailable: server may be too old or the URL prefix is wrong. Update Loom or use a manual API key.";
        if (status == 410) return "Pairing expired, already consumed or invalid. Start again for a new code.";
        if (status == 503) return "Pairing unavailable: Loom needs genuine authentication configured. Use manual setup if authentication is disabled.";
        if (status >= 300 && status < 400) return "Pairing redirected. Enter the final server URL; credentials are never forwarded.";
        return "Pairing returned HTTP " + status + ". Check server/proxy permissions or use a manual API key.";
    }

    static int retrySeconds(String value) {
        try { return Math.max(5, Math.min(3600, Integer.parseInt(value))); }
        catch (NumberFormatException e) { return 5; }
    }
}
