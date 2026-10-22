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

/** Read-only native API client. Call only on the IO executor. Never follows redirects. */
final class LoomClient {
    private static final int MAX_BYTES = 8 * 1024 * 1024;
    private static final long MAX_REQUEST_NANOS = 30_000_000_000L;

    static final class Failure extends IOException {
        Failure(String message) { super(message); }
    }

    Catalog load(ServerConfig config, boolean movies, int offset) throws IOException {
        long started = System.nanoTime();
        String path = movies ? "/api/v1/movies?limit=" + Catalog.PAGE_SIZE + "&offset=" + offset
                : "/api/v1/series";
        HttpURLConnection connection = (HttpURLConnection) new URL(config.baseUrl + path).openConnection();
        try {
            connection.setConnectTimeout(8000);
            connection.setReadTimeout(12000);
            connection.setInstanceFollowRedirects(false);
            connection.setRequestProperty("Accept", "application/json");
            if (!config.apiKey.isEmpty()) connection.setRequestProperty("X-Api-Key", config.apiKey);
            int status = connection.getResponseCode();
            if (status != 200) throw new Failure(httpError(status));
            try (InputStream stream = connection.getInputStream();
                 ByteArrayOutputStream bytes = new ByteArrayOutputStream()) {
                byte[] buffer = new byte[8192];
                int count;
                while ((count = stream.read(buffer)) != -1) {
                    if (Thread.currentThread().isInterrupted()) throw new Failure("Request cancelled. Retry when ready.");
                    if (System.nanoTime() - started > MAX_REQUEST_NANOS) {
                        throw new Failure("Catalog took too long to download. Check the network, then Retry.");
                    }
                    if (bytes.size() + count > MAX_BYTES) {
                        throw new Failure("Catalog exceeds the 8 MiB safety limit. Use Loom web for this library.");
                    }
                    bytes.write(buffer, 0, count);
                }
                try {
                    return Catalog.parse(bytes.toString(StandardCharsets.UTF_8.name()));
                } catch (JSONException e) {
                    throw new Failure("Unexpected catalog response. Check the server URL and Loom version.");
                }
            }
        } catch (SocketTimeoutException e) {
            throw new Failure("Server timed out. Check its address and network, then Retry.");
        } catch (SSLException e) {
            throw new Failure("TLS connection failed. Check the server certificate and TV date. Certificates are never bypassed.");
        } catch (Failure e) {
            throw e;
        } catch (IOException e) {
            // Never surface transport exceptions: they can contain URLs or response contents.
            throw new Failure("Cannot reach Loom. Check Wi-Fi, server address and server availability, then Retry.");
        } finally {
            connection.disconnect();
        }
    }

    static String httpError(int code) {
        if (code == 401) return "Authentication failed (401). Edit Server and check your API key.";
        if (code == 403) return "Access denied (403). Check your Loom account and proxy permissions.";
        if (code >= 300 && code < 400) return "Server redirected the request. Enter the final server URL; credentials are not forwarded.";
        if (code == 404) return "Catalog not found (404). Check the URL prefix and Loom server version.";
        if (code == 429) return "Server rate limit reached (429). Wait, then Retry.";
        return "Loom returned HTTP " + code + ". Check the server, then Retry.";
    }
}
