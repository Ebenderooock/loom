package dev.loom.tv;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.ActivityNotFoundException;
import android.content.ComponentName;
import android.content.Intent;
import android.content.pm.ResolveInfo;
import android.content.res.ColorStateList;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.graphics.drawable.StateListDrawable;
import android.media.tv.TvContract;
import android.media.tv.TvInputInfo;
import android.media.tv.TvInputManager;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.provider.Settings;
import android.text.InputType;
import android.view.Gravity;
import android.view.View;
import android.view.WindowManager;
import android.widget.Button;
import android.widget.CheckBox;
import android.widget.EditText;
import android.widget.HorizontalScrollView;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextClock;
import android.widget.TextView;

import java.io.IOException;
import java.security.GeneralSecurityException;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;

/** Native, D-pad-first home. No streaming, firmware modification or boot takeover. */
public final class HomeActivity extends Activity {
    private static final int INK = 0xFF0B1220, SURFACE = 0xFF172439, FOCUS_SURFACE = 0xFF233D5C;
    private static final int WHITE = 0xFFF5F2EA, MUTED = 0xFFB8C7DB, BLUE = 0xFF89BFFF;
    private final ExecutorService io = Executors.newFixedThreadPool(4);
    private final Handler main = new Handler(Looper.getMainLooper());
    private final List<Future<?>> tasks = new ArrayList<>();
    private ConfigStore store;
    private ServerConfig config;
    private TextView connection;
    private Shelf movies, series;
    private LinearLayout appRow, inputRow;
    private TextView appStatus, inputStatus;
    private TvInputManager inputs;
    private int generation;
    private boolean active;
    private boolean inputCallbackRegistered;
    private AlertDialog dialog;
    private PairingSession pairing;
    private int pairingRequest;

    private final TvInputManager.TvInputCallback inputCallback = new TvInputManager.TvInputCallback() {
        @Override public void onInputStateChanged(String id, int state) { refreshInputs(); }
        @Override public void onInputAdded(String id) { refreshInputs(); }
        @Override public void onInputRemoved(String id) { refreshInputs(); }
        @Override public void onInputUpdated(String id) { refreshInputs(); }
    };

    @Override public void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        store = new ConfigStore(this);
        String initialStatus = "Connect a Loom server to browse. Apps, inputs and settings work without it.";
        try {
            config = store.load();
        } catch (GeneralSecurityException e) {
            initialStatus = "Saved credentials could not be unlocked. Edit Server or Forget server to reconnect.";
        }
        inputs = (TvInputManager) getSystemService(TV_INPUT_SERVICE);
        LinearLayout root = column();
        root.setPadding(dp(32), dp(24), dp(32), dp(32));
        root.setBackgroundColor(INK);
        ScrollView scroll = new ScrollView(this);
        scroll.setFillViewport(true);
        scroll.addView(root);
        setContentView(scroll);

        LinearLayout header = row();
        TextView brand = text("+  Loom", 34, WHITE);
        brand.setTypeface(Typeface.create("sans-serif-medium", Typeface.NORMAL));
        header.addView(brand, new LinearLayout.LayoutParams(0, -2, 1));
        TextClock clock = new TextClock(this);
        clock.setTextSize(22);
        clock.setTextColor(MUTED);
        clock.setTypeface(Typeface.MONOSPACE);
        header.addView(clock);
        root.addView(header);
        root.addView(text("Your media. Your TV.", 20, MUTED));

        LinearLayout actions = shelfRow(root);
        Button server = button("Server", v -> showConfig());
        actions.addView(server);
        actions.addView(button("Inputs", v -> systemInputs()));
        actions.addView(button("TV settings", v -> launchSettings()));
        actions.addView(button("Home setup", v -> homeSetup()));
        connection = text(initialStatus, 18, MUTED);
        connection.setAccessibilityLiveRegion(View.ACCESSIBILITY_LIVE_REGION_POLITE);
        root.addView(connection);

        movies = new Shelf(root, "Movies", true);
        series = new Shelf(root, "Series", false);
        heading(root, "Apps");
        appStatus = text("Finding installed TV apps…", 18, MUTED);
        root.addView(appStatus);
        appRow = shelfRow(root);
        heading(root, "Inputs");
        inputStatus = text("Reading TV inputs…", 18, MUTED);
        root.addView(inputStatus);
        inputRow = shelfRow(root);
        LinearLayout inputActions = shelfRow(root);
        inputActions.addView(button("System input picker", v -> systemInputs()));
        inputActions.addView(button("Input settings", v -> inputSettings()));
        inputActions.addView(button("Refresh apps & inputs", v -> refreshDevice()));
        root.addView(text("D-pad to browse  ·  OK to open  ·  Back to close details", 16, MUTED));
        server.requestFocus();
    }

    @Override protected void onStart() {
        super.onStart();
        active = true;
        if (inputs != null) {
            try {
                inputs.registerCallback(inputCallback, main);
                inputCallbackRegistered = true;
            } catch (SecurityException | IllegalStateException e) {
                inputStatus.setText("This TV restricts input access. Use System input picker or Input settings.");
            }
        }
        refreshDevice();
        if (!movies.loaded) load(movies, movies.offset);
        if (!series.loaded) load(series, series.offset);
    }

    @Override protected void onStop() {
        active = false;
        stopPairing();
        if (dialog != null) dialog.dismiss();
        generation++;
        for (Future<?> task : tasks) task.cancel(true);
        tasks.clear();
        if (movies.busy) movies.loaded = false;
        if (series.busy) series.loaded = false;
        movies.busy = false;
        series.busy = false;
        movies.retry.setEnabled(true);
        series.retry.setEnabled(true);
        if (inputCallbackRegistered) {
            inputs.unregisterCallback(inputCallback);
            inputCallbackRegistered = false;
        }
        super.onStop();
    }

    @Override protected void onDestroy() {
        if (dialog != null) dialog.dismiss();
        io.shutdownNow();
        main.removeCallbacksAndMessages(null);
        super.onDestroy();
    }

    private void submit(Runnable task) {
        tasks.removeIf(Future::isDone);
        tasks.add(io.submit(task));
    }

    private void deliver(int token, Runnable action) {
        main.post(() -> {
            if (active && token == generation && !isDestroyed()) action.run();
        });
    }

    private final class Shelf {
        final boolean isMovies;
        final TextView status;
        final LinearLayout cards;
        final Button retry, previous, next;
        int offset;
        int request;
        boolean busy, loaded;
        Catalog allSeries;

        Shelf(LinearLayout root, String title, boolean isMovies) {
            this.isMovies = isMovies;
            heading(root, title);
            status = text("Waiting for server configuration.", 18, MUTED);
            status.setAccessibilityLiveRegion(View.ACCESSIBILITY_LIVE_REGION_POLITE);
            root.addView(status);
            cards = shelfRow(root);
            LinearLayout controls = shelfRow(root);
            retry = button("Refresh / Retry", v -> load(this, offset));
            previous = button("Previous", v -> load(this, Math.max(0, offset - Catalog.PAGE_SIZE)));
            next = button("Next", v -> load(this, offset + Catalog.PAGE_SIZE));
            controls.addView(retry);
            controls.addView(previous);
            controls.addView(next);
            previous.setVisibility(View.GONE);
            next.setVisibility(View.GONE);
        }
    }

    private void load(Shelf shelf, int offset) {
        if (shelf.busy) return;
        if (config == null) {
            shelf.status.setText("No server configured. Open Server to connect; Apps and TV controls remain available.");
            return;
        }
        ServerConfig snapshot = config;
        int token = generation;
        int request = ++shelf.request;
        shelf.busy = true;
        shelf.retry.setEnabled(false);
        shelf.previous.setEnabled(false);
        shelf.next.setEnabled(false);
        shelf.status.setText("Loading " + (shelf.isMovies ? "movies" : "series") + "… Apps and TV controls are still available.");
        connection.setText("Loom catalog · " + (snapshot.allowHttp ? "HTTP enabled — trusted network only" : "HTTPS"));
        // Series has no server pagination. Cache the real list in memory and render only 24 views.
        Catalog cachedSeries = !shelf.isMovies && offset != shelf.offset ? shelf.allSeries : null;
        submit(() -> {
            try {
                Catalog catalog = cachedSeries != null ? cachedSeries : new LoomClient().load(snapshot, shelf.isMovies, offset);
                deliver(token, () -> {
                    if (request != shelf.request) return;
                    if (!shelf.isMovies) shelf.allSeries = catalog;
                    showPage(shelf, shelf.isMovies ? catalog : catalog.page(offset), offset);
                });
            } catch (IOException e) {
                deliver(token, () -> {
                    if (request != shelf.request) return;
                    shelf.busy = false;
                    shelf.retry.setEnabled(true);
                    shelf.previous.setEnabled(shelf.offset > 0);
                    shelf.next.setEnabled(shelf.loaded);
                    shelf.status.setText(e instanceof LoomClient.Failure ? e.getMessage()
                            : "Cannot reach Loom. Check the server address and network, then Retry.");
                    if (shelf.loaded) shelf.status.append(" Previously loaded titles below may be out of date.");
                });
            }
        });
    }

    private void showPage(Shelf shelf, Catalog page, int offset) {
        boolean restore = shelf.cards.hasFocus() || shelf.next.hasFocus() || shelf.previous.hasFocus();
        shelf.busy = false;
        shelf.loaded = true;
        shelf.offset = offset;
        shelf.cards.removeAllViews();
        for (Catalog.Item item : page.items) {
            Button card = button(item.title + "\n" + item.caption(), v -> showDetails(item));
            card.setGravity(Gravity.START | Gravity.CENTER_VERTICAL);
            card.setMaxLines(4);
            shelf.cards.addView(card, cardParams());
        }
        shelf.status.setText(page.items.isEmpty()
                ? (page.total == 0 ? "No titles in this Loom library yet. Add media using Loom web."
                : "No titles on this page. Use Previous or Refresh.")
                : "Titles " + (offset + 1) + "–" + (offset + page.items.size()) + " of " + page.total
                + " · Select for details · Browsing only");
        shelf.retry.setEnabled(true);
        shelf.previous.setEnabled(true);
        shelf.next.setEnabled(true);
        shelf.previous.setVisibility(offset > 0 ? View.VISIBLE : View.GONE);
        shelf.next.setVisibility(offset + page.items.size() < page.total && !page.items.isEmpty()
                ? View.VISIBLE : View.GONE);
        if (restore) {
            if (shelf.cards.getChildCount() > 0) shelf.cards.getChildAt(0).requestFocus();
            else shelf.retry.requestFocus();
        }
    }

    private void showDetails(Catalog.Item item) {
        LinearLayout body = dialogBody();
        body.addView(text(item.details(), 20, WHITE));
        body.addView(button("Close details", v -> dialog.dismiss()));
        showDialog(item.title, body);
    }

    private void showConfig() {
        showConfig(config);
    }

    private void showConfig(ServerConfig draft) {
        LinearLayout body = dialogBody();
        body.addView(text("Connect to Loom", 26, WHITE));
        body.addView(text("Server URL (include any reverse-proxy prefix)", 18, MUTED));
        EditText url = field("https://loom.example", false);
        url.setContentDescription("Server URL");
        url.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_URI);
        url.setText(draft == null ? "" : draft.baseUrl);
        body.addView(url);
        body.addView(text("API key (blank only if server authentication is disabled)", 18, MUTED));
        EditText key = field("API key", true);
        key.setText(draft == null ? "" : draft.apiKey);
        body.addView(key);
        body.addView(text("Use a dedicated key from Loom web → Settings → API Key. "
                + "The key is encrypted on this TV; it is never sent to images or other apps.", 18, MUTED));
        CheckBox http = new CheckBox(this);
        http.setText("Allow unencrypted HTTP on my trusted network (exposes API keys and pairing secrets in transit)");
        http.setTextColor(WHITE);
        http.setTextSize(18);
        http.setMinHeight(dp(56));
        http.setBackground(focusBackground());
        http.setChecked(draft != null && draft.allowHttp);
        body.addView(http);
        TextView error = text("", 18, WHITE);
        error.setAccessibilityLiveRegion(View.ACCESSIBILITY_LIVE_REGION_POLITE);
        body.addView(error);
        body.addView(button("Pair with phone / browser", v -> {
            try {
                ServerConfig target = new ServerConfig(url.getText().toString(), "", http.isChecked());
                showPairing(target);
            } catch (IllegalArgumentException e) {
                error.setText(e.getMessage());
                url.requestFocus();
            }
        }));
        body.addView(text("Pairing needs server authentication. Approve the code on your own phone or computer; "
                + "no password is entered on the TV. Manual API key setup remains available.", 18, MUTED));
        body.addView(button("Save & connect", v -> {
            try {
                ServerConfig saved = new ServerConfig(url.getText().toString(), key.getText().toString(), http.isChecked());
                store.save(saved);
                config = saved;
                resetCatalog();
                dialog.dismiss();
                load(movies, 0);
                load(series, 0);
            } catch (IllegalArgumentException e) {
                error.setText(e.getMessage());
                url.requestFocus();
            } catch (GeneralSecurityException e) {
                error.setText("Could not securely save the key. Check TV storage and try again.");
            }
        }));
        body.addView(button("Forget server", v -> {
            store.clear();
            config = null;
            resetCatalog();
            connection.setText("Server forgotten. Apps, inputs and settings remain available.");
            movies.status.setText("No server configured.");
            series.status.setText("No server configured.");
            dialog.dismiss();
        }));
        body.addView(button("Cancel", v -> dialog.dismiss()));
        showDialog("Server", body);
        dialog.getWindow().addFlags(WindowManager.LayoutParams.FLAG_SECURE);
    }

    private void stopPairing() {
        pairingRequest++;
        if (pairing != null) pairing.cancel();
        pairing = null;
    }

    private void showPairing(ServerConfig target) {
        LinearLayout body = dialogBody();
        body.addView(text("1. Open the address below on your phone or computer.\n"
                + "2. Sign in to Loom and approve only this TV's code.", 22, WHITE));
        TextView address = text(target.baseUrl, 20, WHITE);
        TextView code = text("Requesting a pairing code…", 32, WHITE);
        code.setTypeface(Typeface.MONOSPACE);
        TextView status = text("Connecting using your selected server settings…", 18, MUTED);
        status.setAccessibilityLiveRegion(View.ACCESSIBILITY_LIVE_REGION_POLITE);
        body.addView(address);
        body.addView(code);
        body.addView(status);
        Button retry = button("Start again", v -> showPairing(target));
        retry.setVisibility(View.GONE);
        body.addView(retry);
        Button cancel = button("Cancel / manual setup", v -> { stopPairing(); showConfig(target); });
        body.addView(cancel);
        showDialog("Pair Loom TV", body);
        AlertDialog owner = dialog;
        owner.getWindow().addFlags(WindowManager.LayoutParams.FLAG_SECURE);
        int request = pairingRequest;
        PairingSession session = new PairingSession(target, new PairingSession.Listener() {
            private void update(Runnable change) {
                main.post(() -> {
                    if (active && dialog == owner && owner.isShowing()
                            && pairing != null && pairingRequest == request) change.run();
                });
            }
            @Override public void offered(PairingClient.Offer offer) {
                update(() -> {
                    address.setText(offer.verificationUrl);
                    code.setText(offer.code);
                });
            }
            @Override public void waiting(int remaining, int delay) {
                update(() -> status.setText("Waiting for approval · expires in " + remaining
                        + " seconds · next check in " + Math.min(delay, remaining) + " seconds."));
            }
            @Override public void approved(ServerConfig saved) {
                update(() -> {
                    try {
                        store.save(saved);
                        config = saved;
                        stopPairing();
                        resetCatalog();
                        owner.dismiss();
                        connection.setText("TV paired. Loading your library…");
                        load(movies, 0);
                        load(series, 0);
                    } catch (GeneralSecurityException e) {
                        status.setText("Approval received, but the key could not be securely saved. Start again or use manual setup.");
                        retry.setVisibility(View.VISIBLE);
                        code.setText("Not saved");
                    }
                });
            }
            @Override public void failed(String message) {
                update(() -> {
                    status.setText(message);
                    code.setText("Not connected");
                    retry.setVisibility(View.VISIBLE);
                    retry.requestFocus();
                });
            }
        });
        pairing = session;
        owner.setOnDismissListener(ignored -> {
            if (pairing == session) stopPairing();
        });
        cancel.requestFocus();
        io.execute(session);
    }

    private void resetCatalog() {
        generation++;
        for (Future<?> task : tasks) task.cancel(true);
        tasks.clear();
        for (Shelf shelf : new Shelf[]{movies, series}) {
            shelf.request++;
            shelf.loaded = false;
            shelf.busy = false;
            shelf.offset = 0;
            shelf.allSeries = null;
            shelf.cards.removeAllViews();
            shelf.previous.setVisibility(View.GONE);
            shelf.next.setVisibility(View.GONE);
            shelf.retry.setEnabled(true);
        }
        refreshDevice();
    }

    private void refreshDevice() {
        refreshApps();
        refreshInputs();
    }

    private void refreshApps() {
        if (!active) return;
        int token = generation;
        submit(() -> {
            try {
                Intent query = new Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LEANBACK_LAUNCHER);
                List<ResolveInfo> found = getPackageManager().queryIntentActivities(query, 0);
                found.sort(Comparator.comparing(a -> a.loadLabel(getPackageManager()).toString(), String.CASE_INSENSITIVE_ORDER));
                List<AppEntry> apps = new ArrayList<>();
                Set<String> seen = new HashSet<>();
                for (ResolveInfo result : found) {
                    ComponentName component = new ComponentName(result.activityInfo.packageName, result.activityInfo.name);
                    if (component.getPackageName().equals(getPackageName()) || !result.activityInfo.exported
                            || !result.activityInfo.enabled || !seen.add(component.flattenToString())) continue;
                    apps.add(new AppEntry(result.loadLabel(getPackageManager()).toString(), component));
                }
                deliver(token, () -> {
                    String focused = focusedTag(appRow);
                    appRow.removeAllViews();
                    for (AppEntry app : apps) {
                        Button card = button(app.label, v -> launch(new Intent(Intent.ACTION_MAIN)
                                .addCategory(Intent.CATEGORY_LEANBACK_LAUNCHER).setComponent(app.component)
                                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK), "This app could not be opened. It may have been removed or restricted."));
                        card.setTag(app.component.flattenToString());
                        appRow.addView(card, cardParams());
                    }
                    restoreTag(appRow, focused);
                    appStatus.setText(apps.isEmpty() ? "No launchable TV apps found. Use TV settings to manage apps."
                            : apps.size() + " installed TV apps · Players open independently of Loom");
                });
            } catch (RuntimeException e) {
                deliver(token, () -> appStatus.setText("Unable to list TV apps. Use TV settings, then Refresh apps & inputs."));
            }
        });
    }

    private static final class AppEntry {
        final String label;
        final ComponentName component;
        AppEntry(String label, ComponentName component) { this.label = label; this.component = component; }
    }

    private int inputRequest;

    private void refreshInputs() {
        if (!active) return;
        int token = generation;
        int request = ++inputRequest;
        submit(() -> {
            try {
                List<TvInputInfo> list = inputs == null ? new ArrayList<>() : inputs.getTvInputList();
                List<String[]> entries = new ArrayList<>();
                for (TvInputInfo input : list) {
                    if (!input.isPassthroughInput() || input.isHidden(this)) continue;
                    CharSequence label = input.loadCustomLabel(this);
                    if (label == null || label.length() == 0) label = input.loadLabel(this);
                    int state = inputs.getInputState(input.getId());
                    String connectionState = state == TvInputManager.INPUT_STATE_CONNECTED ? "Connected"
                            : state == TvInputManager.INPUT_STATE_CONNECTED_STANDBY ? "Standby" : "Disconnected";
                    entries.add(new String[]{input.getId(), label + "\n" + connectionState});
                }
                deliver(token, () -> {
                    if (request != inputRequest) return;
                    String focused = focusedTag(inputRow);
                    inputRow.removeAllViews();
                    for (String[] entry : entries) {
                        Button card = button(entry[1], v -> {
                            if (!tryLaunch(new Intent(Intent.ACTION_VIEW,
                                    TvContract.buildChannelUriForPassthroughInput(entry[0])))) {
                                inputFailure("This TV blocked direct input switching.", true);
                            }
                        });
                        card.setTag(entry[0]);
                        inputRow.addView(card, cardParams());
                    }
                    restoreTag(inputRow, focused);
                    inputStatus.setText(entries.isEmpty()
                            ? "No visible HDMI/AV inputs exposed by this TV. Use System input picker or Input settings."
                            : "HDMI / AV · TV-provided labels and connection states · Tuner and hidden inputs: use System input picker");
                });
            } catch (RuntimeException e) {
                deliver(token, () -> {
                    if (request != inputRequest) return;
                    inputStatus.setText("This TV restricts input discovery. Use System input picker or Input settings.");
                });
            }
        });
    }

    private void systemInputs() {
        if (!tryLaunch(new Intent("android.intent.action.TV_INPUT"))) {
            inputFailure("The system input picker is not available to this launcher.", false);
        }
    }

    private void inputSettings() {
        if (!tryLaunch(new Intent("android.settings.TV_INPUT_SETTINGS"))) launchSettings();
    }

    private void launchSettings() {
        launch(new Intent(Settings.ACTION_SETTINGS), "TV settings could not be opened. Use the remote's Settings button or stock home.");
    }

    private void inputFailure(String reason, boolean offerPicker) {
        LinearLayout body = dialogBody();
        body.addView(text(reason + " Use Input settings, TV settings, or the remote's Source/Input button. "
                + "OEM firmware may reserve switching to the stock launcher.", 20, WHITE));
        if (offerPicker) body.addView(button("System input picker", v -> {
            dialog.dismiss();
            systemInputs();
        }));
        body.addView(button("Input settings", v -> { dialog.dismiss(); inputSettings(); }));
        body.addView(button("TV settings", v -> { dialog.dismiss(); launchSettings(); }));
        body.addView(button("Close", v -> dialog.dismiss()));
        showDialog("Input access", body);
    }

    private void homeSetup() {
        LinearLayout body = dialogBody();
        body.addView(text("Try Loom before making it your home. Android's Home app selection is a separate, reversible step. "
                + "If offered, choose Loom TV in system Home settings. To roll back, choose your original launcher there.\n\n"
                + "Some TVs lock the Home button or startup screen to the OEM launcher. Loom does not bypass this, "
                + "disable system packages or run on a boot broadcast. Keep your original home installed.", 20, WHITE));
        body.addView(button("Open Home settings", v -> {
            dialog.dismiss();
            if (!tryLaunch(new Intent(Settings.ACTION_HOME_SETTINGS))) launchSettings();
        }));
        body.addView(button("Close", v -> dialog.dismiss()));
        showDialog("Home setup", body);
    }

    private boolean tryLaunch(Intent intent) {
        try {
            startActivity(intent);
            return true;
        } catch (ActivityNotFoundException | SecurityException e) {
            return false;
        }
    }

    private void launch(Intent intent, String failure) {
        if (tryLaunch(intent)) return;
        LinearLayout body = dialogBody();
        body.addView(text(failure, 20, WHITE));
        body.addView(button("Close", v -> dialog.dismiss()));
        showDialog("Could not open", body);
    }

    private String focusedTag(LinearLayout row) {
        View focused = row.findFocus();
        return focused != null && focused.getTag() instanceof String ? (String) focused.getTag() : null;
    }

    private void restoreTag(LinearLayout row, String tag) {
        if (tag == null) return;
        for (int i = 0; i < row.getChildCount(); i++) {
            if (tag.equals(row.getChildAt(i).getTag())) { row.getChildAt(i).requestFocus(); return; }
        }
        if (row.getChildCount() > 0) row.getChildAt(0).requestFocus();
    }

    private void showDialog(String title, LinearLayout body) {
        stopPairing();
        if (dialog != null) dialog.dismiss();
        ScrollView scroll = new ScrollView(this);
        scroll.addView(body);
        dialog = new AlertDialog.Builder(this).setTitle(title).setView(scroll).create();
        dialog.show();
        dialog.getWindow().setBackgroundDrawable(shape(INK, SURFACE));
        dialog.getWindow().setLayout(getResources().getDisplayMetrics().widthPixels * 4 / 5,
                getResources().getDisplayMetrics().heightPixels * 4 / 5);
    }

    private EditText field(String hint, boolean password) {
        EditText field = new EditText(this);
        field.setHint(hint);
        field.setTextSize(20);
        field.setTextColor(WHITE);
        field.setHintTextColor(MUTED);
        field.setSingleLine(true);
        field.setMinHeight(dp(56));
        field.setPadding(dp(16), dp(8), dp(16), dp(8));
        field.setBackground(focusBackground());
        field.setInputType(InputType.TYPE_CLASS_TEXT | (password
                ? InputType.TYPE_TEXT_VARIATION_PASSWORD : InputType.TYPE_TEXT_VARIATION_URI));
        field.setImportantForAutofill(View.IMPORTANT_FOR_AUTOFILL_NO_EXCLUDE_DESCENDANTS);
        field.setSaveEnabled(false);
        field.setContentDescription(hint);
        return field;
    }

    private LinearLayout dialogBody() {
        LinearLayout body = column();
        body.setPadding(dp(24), dp(16), dp(24), dp(24));
        body.setBackgroundColor(INK);
        return body;
    }

    private LinearLayout column() {
        LinearLayout layout = new LinearLayout(this);
        layout.setOrientation(LinearLayout.VERTICAL);
        return layout;
    }

    private LinearLayout row() {
        LinearLayout layout = new LinearLayout(this);
        layout.setOrientation(LinearLayout.HORIZONTAL);
        layout.setGravity(Gravity.CENTER_VERTICAL);
        return layout;
    }

    private LinearLayout shelfRow(LinearLayout root) {
        HorizontalScrollView scroller = new HorizontalScrollView(this);
        scroller.setClipToPadding(false);
        scroller.setPadding(0, dp(8), 0, dp(8));
        scroller.setHorizontalScrollBarEnabled(false);
        LinearLayout row = row();
        scroller.addView(row);
        root.addView(scroller);
        return row;
    }

    private void heading(LinearLayout root, String label) {
        TextView title = text(label, 26, WHITE);
        title.setTypeface(Typeface.create("sans-serif-medium", Typeface.NORMAL));
        title.setPadding(0, dp(24), 0, dp(8));
        if (android.os.Build.VERSION.SDK_INT >= 28) title.setAccessibilityHeading(true);
        root.addView(title);
    }

    private TextView text(String value, int size, int color) {
        TextView text = new TextView(this);
        text.setText(value);
        text.setTextColor(color);
        text.setTextSize(size);
        text.setPadding(0, dp(4), 0, dp(4));
        text.setLineSpacing(dp(3), 1);
        return text;
    }

    private Button button(String title, View.OnClickListener click) {
        Button button = new Button(this);
        button.setText(title);
        button.setAllCaps(false);
        button.setTextSize(20);
        button.setTextColor(new ColorStateList(
                new int[][]{new int[]{-android.R.attr.state_enabled}, new int[]{}},
                new int[]{0xFF8D9CB0, WHITE}));
        button.setMinHeight(dp(56));
        button.setPadding(dp(16), dp(12), dp(16), dp(12));
        button.setBackground(focusBackground());
        button.setOnClickListener(click);
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(-2, -2);
        params.setMargins(0, dp(4), dp(12), dp(4));
        button.setLayoutParams(params);
        return button;
    }

    private LinearLayout.LayoutParams cardParams() {
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(dp(240), dp(128));
        params.setMargins(0, dp(4), dp(12), dp(4));
        return params;
    }

    private StateListDrawable focusBackground() {
        StateListDrawable states = new StateListDrawable();
        states.addState(new int[]{-android.R.attr.state_enabled}, shape(INK, 0xFF657387));
        states.addState(new int[]{android.R.attr.state_pressed}, shape(FOCUS_SURFACE, BLUE));
        states.addState(new int[]{android.R.attr.state_focused}, shape(FOCUS_SURFACE, BLUE));
        states.addState(new int[]{}, shape(SURFACE, SURFACE));
        return states;
    }

    private GradientDrawable shape(int fill, int stroke) {
        GradientDrawable shape = new GradientDrawable();
        shape.setColor(fill);
        shape.setCornerRadius(dp(8));
        shape.setStroke(dp(3), stroke);
        return shape;
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }
}
