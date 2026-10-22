package dev.loom.tv;

import java.io.IOException;
import java.util.concurrent.TimeUnit;

/** One in-memory attempt, owned by a dialog. Listener must guard UI delivery by identity. */
final class PairingSession implements Runnable {
    interface Protocol {
        PairingClient.Offer start(ServerConfig config) throws IOException;
        PairingClient.Poll poll(ServerConfig config, String secret) throws IOException;
        void cancel(ServerConfig config, String secret) throws IOException;
    }
    interface Listener {
        void offered(PairingClient.Offer offer);
        void waiting(int remainingSeconds, int delaySeconds);
        void approved(ServerConfig config);
        void failed(String message);
    }
    interface Clock { long now(); }
    interface Waiter { void pause(int seconds) throws InterruptedException; }

    private final Protocol protocol;
    private final ServerConfig config;
    private final Listener listener;
    private final Clock clock;
    private final Waiter waiter;
    private volatile boolean cancelled;

    PairingSession(ServerConfig config, Listener listener) {
        this(config, listener, new PairingClient(), System::nanoTime, null);
    }

    PairingSession(ServerConfig config, Listener listener, Protocol protocol, Clock clock, Waiter waiter) {
        this.config = config;
        this.listener = listener;
        this.protocol = protocol;
        this.clock = clock;
        this.waiter = waiter == null ? this::pause : waiter;
    }

    synchronized void cancel() {
        cancelled = true;
        notifyAll();
    }

    private synchronized void pause(int seconds) throws InterruptedException {
        long until = System.nanoTime() + TimeUnit.SECONDS.toNanos(seconds);
        while (!cancelled) {
            long remaining = until - System.nanoTime();
            if (remaining <= 0) break;
            TimeUnit.NANOSECONDS.timedWait(this, remaining);
        }
    }

    @Override public void run() {
        PairingClient.Offer offer = null;
        boolean consumed = false;
        try {
            if (cancelled) return;
            long started = clock.now();
            offer = protocol.start(config);
            long deadline = started + TimeUnit.SECONDS.toNanos(offer.expiresIn);
            if (cancelled) return;
            listener.offered(offer);
            int delay = offer.interval;
            while (!cancelled) {
                long remaining = deadline - clock.now();
                if (remaining <= 0) throw new IOException("Pairing expired. Start again for a new code.");
                int seconds = (int) Math.ceil(remaining / 1_000_000_000.0);
                listener.waiting(seconds, delay);
                waiter.pause(Math.min(delay, seconds));
                if (cancelled) return;
                if (clock.now() >= deadline) throw new IOException("Pairing expired. Start again for a new code.");
                try {
                    PairingClient.Poll result = protocol.poll(config, offer.secret);
                    if (cancelled) return;
                    if (clock.now() >= deadline) throw new IOException("Pairing expired. Start again for a new code.");
                    if (result.status.equals("approved")) {
                        consumed = true;
                        listener.approved(new ServerConfig(config.baseUrl, result.key, config.allowHttp));
                        return;
                    }
                    if (result.status.equals("denied")) throw new IOException("Pairing denied. Start again only if you intended to connect this TV.");
                    delay = offer.interval;
                } catch (PairingClient.Retry retry) {
                    delay = Math.max(offer.interval, retry.seconds);
                }
            }
        } catch (IOException e) {
            if (!cancelled) listener.failed(e.getMessage());
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        } finally {
            if (offer != null && !consumed) {
                // Also runs if cancellation happens while start/poll is in flight.
                try { protocol.cancel(config, offer.secret); } catch (IOException ignored) { /* best effort */ }
            }
        }
    }
}
