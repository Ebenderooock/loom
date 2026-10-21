package dev.loom.tv;

import org.json.JSONException;
import org.junit.Test;
import static org.junit.Assert.*;

public class CatalogTest {
    @Test public void parsesActualCamelCaseEnvelopeAndDetails() throws Exception {
        Catalog catalog = Catalog.parse("{\"data\":[{\"id\":\"a\",\"title\":\"Title\",\"year\":2024,"
                + "\"overview\":\"An overview\",\"status\":\"available_right_quality\","
                + "\"runtime\":100,\"rating\":7.5,\"monitoringStatus\":\"monitored\"}],\"total\":42}");
        assertEquals(42, catalog.total);
        assertEquals("2024  ·  available right quality", catalog.items.get(0).caption());
        assertTrue(catalog.items.get(0).details().contains("An overview"));
        assertTrue(catalog.items.get(0).details().contains("does not provide a playable stream"));
    }

    @Test public void emptyIsNotFailure() throws Exception {
        assertTrue(Catalog.parse("{\"data\":[],\"total\":0}").items.isEmpty());
    }

    @Test public void rejectsWrongEnvelopesAndIncompleteRecords() {
        for (String body : new String[]{"[]", "{}", "<html>Login</html>",
                "{\"data\":[],\"total\":-1}", "{\"data\":[{}],\"total\":1}",
                "{\"data\":[{\"id\":\"a\",\"title\":\"A\"}],\"total\":0}"}) {
            assertThrows(JSONException.class, () -> Catalog.parse(body));
        }
    }

    @Test public void pagesSeriesWithoutLosingTail() throws Exception {
        StringBuilder json = new StringBuilder("{\"data\":[");
        for (int i = 0; i < 51; i++) {
            if (i > 0) json.append(',');
            json.append("{\"id\":\"").append(i).append("\",\"title\":\"Title ").append(i).append("\"}");
        }
        Catalog catalog = Catalog.parse(json.append("],\"total\":51}").toString());
        assertEquals(24, catalog.page(0).items.size());
        assertEquals("Title 24", catalog.page(24).items.get(0).title);
        assertEquals(3, catalog.page(48).items.size());
        assertEquals(51, catalog.page(48).total);
        assertTrue(catalog.page(99).items.isEmpty());
    }
}
