package dev.loom.tv;

import java.net.URI;
import java.net.URISyntaxException;
import java.util.Locale;

/** Pure validation shared by configuration, requests and local JVM tests. */
final class ServerConfig {
    final String baseUrl;
    final String apiKey;
    final boolean allowHttp;

    ServerConfig(String url, String key, boolean allowHttp) {
        try {
            String entered = url.trim();
            // Bare LAN addresses deliberately mean HTTP, never silently HTTPS or consent.
            if (!entered.isEmpty() && !entered.contains("://")) entered = "http://" + entered;
            URI uri = new URI(entered);
            String scheme = uri.getScheme() == null ? "" : uri.getScheme().toLowerCase(Locale.ROOT);
            if (!scheme.equals("https") && !scheme.equals("http")) {
                throw new IllegalArgumentException("Use an http:// or https:// server URL.");
            }
            if (uri.getHost() == null || uri.getRawUserInfo() != null
                    || uri.getRawQuery() != null || uri.getRawFragment() != null
                    || uri.getPort() == 0 || uri.getPort() > 65535) {
                throw new IllegalArgumentException("Use a server address without credentials, query or fragment.");
            }
            String path = uri.getRawPath() == null ? "" : uri.getRawPath();
            if (!path.matches("[A-Za-z0-9/_-]*") || path.contains("//")) {
                throw new IllegalArgumentException("Server prefix may contain letters, numbers, /, _ and - only.");
            }
            if (scheme.equals("http") && !allowHttp) {
                throw new IllegalArgumentException("HTTP sends API keys and pairing secrets unencrypted. Enable trusted-network HTTP or use HTTPS.");
            }
            baseUrl = scheme + "://" + uri.getRawAuthority() + path.replaceAll("/+$", "");
        } catch (URISyntaxException e) {
            throw new IllegalArgumentException("Enter a valid server URL.");
        }
        String trimmedKey = key.trim();
        for (int i = 0; i < trimmedKey.length(); i++) {
            if (trimmedKey.charAt(i) < 33 || trimmedKey.charAt(i) > 126) {
                throw new IllegalArgumentException("API key must not contain spaces or control characters.");
            }
        }
        apiKey = trimmedKey;
        this.allowHttp = allowHttp;
    }
}
