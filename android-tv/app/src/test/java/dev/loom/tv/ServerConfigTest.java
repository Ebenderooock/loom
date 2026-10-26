package dev.loom.tv;

import org.junit.Test;
import static org.junit.Assert.*;

public class ServerConfigTest {
    @Test public void normalizesPrefixAndAllowsEmptyKey() {
        ServerConfig config = new ServerConfig(" https://loom.example:1925/loom/ ", "", false);
        assertEquals("https://loom.example:1925/loom", config.baseUrl);
        assertEquals("", config.apiKey);
    }

    @Test public void httpRequiresExplicitConsent() {
        rejects("http://192.168.1.5:1925", "key", false);
        assertEquals("http://192.168.1.5:1925",
                new ServerConfig("http://192.168.1.5:1925/", "key", true).baseUrl);
    }

    @Test public void bareAddressesMeanHttpOnlyWithExplicitConsent() {
        for (String host : new String[]{"192.168.1.20:1925", "loom.local:1925/prefix", "[::1]:1925"}) {
            rejects(host, "", false);
            assertEquals("http://" + host, new ServerConfig(host, "", true).baseUrl);
        }
        rejects("user:password@loom.local", "", true);
        rejects("ftp://loom.local", "", true);
    }

    @Test public void supportsIpv6() {
        assertEquals("https://[::1]:1925", new ServerConfig("https://[::1]:1925", "", false).baseUrl);
    }

    @Test public void rejectsAmbiguousOrCredentialBearingUrls() {
        for (String url : new String[]{"", "loom.local", "file:///tmp/loom", "ftp://loom.local",
                "https://user:password@loom.local", "https://loom.local?apikey=secret",
                "https://loom.local/#secret", "https://loom.local/a/../b", "https://loom.local/%2f",
                "https://loom.local//api", "https://loom.local:99999", "https://loom.local:0"}) {
            rejects(url, "", false);
        }
    }

    @Test public void rejectsHeaderInjection() {
        rejects("https://loom.local", "key\r\nInjected: yes", false);
        rejects("https://loom.local", "key with spaces", false);
    }

    private static void rejects(String url, String key, boolean http) {
        assertThrows(IllegalArgumentException.class, () -> new ServerConfig(url, key, http));
    }
}
