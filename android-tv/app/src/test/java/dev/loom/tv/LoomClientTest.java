package dev.loom.tv;

import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.net.InetAddress;
import java.net.ServerSocket;
import java.net.Socket;
import java.nio.charset.StandardCharsets;
import java.util.Map;
import java.util.TreeMap;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;
import org.junit.After;
import org.junit.Before;
import org.junit.Test;
import static org.junit.Assert.*;

/** Real local HTTP server; no Android stubs and no production server or credentials. */
public class LoomClientTest {
    private ServerSocket server;
    private Thread worker;
    private String base;
    private volatile int code = 200;
    private volatile byte[] body = "{\"data\":[],\"total\":0}".getBytes(StandardCharsets.UTF_8);
    private volatile String redirect;
    private final AtomicReference<String> request = new AtomicReference<>();
    private final AtomicReference<Map<String, String>> headers = new AtomicReference<>();
    private final AtomicInteger requests = new AtomicInteger();

    @Before public void start() throws Exception {
        server = new ServerSocket(0, 8, InetAddress.getByName("127.0.0.1"));
        base = "http://127.0.0.1:" + server.getLocalPort();
        worker = new Thread(() -> {
            while (!server.isClosed()) {
                try (Socket socket = server.accept()) {
                    socket.setSoTimeout(5000);
                    BufferedReader reader = new BufferedReader(new InputStreamReader(
                            socket.getInputStream(), StandardCharsets.US_ASCII));
                    request.set(reader.readLine());
                    Map<String, String> received = new TreeMap<>(String.CASE_INSENSITIVE_ORDER);
                    String line;
                    while ((line = reader.readLine()) != null && !line.isEmpty()) {
                        int colon = line.indexOf(':');
                        if (colon > 0) received.put(line.substring(0, colon), line.substring(colon + 1).trim());
                    }
                    headers.set(received);
                    requests.incrementAndGet();
                    OutputStream out = socket.getOutputStream();
                    String head = "HTTP/1.1 " + code + " Test\r\nContent-Type: application/json\r\n"
                            + "Content-Length: " + body.length + "\r\nConnection: close\r\n"
                            + (redirect == null ? "" : "Location: " + redirect + "\r\n") + "\r\n";
                    out.write(head.getBytes(StandardCharsets.US_ASCII));
                    out.write(body);
                    out.flush();
                } catch (java.io.IOException expectedOnCloseOrSizeLimit) {
                    // Closing the fixture or rejecting an oversized response closes the socket.
                }
            }
        }, "loom-test-http");
        worker.setDaemon(true);
        worker.start();
    }

    @After public void stop() throws Exception {
        server.close();
        worker.join(6000);
        assertFalse("HTTP fixture must terminate", worker.isAlive());
    }

    @Test public void sendsKeyOnlyAsHeaderAndUsesMoviePagination() throws Exception {
        new LoomClient().load(new ServerConfig(base + "/prefix", "test-key", true), true, 24);
        assertEquals("GET /prefix/api/v1/movies?limit=24&offset=24 HTTP/1.1", request.get());
        assertEquals("test-key", headers.get().get("X-Api-Key"));
    }

    @Test public void neverFollowsRedirectOrLeaksCredentials() {
        code = 302;
        redirect = base + "/collector";
        LoomClient.Failure error = assertThrows(LoomClient.Failure.class,
                () -> new LoomClient().load(new ServerConfig(base, "private-key", true), false, 0));
        assertTrue(error.getMessage().contains("redirected"));
        assertFalse(error.getMessage().contains("private-key"));
        assertEquals(1, requests.get());
        assertEquals("GET /api/v1/series HTTP/1.1", request.get());
    }

    @Test public void exposesAuthErrorNotServerBody() {
        code = 401;
        body = "sensitive upstream detail".getBytes(StandardCharsets.UTF_8);
        LoomClient.Failure error = assertThrows(LoomClient.Failure.class,
                () -> new LoomClient().load(new ServerConfig(base, "", true), false, 0));
        assertTrue(error.getMessage().contains("401"));
        assertFalse(error.getMessage().contains("sensitive"));
    }

    @Test public void malformedSuccessIsNotAnEmptyLibrary() {
        body = "{}".getBytes(StandardCharsets.UTF_8);
        assertThrows(LoomClient.Failure.class,
                () -> new LoomClient().load(new ServerConfig(base, "", true), false, 0));
    }

    @Test public void oversizedResponseIsRejectedBeforeParsing() {
        body = new byte[8 * 1024 * 1024 + 1];
        LoomClient.Failure error = assertThrows(LoomClient.Failure.class,
                () -> new LoomClient().load(new ServerConfig(base, "", true), false, 0));
        assertTrue(error.getMessage().contains("8 MiB"));
    }
}
