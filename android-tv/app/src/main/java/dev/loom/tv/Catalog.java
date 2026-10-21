package dev.loom.tv;

import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;
import java.util.ArrayList;
import java.util.List;

final class Catalog {
    static final int PAGE_SIZE = 24;

    static final class Item {
        final String title, overview, status, network, monitoring;
        final int year, runtime;
        final double rating;

        Item(JSONObject json) throws JSONException {
            // Required fields distinguish a real catalog from unrelated JSON/login responses.
            if (json.getString("id").isEmpty() || json.getString("title").isEmpty()) {
                throw new JSONException("Missing identity");
            }
            title = json.getString("title");
            overview = json.optString("overview", "");
            status = json.optString("status", "");
            network = json.optString("network", "");
            monitoring = json.optString("monitoringStatus", "");
            year = json.optInt("year", 0);
            runtime = json.optInt("runtime", 0);
            rating = json.optDouble("rating", 0);
        }

        String caption() {
            return (year > 0 ? year + "  ·  " : "") + status.replace('_', ' ');
        }

        String details() {
            return caption() + (runtime > 0 ? "\nRuntime: " + runtime + " min" : "")
                    + (rating > 0 ? "\nRating: " + rating : "")
                    + (network.isEmpty() ? "" : "\nNetwork: " + network)
                    + (monitoring.isEmpty() ? "" : "\nMonitoring: " + monitoring)
                    + "\n\n" + (overview.isEmpty() ? "No overview supplied by Loom." : overview)
                    + "\n\nBrowsing only. Loom manages media; this API does not provide a playable stream. "
                    + "Open your configured media player from Apps to watch.";
        }
    }

    final List<Item> items;
    final int total;

    Catalog(List<Item> items, int total) {
        this.items = items;
        this.total = total;
    }

    static Catalog parse(String body) throws JSONException {
        JSONObject root = new JSONObject(body);
        JSONArray data = root.getJSONArray("data");
        int total = root.getInt("total");
        if (total < data.length() || total < 0) throw new JSONException("Invalid total");
        List<Item> items = new ArrayList<>();
        for (int i = 0; i < data.length(); i++) items.add(new Item(data.getJSONObject(i)));
        return new Catalog(items, total);
    }

    Catalog page(int offset) {
        int start = Math.min(Math.max(0, offset), items.size());
        return new Catalog(new ArrayList<>(items.subList(start,
                Math.min(start + PAGE_SIZE, items.size()))), total);
    }
}
