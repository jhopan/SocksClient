package com.jhopanstore.socksclient;

import android.Manifest;
import android.app.ActivityManager;
import android.app.Activity;
import android.app.AlertDialog;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.PackageManager;
import android.graphics.Color;
import android.graphics.Typeface;
import android.net.VpnService;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.text.InputType;
// import android.util.Log; // all logs commented out
import android.view.Gravity;
import android.view.View;
import android.widget.Button;
import android.widget.RadioButton;
import android.widget.RadioGroup;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

public class MainActivity extends Activity {
    private static final String TAG = "SocksClientMain";
    private static final int BG = Color.rgb(10, 10, 12);
    private static final int CARD = Color.rgb(24, 24, 30);
    private static final int TEXT_PRIMARY = Color.rgb(245, 245, 247);
    private static final int TEXT_SECONDARY = Color.rgb(136, 136, 136);
    private static final int GREEN = Color.rgb(28, 184, 98);
    private static final int RED = Color.rgb(220, 60, 60);
    private static final int GRAY = Color.rgb(96, 102, 114);
    private static final int REQ_VPN = 300;
    private static final String STATUS_PREFS = "socks_client_status";
    private static final String KEY_CONNECTED = "connected";
    private static final String KEY_STATUS = "status";

    private final Handler handler = new Handler(Looper.getMainLooper());
    private SharedPreferences prefs;

    private EditText hostInput;
    private EditText portInput;
    private EditText userInput;
    private EditText passInput;
    private TextView statusText;
    private TextView pingText;
    private RadioButton pingOnRadio, pingOffRadio;
    private boolean syncingPingUi;
    private Button connectButton;
    // private Button disconnectButton; // merged into connectButton

    // ── Traffic counter UI ──

    private final Runnable ticker = new Runnable() {
        @Override
        public void run() {
            refreshUi();
            handler.postDelayed(this, 1000);
        }
    };

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        prefs = getSharedPreferences("socks_client", MODE_PRIVATE);
        requestNotificationPermission();
        setContentView(buildContent());
        refreshUi();
    }

    @Override
    protected void onStart() {
        super.onStart();
        handler.post(ticker);
    }

    @Override
    protected void onStop() {
        handler.removeCallbacks(ticker);
        super.onStop();
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode == REQ_VPN) {
            if (resultCode == RESULT_OK) {
                startVpn();
            } else {
                connectButton.setEnabled(true);
                statusText.setText("Status: VPN permission ditolak/dibatalkan");
                // Log.w(TAG, "VPN permission denied/cancelled, resultCode=" + resultCode);
            }
        }
    }

    private View buildContent() {
        ScrollView scroll = new ScrollView(this);
        scroll.setFillViewport(true);
        scroll.setBackgroundColor(BG);

        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setPadding(dp(20), dp(20), dp(20), dp(24));
        scroll.addView(root, new ScrollView.LayoutParams(-1, -2));

        TextView title = text("Socks Client", 28, true, TEXT_PRIMARY);
        title.setGravity(Gravity.CENTER);
        root.addView(title, matchWrap());

        hostInput = textInput("SOCKS Host/IP", prefs.getString("host", ""));
        portInput = numberInput("SOCKS Port", prefs.getInt("port", 1080));
        userInput = textInput("Username (opsional)", prefs.getString("user", ""));
        // S3: password disimpan terenkripsi (Android Keystore). Kunci lama "pass"
        // masih dibaca sekali untuk migrasi, lalu ikut ditulis ulang terenkripsi.
        String passPlain = SecurePrefs.decrypt(this, prefs.getString("pass_enc", null));
        passInput = textInput("Password (opsional)",
                passPlain != null ? passPlain : prefs.getString("pass", ""));

        root.addView(fieldBox("Host / IP", hostInput), marginTop(matchWrap(), 18));
        root.addView(fieldBox("Port", portInput), marginTop(matchWrap(), 10));
        root.addView(fieldBox("Username", userInput), marginTop(matchWrap(), 10));
        root.addView(fieldBox("Password", passInput), marginTop(matchWrap(), 10));

        connectButton = button("Connect Socks VPN");
        connectButton.setBackgroundColor(GREEN);
        connectButton.setOnClickListener(v -> onToggleConnection());
        root.addView(connectButton, marginTop(matchWrap(), 16));

        Button guide = button("Cara Pakai Socks Client");
        guide.setBackgroundColor(Color.rgb(86, 96, 111));
        guide.setOnClickListener(v -> showGuide());
        root.addView(guide, marginTop(matchWrap(), 8));

        Button infoDev = button("Info Developer");
        infoDev.setBackgroundColor(Color.rgb(70, 130, 180));
        infoDev.setOnClickListener(v -> showDeveloperInfo());
        root.addView(infoDev, marginTop(matchWrap(), 8));

        // HTTP ping: On/Off. Hasilnya ditulis service ke prefs dan dibaca di sini.
        LinearLayout pingRow = new LinearLayout(this);
        pingRow.setOrientation(LinearLayout.HORIZONTAL);
        pingRow.setGravity(Gravity.CENTER_VERTICAL);
        pingRow.addView(text("HTTP ping:", 14, true, TEXT_PRIMARY), matchWrap());
        RadioGroup pingGroup = new RadioGroup(this);
        pingGroup.setOrientation(RadioGroup.HORIZONTAL);
        pingOnRadio = new RadioButton(this);
        pingOnRadio.setText("On");
        pingOnRadio.setTextColor(TEXT_PRIMARY);
        pingOffRadio = new RadioButton(this);
        pingOffRadio.setText("Off");
        pingOffRadio.setTextColor(TEXT_PRIMARY);
        pingGroup.addView(pingOnRadio);
        pingGroup.addView(pingOffRadio);
        pingGroup.setOnCheckedChangeListener((group, checkedId) -> {
            if (syncingPingUi) return;
            boolean enabled = checkedId == pingOnRadio.getId();
            getSharedPreferences(STATUS_PREFS, MODE_PRIVATE).edit()
                    .putBoolean("ping_enabled", enabled)
                    .apply();
        });
        LinearLayout.LayoutParams pingGroupParams = matchWrap();
        pingGroupParams.leftMargin = dp(4);
        pingRow.addView(pingGroup, pingGroupParams);
        root.addView(pingRow, marginTop(matchWrap(), 10));

        pingText = text("Ping: -", 13, false, TEXT_SECONDARY);
        root.addView(pingText, marginTop(matchWrap(), 4));
        statusText = text("", 15, true, TEXT_PRIMARY);
        root.addView(statusText, marginTop(matchWrap(), 18));

        return scroll;
    }


    private void onToggleConnection() {
        SharedPreferences statusPrefs = getSharedPreferences(STATUS_PREFS, MODE_PRIVATE);
        boolean connected = statusPrefs.getBoolean(KEY_CONNECTED, false);
        boolean connecting = statusText.getText().toString().toLowerCase().contains("connecting");
        if (connected || connecting || isVpnServiceAlive()) {
            stopVpn();
        } else {
            prepareAndConnect();
        }
    }

    private void prepareAndConnect() {
        String host = hostInput.getText().toString().trim();
        int port = parsePort(portInput, 1080);
        if (host.isEmpty()) {
            statusText.setText("Status: Host/IP wajib diisi");
            // Log.w(TAG, "connect blocked: empty host");
            return;
        }

        connectButton.setEnabled(false);
        statusText.setText("Status: Preparing VPN...");
        // Log.i(TAG, "prepareAndConnect host=" + host + " port=" + port);

        prefs.edit()
                .putString("host", host)
                .putInt("port", port)
                .putString("user", userInput.getText().toString().trim())
                .putString("pass_enc", SecurePrefs.encrypt(MainActivity.this, passInput.getText().toString()))
                .remove("pass") // S3: jangan tinggalkan bekas plaintext
                .apply();

        // P8: hanya IP literal. Hostname memaksa sing-box melakukan bootstrap DNS
        // di dalam proses app - dan proses app dikecualikan dari VPN, jadi query
        // itu keluar lewat jaringan bawah (bocor) atau gagal total.
        if (!isIpLiteral(hostInput.getText().toString().trim())) {
            statusText.setText("Status: Host harus IP (contoh 10.12.132.225)");
            connectButton.setEnabled(true);
            return;
        }

        new Thread(() -> {
            try {
                Intent vpnIntent = VpnService.prepare(MainActivity.this);
                handler.post(() -> {
                    if (vpnIntent != null) {
                        try {
                            startActivityForResult(vpnIntent, REQ_VPN);
                            // Log.i(TAG, "VpnService.prepare requires user consent");
                        } catch (Exception e) {
                            statusText.setText("Status: VPN permission error - " + safeMessage(e));
                            connectButton.setEnabled(true);
                            // Log.e(TAG, "vpn permission error", e);
                        }
                    } else {
                        // Log.i(TAG, "VpnService.prepare already granted");
                        startVpn();
                    }
                });
            } catch (Exception e) {
                handler.post(() -> {
                    statusText.setText("Status: Gagal prepare VPN - " + safeMessage(e));
                    connectButton.setEnabled(true);
                    // Log.e(TAG, "prepare failed", e);
                });
            }
        }).start();
    }

    /** Bootstrap dan rule anti-loop aman hanya kalau host berupa IP literal. */
    private static boolean isIpLiteral(String host) {
        if (host == null) return false;
        String h = host.trim();
        if (h.matches("^\\d{1,3}(\\.\\d{1,3}){3}$")) return true;
        return h.contains(":") && h.matches("^[0-9a-fA-F:]+$");
    }

    private void startVpn() {
        try {
            Intent intent = new Intent(this, SocksVpnService.class)
                    .setAction(SocksVpnService.ACTION_CONNECT)
                    .putExtra(SocksVpnService.EXTRA_HOST, hostInput.getText().toString().trim())
                    .putExtra(SocksVpnService.EXTRA_PORT, parsePort(portInput, 1080))
                    .putExtra(SocksVpnService.EXTRA_USER, userInput.getText().toString().trim())
                    .putExtra(SocksVpnService.EXTRA_PASS, passInput.getText().toString());
            if (Build.VERSION.SDK_INT >= 26) startForegroundService(intent);
            else startService(intent);
            // Log.i(TAG, "startVpn service started");
        } catch (Throwable e) {
            saveStatus(false, "Gagal start VPN: " + safeMessage(e));
            // Log.e(TAG, "startVpn failed", e);
        }
        refreshUi();
    }

    private void stopVpn() {
        try {
            connectButton.setEnabled(false);
            statusText.setText("Status: Stopping VPN...");
            // Log.i(TAG, "stopVpn clicked");

            Intent stop = new Intent(this, SocksVpnService.class).setAction(SocksVpnService.ACTION_DISCONNECT);
            startService(stop);

            new Thread(() -> {
                try {
                    Thread.sleep(500);
                } catch (InterruptedException ignored) {}
                handler.post(this::refreshUi);
            }).start();
        } catch (Throwable e) {
            saveStatus(false, "Stop error: " + safeMessage(e));
            // Log.e(TAG, "stopVpn failed", e);
            refreshUi();
        }
    }

    private void refreshUi() {
        SharedPreferences statusPrefs = getSharedPreferences(STATUS_PREFS, MODE_PRIVATE);
        boolean connected = statusPrefs.getBoolean(KEY_CONNECTED, false);

        if (connected && !isVpnServiceAlive()) {
            connected = false;
            saveStatus(false, "Disconnected");
        }

        statusText.setText("Status: " + (connected ? "Connected" : "Disconnected"));

        // HTTP ping: samakan radio dengan pref (tanpa memicu penulisan balik),
        // lalu tampilkan hasil terakhir yang ditulis service.
        boolean pingEnabled = statusPrefs.getBoolean("ping_enabled", false);
        syncingPingUi = true;
        if (pingEnabled && !pingOnRadio.isChecked()) {
            pingOnRadio.setChecked(true);
        } else if (!pingEnabled && !pingOffRadio.isChecked()) {
            pingOffRadio.setChecked(true);
        }
        syncingPingUi = false;
        if (pingText != null) {
            String pingResult = statusPrefs.getString("ping_result", null);
            if (pingResult != null) {
                pingText.setText("Ping: " + pingResult);
            } else {
                pingText.setText(pingEnabled
                        ? (connected ? "Ping: menunggu hasil..." : "Ping: aktif saat Connected")
                        : "Ping: off (tidak ada trafik tambahan)");
            }
        }

        boolean connecting = false;
        String status = statusPrefs.getString(KEY_STATUS, "");
        if (status != null && status.toLowerCase().contains("connecting")) {
            connecting = true;
        }
        
        boolean allowDisconnect = connected || connecting || isVpnServiceAlive();

        // Merged toggle button: Connect ↔ Disconnect
        if (allowDisconnect) {
            connectButton.setText("Disconnect");
            connectButton.setBackgroundColor(RED);
            connectButton.setEnabled(true);
        } else {
            connectButton.setText("Connect Socks VPN");
            connectButton.setBackgroundColor(GREEN);
            connectButton.setEnabled(!connecting);
        }

        boolean lockInputs = connected || connecting;
        hostInput.setEnabled(!lockInputs);
        portInput.setEnabled(!lockInputs);
        userInput.setEnabled(!lockInputs);
        passInput.setEnabled(!lockInputs);
    }

    private boolean isVpnServiceAlive() {
        SharedPreferences sp = getSharedPreferences(STATUS_PREFS, MODE_PRIVATE);
        long lastSeen = sp.getLong("last_seen", 0);
        // Service menulis heartbeat tiap 10 detik (SocksVpnService.HEARTBEAT_MS), jadi jendela
        // ini harus >= 3x interval supaya Doze/layar mati sesaat tidak dibaca sebagai service mati.
        if (lastSeen > 0 && (System.currentTimeMillis() - lastSeen) < 45000) {
            return true;
        }
        ActivityManager am = (ActivityManager) getSystemService(ACTIVITY_SERVICE);
        if (am == null) return false;
        for (ActivityManager.RunningServiceInfo info : am.getRunningServices(Integer.MAX_VALUE)) {
            if (info.service != null
                    && SocksVpnService.class.getName().equals(info.service.getClassName())) {
                return true;
            }
        }
        return false;
    }

    private void saveStatus(boolean connected, String status) {
        getSharedPreferences(STATUS_PREFS, MODE_PRIVATE)
                .edit()
                .putBoolean(KEY_CONNECTED, connected)
                .putString(KEY_STATUS, status)
                .apply();
    }

    private String safeMessage(Throwable e) {
        if (e == null) return "unknown";
        String msg = e.getMessage();
        return msg == null || msg.trim().isEmpty() ? e.getClass().getSimpleName() : msg;
    }

    private String formatBytes(long bytes) {
        if (bytes < 1024) return bytes + " B";
        if (bytes < 1024 * 1024) return String.format(java.util.Locale.US, "%.1f KB", bytes / 1024.0);
        if (bytes < 1024L * 1024 * 1024) return String.format(java.util.Locale.US, "%.1f MB", bytes / (1024.0 * 1024));
        return String.format(java.util.Locale.US, "%.2f GB", bytes / (1024.0 * 1024 * 1024));
    }

    private void showGuide() {
        String guide = "1) Hubungkan HP client ke hotspot/USB tether dari HP server.\n"
                + "2) Isi Host/IP dengan alamat SOCKS5 server (contoh 192.168.1.10).\n"
                + "3) Isi Port (default 1080).\n"
                + "4) Jika server pakai auth, isi Username dan Password.\n"
                + "5) Tekan Connect Socks VPN lalu izinkan VPN Android.\n"
                + "6) Saat status Connected, SEMUA trafik (TCP + UDP) dari aplikasi client "
                + "akan di-tunnel ke SOCKS5 server via VPN.\n\n"
                + "Fitur:\n"
                + "• TCP + UDP via SOCKS5 (UDP Associate)\n"
                + "• DNS remote via tunnel (anti DNS leak)\n"
                + "• Anti routing loop (bind_interface + bypass rule)\n"
                + "• Protocol sniffing (HTTP/TLS/QUIC)\n"
                + "• IPv4 + IPv6 support\n"
                + "• Traffic Counter (upload/download stats)\n\n"
                + "Tips: Pastikan SOCKS5 server support UDP Associate agar UDP lancar.";
        new AlertDialog.Builder(this)
                .setTitle("Panduan Socks Client")
                .setMessage(guide)
                .setPositiveButton("Tutup", null)
                .show();
    }

    private void showDeveloperInfo() {
        String versionName = "1.2.0";
        try {
            versionName = getPackageManager().getPackageInfo(getPackageName(), 0).versionName;
        } catch (Exception ignored) {}

        String info = "Socks Client v" + versionName + "\n\n"
                + "Developer: JhopanStore\n"
                + "Platform: Android (SOCKS5 VPN Client)\n\n"
                + "Hubungi developer atau dukung pengembangan aplikasi:";

        AlertDialog dialog = new AlertDialog.Builder(this)
                .setTitle("Info Developer")
                .setMessage(info)
                .setPositiveButton("Telegram", (d, w) -> openUrl("https://t.me/jhopan_05"))
                .setNeutralButton("Website", (d, w) -> openUrl("https://jhopanstore.my.id"))
                .setNegativeButton("Trakteer", (d, w) -> openUrl("https://trakteer.id/jhopan"))
                .create();

        dialog.setOnShowListener(d -> {
            AlertDialog ad = (AlertDialog) d;
            ad.getButton(AlertDialog.BUTTON_POSITIVE).setTextColor(Color.rgb(0, 136, 204));
            ad.getButton(AlertDialog.BUTTON_NEUTRAL).setTextColor(Color.rgb(76, 175, 80));
            ad.getButton(AlertDialog.BUTTON_NEGATIVE).setTextColor(Color.rgb(244, 67, 54));
        });

        dialog.show();
    }

    private void openUrl(String url) {
        try {
            startActivity(new Intent(Intent.ACTION_VIEW, android.net.Uri.parse(url)));
        } catch (Exception ignored) {}
    }

    private void requestNotificationPermission() {
        if (Build.VERSION.SDK_INT >= 33 &&
                checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS}, 40);
        }
    }

    private int parsePort(EditText input, int fallback) {
        try {
            int port = Integer.parseInt(input.getText().toString().trim());
            return port > 0 && port <= 65535 ? port : fallback;
        } catch (Exception e) {
            return fallback;
        }
    }

    private TextView text(String value, int sp, boolean bold, int color) {
        TextView view = new TextView(this);
        view.setText(value);
        view.setTextSize(sp);
        view.setTextColor(color);
        if (bold) view.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        return view;
    }

    private EditText textInput(String hint, String value) {
        EditText input = new EditText(this);
        input.setText(value);
        input.setHint(hint);
        input.setTextSize(17);
        input.setSingleLine(true);
        input.setTextColor(TEXT_PRIMARY);
        input.setHintTextColor(TEXT_SECONDARY);
        input.setBackgroundColor(CARD);
        return input;
    }

    private EditText numberInput(String hint, int value) {
        EditText input = textInput(hint, String.valueOf(value));
        input.setInputType(InputType.TYPE_CLASS_NUMBER);
        if (Build.VERSION.SDK_INT >= 21) input.setFontFeatureSettings("tnum");
        return input;
    }

    private LinearLayout fieldBox(String label, EditText input) {
        LinearLayout box = new LinearLayout(this);
        box.setOrientation(LinearLayout.VERTICAL);
        box.setPadding(dp(12), dp(10), dp(12), dp(8));
        box.setBackgroundColor(CARD);
        TextView tv = text(label, 13, true, TEXT_SECONDARY);
        box.addView(tv, matchWrap());
        box.addView(input, marginTop(matchWrap(), 6));
        return box;
    }

    private Button button(String label) {
        Button button = new Button(this);
        button.setText(label);
        button.setTextColor(Color.WHITE);
        button.setAllCaps(false);
        button.setTextSize(15);
        button.setBackgroundColor(GRAY);
        return button;
    }

    private LinearLayout.LayoutParams matchWrap() {
        return new LinearLayout.LayoutParams(-1, -2);
    }

    private LinearLayout.LayoutParams marginTop(LinearLayout.LayoutParams params, int topDp) {
        params.topMargin = dp(topDp);
        return params;
    }

    private int dp(int value) {
        return (int) (value * getResources().getDisplayMetrics().density + 0.5f);
    }
}
