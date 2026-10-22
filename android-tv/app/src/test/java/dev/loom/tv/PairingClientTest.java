package dev.loom.tv;

import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.net.InetAddress;
import java.net.ServerSocket;
import java.net.Socket;
import java.nio.charset.StandardCharsets;
import java.util.Map;
import java.util.TreeMap;
import java.util.concurrent.atomic.AtomicInteger;
import org.json.JSONObject;
import org.junit.After;
import org.junit.Before;
import org.junit.Test;
import static org.junit.Assert.*;

/** Synthetic credentials and loopback HTTP only; no production service or TV connection. */
public class PairingClientTest {
    private static final String SECRET = "synthetic-device-secret-for-unit-tests";
    private ServerSocket server;
    private Thread worker;
    private ServerConfig config;
    private volatile int status = 201;
    private volatile String response = offer().toString();
    private volatile String request, requestBody;
    private volatile Map<String, String> headers;
    private final AtomicInteger requests = new AtomicInteger();

    private static JSONObject offer() {
        try {
            return new JSONObject().put("device_secret", SECRET).put("user_code", "ABCD-EFGH")
                    .put("verification_path", "/pair?code=ABCD-EFGH").put("expires_in", 600).put("interval", 5);
        } catch (org.json.JSONException e) { throw new AssertionError(e); }
    }

    @Before public void startServer() throws Exception {
        server = new ServerSocket(0, 8, InetAddress.getByName("127.0.0.1"));
        config = new ServerConfig("http://127.0.0.1:" + server.getLocalPort() + "/loom", "saved-key-not-to-send", true);
        worker = new Thread(() -> {
            while (!server.isClosed()) {
                try (Socket socket = server.accept()) {
                    socket.setSoTimeout(3000);
                    BufferedReader input = new BufferedReader(new InputStreamReader(
                            socket.getInputStream(), StandardCharsets.UTF_8));
                    request = input.readLine();
                    Map<String, String> received = new TreeMap<>(String.CASE_INSENSITIVE_ORDER);
                    String line;
                    while ((line = input.readLine()) != null && !line.isEmpty()) {
                        int colon = line.indexOf(':');
                        if (colon > 0) received.put(line.substring(0, colon), line.substring(colon + 1).trim());
                    }
                    headers = received;
                    int length = Integer.parseInt(received.getOrDefault("Content-Length", "0"));
                    char[] body = new char[length];
                    int offset = 0, read;
                    while (offset < length && (read = input.read(body, offset, length - offset)) != -1) offset += read;
                    requestBody = new String(body, 0, offset);
                    requests.incrementAndGet();
                    byte[] bytes = response.getBytes(StandardCharsets.UTF_8);
                    OutputStream out = socket.getOutputStream();
                    out.write(("HTTP/1.1 " + status + " Test\r\nContent-Type: application/json\r\n"
                            + "Content-Length: " + bytes.length + "\r\nConnection: close\r\n"
                            + "Retry-After: 5\r\nLocation: " + config.baseUrl + "/collector\r\n\r\n")
                            .getBytes(StandardCharsets.US_ASCII));
                    if (status != 204) out.write(bytes);
                    out.flush();
                } catch (IOException ignored) { /* teardown / rejected response closes the stream */ }
            }
        }, "pairing-test-http");
        worker.setDaemon(true);
        worker.start();
    }

    @After public void stopServer() throws Exception {
        server.close();
        worker.join(4000);
        assertFalse(worker.isAlive());
    }

    @Test public void startUsesJsonPostAndPreservesVerificationPrefix() throws Exception {
        PairingClient.Offer result = new PairingClient().start(config);
        assertEquals("POST /loom/api/v1/auth/pairing/start HTTP/1.1", request);
        assertEquals("Loom TV", new JSONObject(requestBody).getString("device_name"));
        assertEquals(config.baseUrl + "/pair?code=ABCD-EFGH", result.verificationUrl);
        assertEquals(600, result.expiresIn);
        assertEquals(5, result.interval);
        assertFalse(headers.containsKey("X-Api-Key"));
        assertFalse(headers.containsKey("Authorization"));
        assertFalse(requestBody.contains(config.apiKey));
        assertTrue(headers.get("Content-Type").contains("application/json"));
    }

    @Test public void pollAndCancelUseSecretInBodyOnly() throws Exception {
        status = 200;
        response = "{\"status\":\"approved\",\"api_key\":\"loom_0123456789ABCDEFGHIJKLMN\"}";
        assertEquals("loom_0123456789ABCDEFGHIJKLMN", new PairingClient().poll(config, SECRET).key);
        assertEquals("POST /loom/api/v1/auth/pairing/poll HTTP/1.1", request);
        assertEquals(SECRET, new JSONObject(requestBody).getString("device_secret"));
        assertFalse(headers.containsKey("X-Api-Key"));
        status = 204;
        response = "";
        new PairingClient().cancel(config, SECRET);
        assertEquals("POST /loom/api/v1/auth/pairing/cancel HTTP/1.1", request);
        assertEquals(SECRET, new JSONObject(requestBody).getString("device_secret"));
    }

    @Test public void pendingAndDeniedAreExplicit() throws Exception {
        status = 200;
        for (String state : new String[]{"pending", "denied"}) {
            response = "{\"status\":\"" + state + "\"}";
            assertEquals(state, new PairingClient().poll(config, SECRET).status);
        }
    }

    @Test public void redirectsNeverFollowOrDiscloseServerBody() {
        status = 307;
        response = "sensitive upstream " + SECRET;
        IOException failure = assertThrows(IOException.class, () -> new PairingClient().start(config));
        assertTrue(failure.getMessage().contains("redirected"));
        assertFalse(failure.getMessage().contains(SECRET));
        assertEquals(1, requests.get());
    }

    @Test public void oldDisabledExpiredServersHaveActionableMessages() {
        for (int code : new int[]{404, 405, 410, 503}) {
            status = code;
            response = "{\"error\":\"sensitive server detail\"}";
            IOException failure = assertThrows(IOException.class, () -> new PairingClient().start(config));
            String expected = code == 410 ? "expired" : code == 503 ? "authentication" : "too old";
            assertTrue(failure.getMessage(), failure.getMessage().contains(expected));
            assertFalse(failure.getMessage().contains("sensitive"));
        }
    }

    @Test public void rateLimitIncludesSafeDelay() {
        status = 429;
        PairingClient.Retry retry = assertThrows(PairingClient.Retry.class,
                () -> new PairingClient().poll(config, SECRET));
        assertEquals(5, retry.seconds);
        assertEquals(5, PairingClient.retrySeconds("-1"));
        assertEquals(5, PairingClient.retrySeconds(null));
        assertEquals(5, PairingClient.retrySeconds("nonsense"));
        assertEquals(10, PairingClient.retrySeconds("10"));
        assertEquals(3600, PairingClient.retrySeconds("99999"));
    }

    @Test public void rejectsForeignOrEscapingVerificationPathsAndMalformedOffers() throws Exception {
        for (String path : new String[]{"https://evil.example/pair", "//evil.example/pair", "/../pair",
                "/pair?code=ABCD-EFGH&secret=x", "/pair?code=ZZZZ-ZZZZ", "/%70air?code=ABCD-EFGH"}) {
            JSONObject json = offer().put("verification_path", path);
            assertThrows(IOException.class, () -> PairingClient.parseOffer(config, json));
        }
        for (JSONObject json : new JSONObject[]{
                offer().put("device_secret", ""), offer().put("user_code", "bad"),
                offer().put("expires_in", -1), offer().put("expires_in", "600"),
                offer().put("interval", 0), offer().put("interval", 1.5)}) {
            assertThrows(IOException.class, () -> PairingClient.parseOffer(config, json));
        }
        assertEquals(5, PairingClient.parseOffer(config, offer().put("interval", 1)).interval);
    }

    @Test public void malformedApprovedKeyCannotBePersisted() {
        status = 200;
        for (String body : new String[]{"{}", "{\"status\":\"approved\"}",
                "{\"status\":\"approved\",\"api_key\":\"\"}",
                "{\"status\":\"approved\",\"api_key\":\"not-a-loom-key\"}",
                "{\"status\":\"approved\",\"api_key\":\"bad key\"}",
                "{\"status\":\"approved\",\"api_key\":\" key \"}", "{\"status\":\"unknown\"}"}) {
            response = body;
            assertThrows(IOException.class, () -> new PairingClient().poll(config, SECRET));
        }
    }

    @Test public void boundedResponseAndWrongSuccessStatusFailClosed() {
        response = "x".repeat(65537);
        assertThrows(IOException.class, () -> new PairingClient().start(config));
        response = offer().toString();
        status = 200;
        assertThrows(IOException.class, () -> new PairingClient().start(config));
    }
}
