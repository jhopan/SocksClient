package com.jhopanstore.socksclient;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Intent;
import android.content.SharedPreferences;
import android.net.VpnService;
import android.os.Build;
import android.os.ParcelFileDescriptor;
// import android.util.Log; // all logs commented out

import java.util.Locale;
import java.util.concurrent.atomic.AtomicLong;

import io.nekohasekai.libbox.CommandServer;
import io.nekohasekai.libbox.CommandServerHandler;
import io.nekohasekai.libbox.ConnectionOwner;
import io.nekohasekai.libbox.InterfaceUpdateListener;
import io.nekohasekai.libbox.Libbox;
import io.nekohasekai.libbox.LocalDNSTransport;
import io.nekohasekai.libbox.NetworkInterface;
import io.nekohasekai.libbox.NetworkInterfaceIterator;
import io.nekohasekai.libbox.OverrideOptions;
import io.nekohasekai.libbox.PlatformInterface;
import io.nekohasekai.libbox.RoutePrefix;
import io.nekohasekai.libbox.RoutePrefixIterator;
import io.nekohasekai.libbox.SetupOptions;
import io.nekohasekai.libbox.StringBox;
import io.nekohasekai.libbox.StringIterator;
import io.nekohasekai.libbox.SystemProxyStatus;
import io.nekohasekai.libbox.TunOptions;
import io.nekohasekai.libbox.WIFIState;

import java.net.Inet4Address;
import java.net.InetAddress;
import java.util.ArrayList;
import java.util.Enumeration;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

public class SocksVpnService extends VpnService implements PlatformInterface, CommandServerHandler {
    static final String ACTION_CONNECT = "com.jhopanstore.socksclient.CONNECT";
    static final String ACTION_DISCONNECT = "com.jhopanstore.socksclient.DISCONNECT";
    static final String EXTRA_HOST = "host";
    static final String EXTRA_PORT = "port";
    static final String EXTRA_USER = "user";
    static final String EXTRA_PASS = "pass";

    private static final String TAG = "SocksVpnService";
    private static final String CHANNEL_ID = "socks_client";
    private static final int NOTIF_ID = 11;
    private static final String PREFS = "socks_client_status";
    private static final String KEY_CONNECTED = "connected";
    private static final String KEY_STATUS = "status";
    private static final String KEY_LAST_SEEN = "last_seen";
    private static final String KEY_TRAFFIC_ENABLED = "traffic_counter_enabled";
    private static final String KEY_UPLOAD_BYTES = "upload_bytes";
    private static final String KEY_DOWNLOAD_BYTES = "download_bytes";
    private static final String TUN_IFACE = "sb-tun";
    private static final long TRAFFIC_POLL_MS = 2000;

    private final ExecutorService worker = Executors.newSingleThreadExecutor();
    private final Object lock = new Object();
    private volatile boolean running;
    private volatile boolean connecting;
    private volatile boolean stopping;
    private CommandServer commandServer;
    private ParcelFileDescriptor vpnFd;
    private Thread heartbeatThread;

    // ── Traffic counter ──
    private final AtomicLong uploadBytes = new AtomicLong(0);
    private final AtomicLong downloadBytes = new AtomicLong(0);
    private volatile boolean trafficCounterEnabled;
    private Thread trafficThread;

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        String action = intent == null ? null : intent.getAction();
        logI("onStartCommand action=" + action);

        if (ACTION_DISCONNECT.equals(action)) {
            new Thread(this::disconnectInternal, "sb-disconnect").start();
            return START_NOT_STICKY;
        }

        if (ACTION_CONNECT.equals(action)) {
            if (running || connecting) {
                logI("connect ignored: running=" + running + " connecting=" + connecting);
                return START_STICKY;
            }
            final String host = intent.getStringExtra(EXTRA_HOST);
            final int port = intent.getIntExtra(EXTRA_PORT, 1080);
            final String user = intent.getStringExtra(EXTRA_USER);
            final String pass = intent.getStringExtra(EXTRA_PASS);
            setStatus(false, "Connecting...");
            startForeground(NOTIF_ID, buildNotification("Connecting..."));
            worker.execute(() -> connectInternal(host, port, user, pass));
        }
        return START_STICKY;
    }

    @Override
    public void onDestroy() {
        logI("onDestroy");
        disconnectInternal();
        worker.shutdownNow();
        super.onDestroy();
    }

    @Override
    public void onRevoke() {
        logI("onRevoke");
        disconnectInternal();
        super.onRevoke();
    }

    // ──────────────────────────────────────────────
    // Connect
    // ──────────────────────────────────────────────

    private void connectInternal(String host, int port, String user, String pass) {
        synchronized (lock) {
            if (running) {
                logI("connectInternal: already running, skip");
                return;
            }
            connecting = true;
            stopping = false;
        }

        try {
            if (host == null || host.trim().isEmpty()) throw new IllegalArgumentException("Host kosong");
            if (port <= 0 || port > 65535) throw new IllegalArgumentException("Port tidak valid: " + port);

            // Setup sing-box
            SetupOptions setupOptions = new SetupOptions();
            setupOptions.setBasePath(getFilesDir().getAbsolutePath());
            setupOptions.setWorkingPath(getNoBackupFilesDir().getAbsolutePath());
            setupOptions.setTempPath(getCacheDir().getAbsolutePath());
            setupOptions.setDebug(BuildConfig.DEBUG);
            setupOptions.setLogMaxLines(1000);
            Libbox.setup(setupOptions);
            Libbox.redirectStderr(getFileStreamPath("singbox-stderr.log").getAbsolutePath());

            // Start command server
            commandServer = Libbox.newCommandServer(this, this);
            commandServer.start();

            // Build config dan start service (bind_interface = null, pakai auto_detect_interface)
            String config = buildSingBoxConfig(host.trim(), port, user, pass, null);
            logI("starting sing-box, config-length=" + config.length() + ", auto_detect_interface");
            if (BuildConfig.DEBUG) {
                logI("config: " + config);
            }
            commandServer.startOrReloadService(config, new OverrideOptions());

            synchronized (lock) {
                running = true;
                connecting = false;
            }

            setStatus(true, "Connected ke SOCKS " + host + ":" + port);
            notifyStatus("Connected ✓ " + host + ":" + port);
            startHeartbeat();
            resetTrafficCounters();
            loadTrafficToggle();
            startTrafficMonitor();
            logI("VPN connected successfully");

        } catch (Throwable t) {
            logE("connectInternal failed", t);
            setStatus(false, "Gagal connect: " + safeMessage(t));
            notifyStatus("Gagal connect");
            synchronized (lock) {
                running = false;
                connecting = false;
            }
            disconnectCoreOnly();
        }
    }

    // ──────────────────────────────────────────────
    // Disconnect
    // ──────────────────────────────────────────────

    private void disconnectInternal() {
        synchronized (lock) {
            if (stopping) return;
            stopping = true;
        }

        logI("disconnectInternal");
        stopHeartbeat();
        stopTrafficMonitor();
        disconnectCoreOnly();
        setStatus(false, "Disconnected");
        // Clear traffic stats on disconnect
        SharedPreferences sp = getSharedPreferences(PREFS, MODE_PRIVATE);
        sp.edit()
                .putLong(KEY_UPLOAD_BYTES, 0)
                .putLong(KEY_DOWNLOAD_BYTES, 0)
                .apply();
        notifyStatus("Disconnected");
        stopForeground(true);
        stopSelf();
    }

    private void disconnectCoreOnly() {
        synchronized (lock) {
            running = false;
            connecting = false;
        }

        try {
            if (commandServer != null) {
                commandServer.closeService();
                commandServer.close();
            }
        } catch (Throwable t) {
            logE("disconnectCoreOnly close commandServer", t);
        } finally {
            commandServer = null;
        }

        if (vpnFd != null) {
            try {
                vpnFd.close();
            } catch (Exception ignored) {
            }
            vpnFd = null;
        }
    }

    // ──────────────────────────────────────────────
    // Heartbeat — agar UI tahu service masih hidup
    // ──────────────────────────────────────────────

    private void startHeartbeat() {
        stopHeartbeat();
        heartbeatThread = new Thread(() -> {
            while (running && !stopping) {
                try {
                    SharedPreferences sp = getSharedPreferences(PREFS, MODE_PRIVATE);
                    sp.edit().putLong(KEY_LAST_SEEN, System.currentTimeMillis()).apply();
                    Thread.sleep(3000);
                } catch (InterruptedException e) {
                    break;
                } catch (Exception e) {
                    // Log.w(TAG, "heartbeat error", e);
                }
            }
        }, "sb-heartbeat");
        heartbeatThread.setDaemon(true);
        heartbeatThread.start();
    }

    private void stopHeartbeat() {
        if (heartbeatThread != null) {
            heartbeatThread.interrupt();
            heartbeatThread = null;
        }
    }

    // ──────────────────────────────────────────────
    // Traffic Counter — monitor TUN interface usage
    // ──────────────────────────────────────────────

    private void loadTrafficToggle() {
        SharedPreferences sp = getSharedPreferences(PREFS, MODE_PRIVATE);
        trafficCounterEnabled = sp.getBoolean(KEY_TRAFFIC_ENABLED, true);
    }

    /** Toggle the traffic counter on/off at runtime. Persists across reconnects. */
    public void setTrafficCounterEnabled(boolean enabled) {
        trafficCounterEnabled = enabled;
        SharedPreferences sp = getSharedPreferences(PREFS, MODE_PRIVATE);
        sp.edit().putBoolean(KEY_TRAFFIC_ENABLED, enabled).apply();
        if (enabled && running) {
            startTrafficMonitor();
        } else {
            stopTrafficMonitor();
        }
        // Refresh notification immediately
        if (running) {
            notifyStatus(enabled
                    ? "Connected ✓ (traffic on)"
                    : "Connected ✓");
        }
    }

    public boolean isTrafficCounterEnabled() {
        return trafficCounterEnabled;
    }

    public long getUploadBytes() {
        return uploadBytes.get();
    }

    public long getDownloadBytes() {
        return downloadBytes.get();
    }

    private void resetTrafficCounters() {
        uploadBytes.set(0);
        downloadBytes.set(0);
        SharedPreferences sp = getSharedPreferences(PREFS, MODE_PRIVATE);
        sp.edit()
                .putLong(KEY_UPLOAD_BYTES, 0)
                .putLong(KEY_DOWNLOAD_BYTES, 0)
                .apply();
    }

    private void startTrafficMonitor() {
        stopTrafficMonitor();
        if (!trafficCounterEnabled) return;

        trafficThread = new Thread(() -> {
            long prevTx = -1, prevRx = -1;
            while (running && !stopping && trafficCounterEnabled) {
                try {
                    long[] stats = readTunTraffic();
                    long curTx = stats[0];
                    long curRx = stats[1];

                    if (prevTx >= 0 && curTx >= prevTx) {
                        uploadBytes.addAndGet(curTx - prevTx);
                    }
                    if (prevRx >= 0 && curRx >= prevRx) {
                        downloadBytes.addAndGet(curRx - prevRx);
                    }
                    prevTx = curTx;
                    prevRx = curRx;

                    // Persist traffic stats to SharedPreferences for Activity to read
                    SharedPreferences sp = getSharedPreferences(PREFS, MODE_PRIVATE);
                    sp.edit()
                            .putLong(KEY_UPLOAD_BYTES, uploadBytes.get())
                            .putLong(KEY_DOWNLOAD_BYTES, downloadBytes.get())
                            .apply();

                    // Update notification with latest traffic
                    if (running) {
                        NotificationManager mgr = (NotificationManager) getSystemService(NOTIFICATION_SERVICE);
                        if (mgr != null) {
                            mgr.notify(NOTIF_ID, buildNotification(
                                    "Connected ✓"));
                        }
                    }

                    Thread.sleep(TRAFFIC_POLL_MS);
                } catch (InterruptedException e) {
                    break;
                } catch (Exception e) {
                    // Log.w(TAG, "traffic monitor error", e);
                    try { Thread.sleep(TRAFFIC_POLL_MS); } catch (InterruptedException ie) { break; }
                }
            }
        }, "sb-traffic");
        trafficThread.setDaemon(true);
        trafficThread.start();
    }

    private void stopTrafficMonitor() {
        if (trafficThread != null) {
            trafficThread.interrupt();
            trafficThread = null;
        }
    }

    /**
     * Read TX/RX bytes from the TUN interface via sysfs.
     * Returns [txBytes, rxBytes]. Falls back to [-1, -1] on error.
     */
    private long[] readTunTraffic() {
        long tx = -1, rx = -1;
        try {
            // Try sysfs first (most reliable)
            tx = readLongFromFile("/sys/class/net/" + TUN_IFACE + "/statistics/tx_bytes");
            rx = readLongFromFile("/sys/class/net/" + TUN_IFACE + "/statistics/rx_bytes");
        } catch (Exception ignored) {
        }

        // Fallback: parse /proc/net/dev
        if (tx < 0 || rx < 0) {
            try {
                java.io.BufferedReader br = new java.io.BufferedReader(
                        new java.io.FileReader("/proc/net/dev"));
                String line;
                while ((line = br.readLine()) != null) {
                    line = line.trim();
                    if (line.startsWith(TUN_IFACE + ":")) {
                        String[] parts = line.split("\\s+");
                        // Format: iface: rxBytes rxPackets ... txBytes txPackets ...
                        // Index:  0      1        2           9        10
                        if (parts.length >= 11) {
                            String rxStr = parts[1];
                            // Handle "iface:rxBytes" format (no space after colon)
                            if (rxStr.contains(":")) {
                                rxStr = rxStr.substring(rxStr.indexOf(':') + 1);
                            }
                            rx = Long.parseLong(rxStr);
                            tx = Long.parseLong(parts[9]);
                        }
                        break;
                    }
                }
                br.close();
            } catch (Exception ignored) {
            }
        }

        return new long[]{tx, rx};
    }

    private long readLongFromFile(String path) throws Exception {
        java.io.BufferedReader br = new java.io.BufferedReader(new java.io.FileReader(path));
        String val = br.readLine();
        br.close();
        return Long.parseLong(val.trim());
    }

    private String formatBytes(long bytes) {
        if (bytes < 1024) return bytes + " B";
        if (bytes < 1024 * 1024) return String.format(Locale.US, "%.1f KB", bytes / 1024.0);
        if (bytes < 1024L * 1024 * 1024) return String.format(Locale.US, "%.1f MB", bytes / (1024.0 * 1024));
        return String.format(Locale.US, "%.2f GB", bytes / (1024.0 * 1024 * 1024));
    }

    // ──────────────────────────────────────────────
    // Network Interface Detection
    // ──────────────────────────────────────────────

    private String detectActiveInterface() {
        try {
            Enumeration<java.net.NetworkInterface> nifs = java.net.NetworkInterface.getNetworkInterfaces();
            List<String> candidates = new ArrayList<>();

            while (nifs != null && nifs.hasMoreElements()) {
                java.net.NetworkInterface nif = nifs.nextElement();
                try {
                    if (!nif.isUp() || nif.isLoopback() || nif.isVirtual() || nif.isPointToPoint()) continue;

                    // Cek apakah punya IPv4 address (artinya benar-benar connected)
                    boolean hasIPv4 = false;
                    for (java.net.InterfaceAddress addr : nif.getInterfaceAddresses()) {
                        if (addr.getAddress() instanceof Inet4Address) {
                            hasIPv4 = true;
                            break;
                        }
                    }
                    if (!hasIPv4) continue;

                    String name = nif.getName();
                    candidates.add(name);
                    logI("found interface: " + name + " (" + nif.getDisplayName() + ")");
                } catch (Exception e) {
                    // Log.w(TAG, "skip interface check", e);
                }
            }

            // Prioritas: wlan (WiFi/hotspot) > eth > rmnet/rndis/usb (mobile/tether) > lainnya
            for (String name : candidates) {
                if (name.startsWith("wlan")) return name;
            }
            for (String name : candidates) {
                if (name.startsWith("eth")) return name;
            }
            for (String name : candidates) {
                if (name.startsWith("rmnet") || name.startsWith("rndis") || name.startsWith("usb"))
                    return name;
            }
            if (!candidates.isEmpty()) return candidates.get(0);

        } catch (Exception e) {
            // Log.e(TAG, "detectActiveInterface failed", e);
        }
        return "wlan0"; // default fallback untuk WiFi hotspot
    }

    // ──────────────────────────────────────────────
    // Sing-box Config Builder
    // ──────────────────────────────────────────────

    private String buildSingBoxConfig(String host, int port, String user, String pass, String bindIface) {
        StringBuilder sb = new StringBuilder();
        sb.append("{");

        // ── Log ──
        String logLevel = BuildConfig.DEBUG ? "debug" : "warn";
        sb.append("\"log\":{\"level\":\"").append(logLevel).append("\",\"timestamp\":true},");

        // ── DNS ──
        // Remote DNS via SOCKS tunnel (privasi + geo-unblock)
        // Local DNS via direct (bootstrap / resolve internal)
        sb.append("\"dns\":{");
        sb.append("\"servers\":[");
        // Satu DNS saja: remote via SOCKS TCP. Jangan sediakan local/direct DNS agar tidak ada path bocor.
        sb.append("{\"tag\":\"remote\",\"address\":\"tcp://8.8.8.8\",\"detour\":\"socks-out\"}");
        sb.append("],");
        sb.append("\"final\":\"remote\",");
        sb.append("\"strategy\":\"ipv4_only\"");
        sb.append("},");

        // ── Inbounds (TUN) ──
        sb.append("\"inbounds\":[{");
        sb.append("\"type\":\"tun\",");
        sb.append("\"tag\":\"tun-in\",");
        sb.append("\"interface_name\":\"sb-tun\",");
        sb.append("\"address\":[\"172.19.0.1/30\"],");
        sb.append("\"mtu\":1400,");
        sb.append("\"auto_route\":true,");
        sb.append("\"strict_route\":true,"); // Force semua traffic lewat TUN
        // Gaming UDP paling aman tanpa sniff/rewrite destination.
        // Sniff override bisa mengubah destination dan bikin handshake game salah server.
        sb.append("\"sniff\":false,");
        sb.append("\"sniff_override_destination\":false");
        sb.append("}],");

        // ── Outbounds ──
        sb.append("\"outbounds\":[");

        // SOCKS5 outbound
        sb.append("{");
        sb.append("\"type\":\"socks\",");
        sb.append("\"tag\":\"socks-out\",");
        sb.append("\"server\":\"").append(escapeJson(host)).append("\",");
        sb.append("\"server_port\":").append(port).append(",");
        sb.append("\"version\":\"5\"");

        // Auth opsional
        if (user != null && !user.trim().isEmpty() && pass != null && !pass.isEmpty()) {
            sb.append(",\"username\":\"").append(escapeJson(user.trim())).append("\"");
            sb.append(",\"password\":\"").append(escapeJson(pass)).append("\"");
        }

        // bind_interface agar traffic ke SOCKS server keluar via interface yang benar (bukan loop ke TUN)
        if (bindIface != null && !bindIface.isEmpty()) {
            sb.append(",\"bind_interface\":\"").append(escapeJson(bindIface)).append("\"");
        }

        sb.append("},");

        // Direct outbound (untuk bootstrap DNS + bypass SOCKS server IP)
        sb.append("{");
        sb.append("\"type\":\"direct\",");
        sb.append("\"tag\":\"direct\"");
        if (bindIface != null && !bindIface.isEmpty()) {
            sb.append(",\"bind_interface\":\"").append(escapeJson(bindIface)).append("\"");
        }
        sb.append("}");

        sb.append("],");

        // ── Route ──
        sb.append("\"route\":{");
        sb.append("\"auto_detect_interface\":true,");
        sb.append("\"rules\":[");

        // SOCKS server IP → direct (anti routing loop!)
        sb.append("{\"ip_cidr\":[\"").append(escapeJson(host)).append("/32\"],\"outbound\":\"direct\"},");

        // Block ALL IPv6 — server tidak support IPv6 routing
        sb.append("{\"ip_version\":6,\"action\":\"reject\"}");

        sb.append("],");
        sb.append("\"final\":\"socks-out\"");
        sb.append("}");

        sb.append("}");
        return sb.toString();
    }

    // ──────────────────────────────────────────────
    // Helpers
    // ──────────────────────────────────────────────

    private String escapeJson(String s) {
        if (s == null) return "";
        return s.replace("\\", "\\\\")
                .replace("\"", "\\\"")
                .replace("\n", "\\n")
                .replace("\r", "\\r")
                .replace("\t", "\\t");
    }

    private void setStatus(boolean connected, String status) {
        SharedPreferences sp = getSharedPreferences(PREFS, MODE_PRIVATE);
        sp.edit()
                .putBoolean(KEY_CONNECTED, connected)
                .putString(KEY_STATUS, status)
                .putLong(KEY_LAST_SEEN, System.currentTimeMillis())
                .apply();
    }

    private void notifyStatus(String content) {
        NotificationManager manager = (NotificationManager) getSystemService(NOTIFICATION_SERVICE);
        if (manager != null) {
            manager.notify(NOTIF_ID, buildNotification(content));
        }
    }

    private Notification buildNotification(String content) {
        createChannel();
        Intent openIntent = new Intent(this, MainActivity.class);
        PendingIntent contentIntent = PendingIntent.getActivity(
                this, 0, openIntent, PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);

        String displayText = content;
        if (trafficCounterEnabled && running) {
            displayText = content + "  |  ↑ " + formatBytes(uploadBytes.get())
                    + "  ↓ " + formatBytes(downloadBytes.get());
        }

        return new Notification.Builder(this, CHANNEL_ID)
                .setSmallIcon(android.R.drawable.stat_sys_upload_done)
                .setContentTitle("Socks Client VPN")
                .setContentText(displayText)
                .setOngoing(true)
                .setContentIntent(contentIntent)
                .build();
    }

    private void createChannel() {
        if (Build.VERSION.SDK_INT < 26) return;
        NotificationManager manager = (NotificationManager) getSystemService(NOTIFICATION_SERVICE);
        if (manager == null) return;
        NotificationChannel channel = new NotificationChannel(CHANNEL_ID, "Socks Client", NotificationManager.IMPORTANCE_LOW);
        manager.createNotificationChannel(channel);
    }

    private String safeMessage(Throwable e) {
        if (e == null) return "unknown";
        String msg = e.getMessage();
        return (msg == null || msg.trim().isEmpty()) ? e.getClass().getSimpleName() : msg;
    }

    private void logI(String m) {
        // if (BuildConfig.DEBUG) {
        //     Log.i(TAG, m);
        //     DebugLog.append(this, m);
        // }
    }

    private void logE(String m, Throwable t) {
        // Log.e(TAG, m, t);
        // if (BuildConfig.DEBUG) {
        //     DebugLog.append(this, m + ": " + safeMessage(t));
        // }
    }

    // ══════════════════════════════════════════════
    // PlatformInterface implementation
    // ══════════════════════════════════════════════

    @Override
    public void autoDetectInterfaceControl(int fd) {
        protect(fd);
    }

    @Override
    public void clearDNSCache() {
    }

    @Override
    public void closeDefaultInterfaceMonitor(InterfaceUpdateListener listener) {
    }

    @Override
    public ConnectionOwner findConnectionOwner(int i, String s, int i1, String s1, int i2) {
        return new ConnectionOwner();
    }

    @Override
    public NetworkInterfaceIterator getInterfaces() {
        final List<NetworkInterface> result = new ArrayList<>();
        try {
            Enumeration<java.net.NetworkInterface> nifs = java.net.NetworkInterface.getNetworkInterfaces();
            while (nifs != null && nifs.hasMoreElements()) {
                java.net.NetworkInterface jni = nifs.nextElement();
                try {
                    if (!jni.isUp() || jni.isLoopback() || jni.isVirtual() || jni.isPointToPoint()) continue;

                    NetworkInterface libIf = new NetworkInterface();
                    libIf.setName(jni.getName());
                    libIf.setIndex(jni.getIndex());
                    libIf.setMTU(jni.getMTU());

                    // Kumpulkan addresses — IPv4 ONLY (skip semua IPv6 biar gak crash di Go)
                    List<String> addrs = new ArrayList<>();
                    for (java.net.InterfaceAddress ia : jni.getInterfaceAddresses()) {
                        if (ia.getAddress() instanceof Inet4Address) {
                            String cidr = ia.getAddress().getHostAddress() + "/" + ia.getNetworkPrefixLength();
                            addrs.add(cidr);
                        }
                    }

                    // Skip interface yang gak punya IPv4 (contoh: rmnet_data0, dummy0)
                    if (addrs.isEmpty()) continue;

                    // Wrap ke StringIterator
                    final List<String> addrList = new ArrayList<>(addrs);
                    libIf.setAddresses(new io.nekohasekai.libbox.StringIterator() {
                        int i = 0;
                        @Override public boolean hasNext() { return i < addrList.size(); }
                        @Override public String next() { return addrList.get(i++); }
                        @Override public int len() { return addrList.size(); }
                    });

                    // Flags
                    int flags = 0;
                    if (jni.isUp()) flags |= 1;
                    if (jni.supportsMulticast()) flags |= 2;
                    libIf.setFlags(flags);

                    logI("getInterfaces: " + jni.getName() + " index=" + jni.getIndex()
                            + " addrs=" + addrs + " mtu=" + jni.getMTU());
                    result.add(libIf);
                } catch (Throwable t) {
                    // Log.w(TAG, "getInterfaces: skip " + jni.getName(), t);
                }
            }
        } catch (Exception e) {
            // Log.w(TAG, "getInterfaces enumeration failed", e);
        }

        logI("getInterfaces: returning " + result.size() + " interfaces");
        return new NetworkInterfaceIterator() {
            int idx = 0;
            @Override public boolean hasNext() { return idx < result.size(); }
            @Override public NetworkInterface next() { return result.get(idx++); }
        };
    }

    @Override
    public boolean includeAllNetworks() {
        return false;
    }

    @Override
    public LocalDNSTransport localDNSTransport() {
        return null;
    }

    @Override
    public int openTun(TunOptions options) throws Exception {
        logI("openTun called");

        Builder builder = new Builder()
                .setSession("sing-box")
                .setMtu(options.getMTU());

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            builder.setMetered(false);
        }

        // IPv4 addresses
        RoutePrefixIterator v4Addr = options.getInet4Address();
        boolean hasV4 = false;
        while (v4Addr != null && v4Addr.hasNext()) {
            RoutePrefix p = v4Addr.next();
            builder.addAddress(p.address(), p.prefix());
            hasV4 = true;
        }
        if (!hasV4) {
            builder.addAddress("172.19.0.1", 30);
        }

        // IPv6 addresses
        RoutePrefixIterator v6Addr = options.getInet6Address();
        boolean hasV6 = false;
        while (v6Addr != null && v6Addr.hasNext()) {
            RoutePrefix p = v6Addr.next();
            builder.addAddress(p.address(), p.prefix());
            hasV6 = true;
        }
        if (!hasV6) {
            try {
                builder.addAddress("fdfe:dcba:9876::1", 126);
            } catch (Exception ignored) {
            }
        }

        if (options.getAutoRoute()) {
            // DNS server
            StringBox dns = options.getDNSServerAddress();
            if (dns != null && dns.getValue() != null && !dns.getValue().isEmpty()) {
                try {
                    builder.addDnsServer(dns.getValue());
                } catch (Exception e) {
                    // Log.w(TAG, "addDnsServer failed: " + dns.getValue(), e);
                }
            }
            // Fallback DNS
            builder.addDnsServer("8.8.8.8");
            try {
                builder.addDnsServer("1.1.1.1");
            } catch (Exception ignored) {
            }

            // IPv4 routes
            RoutePrefixIterator v4Route = options.getInet4RouteAddress();
            boolean hasV4Route = false;
            if (v4Route != null) {
                while (v4Route.hasNext()) {
                    RoutePrefix p = v4Route.next();
                    builder.addRoute(p.address(), p.prefix());
                    hasV4Route = true;
                }
            }
            if (!hasV4Route) {
                builder.addRoute("0.0.0.0", 0); // catch-all IPv4
            }

            // IPv6 routes
            RoutePrefixIterator v6Route = options.getInet6RouteAddress();
            boolean hasV6Route = false;
            if (v6Route != null) {
                while (v6Route.hasNext()) {
                    RoutePrefix p = v6Route.next();
                    builder.addRoute(p.address(), p.prefix());
                    hasV6Route = true;
                }
            }
            if (!hasV6Route) {
                try {
                    builder.addRoute("::", 0); // catch-all IPv6
                } catch (Exception ignored) {
                }
            }
        }

        // Exclude packages
        StringIterator exclude = options.getExcludePackage();
        while (exclude != null && exclude.hasNext()) {
            String pkg = exclude.next();
            try {
                builder.addDisallowedApplication(pkg);
            } catch (Exception ignored) {
            }
        }

        // Include packages
        StringIterator include = options.getIncludePackage();
        while (include != null && include.hasNext()) {
            String pkg = include.next();
            try {
                builder.addAllowedApplication(pkg);
            } catch (Exception ignored) {
            }
        }

        // Exclude app sendiri agar tidak loop
        try {
            builder.addDisallowedApplication(getPackageName());
        } catch (Exception ignored) {
        }

        ParcelFileDescriptor pfd = builder.establish();
        if (pfd == null) throw new IllegalStateException("builder.establish() returned null");
        vpnFd = pfd;
        logI("openTun established, fd=" + pfd.getFd());
        return pfd.getFd();
    }

    @Override
    public WIFIState readWIFIState() {
        return new WIFIState("", "");
    }

    @Override
    public void sendNotification(io.nekohasekai.libbox.Notification notification) {
        String body = notification == null ? "Running" : notification.getBody();
        notifyStatus(body == null || body.isEmpty() ? "Running" : body);
    }

    @Override
    public void startDefaultInterfaceMonitor(InterfaceUpdateListener listener) {
        try {
            // Gunakan ConnectivityManager untuk menemukan network interface aktif
            android.net.ConnectivityManager cm =
                    (android.net.ConnectivityManager) getSystemService(CONNECTIVITY_SERVICE);
            android.net.Network activeNetwork = cm.getActiveNetwork();

            if (activeNetwork == null) {
                logI("startDefaultInterfaceMonitor: no active network, report empty");
                listener.updateDefaultInterface("", 0, false, false);
                return;
            }

            android.net.LinkProperties lp = cm.getLinkProperties(activeNetwork);
            if (lp == null) {
                logI("startDefaultInterfaceMonitor: no LinkProperties, report empty");
                listener.updateDefaultInterface("", 0, false, false);
                return;
            }

            String ifaceName = lp.getInterfaceName();
            logI("startDefaultInterfaceMonitor: active interface = " + ifaceName);

            // Cari index dari Java NetworkInterface
            int ifaceIndex = 0;
            boolean isExpensive = false;
            boolean isConstrained = false;

            if (ifaceName != null && !ifaceName.isEmpty()) {
                try {
                    java.net.NetworkInterface jni = java.net.NetworkInterface.getByName(ifaceName);
                    if (jni != null) {
                        ifaceIndex = jni.getIndex();
                        logI("startDefaultInterfaceMonitor: index=" + ifaceIndex
                                + " mtu=" + jni.getMTU() + " up=" + jni.isUp());
                    }
                } catch (Exception e) {
                    // Log.w(TAG, "getByName failed for " + ifaceName, e);
                }

                // Cek apakah network expensive (metered) atau constrained
                android.net.NetworkCapabilities caps = cm.getNetworkCapabilities(activeNetwork);
                if (caps != null) {
                    isExpensive = !caps.hasCapability(android.net.NetworkCapabilities.NET_CAPABILITY_NOT_METERED);
                    isConstrained = !caps.hasCapability(android.net.NetworkCapabilities.NET_CAPABILITY_NOT_SUSPENDED);
                }
            }

            // LAPOR ke sing-box — ini yang bikin auto_detect_interface bekerja!
            listener.updateDefaultInterface(
                    ifaceName != null ? ifaceName : "",
                    ifaceIndex,
                    isExpensive,
                    isConstrained
            );
            logI("startDefaultInterfaceMonitor: reported iface=" + ifaceName
                    + " index=" + ifaceIndex + " expensive=" + isExpensive
                    + " constrained=" + isConstrained);

        } catch (Exception e) {
            // Log.e(TAG, "startDefaultInterfaceMonitor failed", e);
            try {
                listener.updateDefaultInterface("", 0, false, false);
            } catch (Exception ignored) {}
        }
    }

    @Override
    public StringIterator systemCertificates() {
        final List<String> empty = new ArrayList<>();
        return new StringIterator() {
            int index = 0;

            @Override
            public boolean hasNext() {
                return index < empty.size();
            }

            @Override
            public int len() {
                return empty.size();
            }

            @Override
            public String next() {
                return empty.get(index++);
            }
        };
    }

    @Override
    public boolean underNetworkExtension() {
        return false;
    }

    @Override
    public boolean usePlatformAutoDetectInterfaceControl() {
        return true;
    }

    @Override
    public boolean useProcFS() {
        return false;
    }

    // ══════════════════════════════════════════════
    // CommandServerHandler implementation
    // ══════════════════════════════════════════════

    @Override
    public SystemProxyStatus getSystemProxyStatus() {
        return new SystemProxyStatus();
    }

    @Override
    public void serviceReload() {
        logI("serviceReload requested");
    }

    @Override
    public void serviceStop() {
        logI("serviceStop requested by sing-box");
        new Thread(this::disconnectInternal, "sb-serviceStop").start();
    }

    @Override
    public void setSystemProxyEnabled(boolean b) {
    }

    @Override
    public void writeDebugMessage(String s) {
        logI("libbox: " + s);
    }
}
