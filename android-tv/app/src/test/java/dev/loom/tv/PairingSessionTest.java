package dev.loom.tv;

import java.io.IOException;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import org.junit.Test;
import static org.junit.Assert.*;

public class PairingSessionTest {
    private static final ServerConfig CONFIG = new ServerConfig("https://loom.example/prefix", "", false);
    private final List<Integer> waits = new ArrayList<>();
    private long now;
    private int polls, cancels, approvals, failures, offers;
    private String error;
    private PairingSession session;
    private PairingClient.Offer offer = new PairingClient.Offer("synthetic-device-secret-not-a-real-key",
            "ABCD-EFGH", CONFIG.baseUrl + "/pair?code=ABCD-EFGH", 600, 5);
    private final PairingSession.Listener listener = new PairingSession.Listener() {
        public void offered(PairingClient.Offer value) { offers++; }
        public void waiting(int remaining, int delay) { }
        public void approved(ServerConfig config) {
            approvals++;
            assertEquals(CONFIG.baseUrl, config.baseUrl);
            assertEquals("synthetic-api-key", config.apiKey);
        }
        public void failed(String message) { failures++; error = message; }
    };
    private class Protocol implements PairingSession.Protocol {
        public PairingClient.Offer start(ServerConfig config) throws IOException { return offer; }
        public PairingClient.Poll poll(ServerConfig config, String secret) throws IOException {
            polls++;
            assertEquals(offer.secret, secret);
            return new PairingClient.Poll("approved", "synthetic-api-key");
        }
        public void cancel(ServerConfig config, String secret) { cancels++; assertEquals(offer.secret, secret); }
    }
    private PairingSession create(PairingSession.Protocol protocol) {
        session = new PairingSession(CONFIG, listener, protocol, () -> now, seconds -> {
            waits.add(seconds);
            now += TimeUnit.SECONDS.toNanos(seconds);
        });
        return session;
    }

    @Test public void pendingThenApprovedOnlyAfterPollIntervals() {
        create(new Protocol() {
            @Override public PairingClient.Poll poll(ServerConfig config, String secret) throws IOException {
                if (polls++ == 0) return new PairingClient.Poll("pending", "");
                return new PairingClient.Poll("approved", "synthetic-api-key");
            }
        }).run();
        assertEquals(List.of(5, 5), waits);
        assertEquals(1, approvals);
        assertEquals(0, cancels);
    }

    @Test public void retryAfterCannotAcceleratePolling() {
        create(new Protocol() {
            @Override public PairingClient.Poll poll(ServerConfig config, String secret) throws IOException {
                if (polls++ == 0) throw new PairingClient.Retry(12);
                return new PairingClient.Poll("approved", "synthetic-api-key");
            }
        }).run();
        assertEquals(List.of(5, 12), waits);
        assertEquals(1, approvals);
    }

    @Test public void expiryStopsWithoutExtraPollAndCancels() {
        offer = new PairingClient.Offer(offer.secret, offer.code, offer.verificationUrl, 10, 5);
        create(new Protocol() {
            @Override public PairingClient.Poll poll(ServerConfig config, String secret) {
                polls++;
                return new PairingClient.Poll("pending", "");
            }
        }).run();
        assertEquals(1, polls);
        assertEquals(1, cancels);
        assertTrue(error.contains("expired"));
        assertEquals(0, approvals);
    }

    @Test public void denialIsNotSavedAndCancels() {
        create(new Protocol() {
            @Override public PairingClient.Poll poll(ServerConfig config, String secret) {
                return new PairingClient.Poll("denied", "");
            }
        }).run();
        assertTrue(error.contains("denied"));
        assertEquals(1, failures);
        assertEquals(0, approvals);
        assertEquals(1, cancels);
    }

    @Test public void cancelledStartResponseIsCleanedUpWithoutOfferingOrPolling() {
        create(new Protocol() {
            @Override public PairingClient.Offer start(ServerConfig config) {
                session.cancel();
                return offer;
            }
        }).run();
        assertEquals(0, offers);
        assertEquals(0, polls);
        assertEquals(0, approvals);
        assertEquals(1, cancels);
    }

    @Test public void cancelledPollCannotDeliverCredential() {
        create(new Protocol() {
            @Override public PairingClient.Poll poll(ServerConfig config, String secret) {
                session.cancel();
                return new PairingClient.Poll("approved", "synthetic-api-key");
            }
        }).run();
        assertEquals(0, approvals);
        assertEquals(0, failures);
        assertEquals(1, cancels);
    }

    @Test public void approvalArrivingAfterLocalExpiryCannotSave() {
        create(new Protocol() {
            @Override public PairingClient.Poll poll(ServerConfig config, String secret) {
                now = TimeUnit.SECONDS.toNanos(601);
                return new PairingClient.Poll("approved", "synthetic-api-key");
            }
        }).run();
        assertEquals(0, approvals);
        assertTrue(error.contains("expired"));
        assertEquals(1, cancels);
    }

    @Test public void cancellationWakesRealWaitWithoutWaitingForPollInterval() throws Exception {
        CountDownLatch waiting = new CountDownLatch(1);
        PairingSession.Listener watching = new PairingSession.Listener() {
            public void offered(PairingClient.Offer value) { }
            public void waiting(int remaining, int delay) { waiting.countDown(); }
            public void approved(ServerConfig config) { fail("cancelled attempt saved"); }
            public void failed(String message) { fail(message); }
        };
        offer = new PairingClient.Offer(offer.secret, offer.code, offer.verificationUrl, 600, 300);
        session = new PairingSession(CONFIG, watching, new Protocol(), System::nanoTime, null);
        Thread thread = new Thread(session);
        thread.start();
        try {
            assertTrue(waiting.await(2, TimeUnit.SECONDS));
            session.cancel();
            thread.join(2000);
            assertFalse(thread.isAlive());
            assertEquals(0, polls);
            assertEquals(1, cancels);
        } finally {
            session.cancel();
            thread.join(2000);
        }
    }
}
