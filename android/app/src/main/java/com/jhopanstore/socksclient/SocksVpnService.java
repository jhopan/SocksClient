package com.jhopanstore.socksclient;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Intent;
import android.content.SharedPreferences;
import android.net.ConnectivityManager;

import java.io.IOException;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.InetSocketAddress;
import java.net.Proxy;
import java.net.URL;
import android.net.Network;
import android.net.VpnService;
import android.os.Build;
import android.os.ParcelFileDescriptor;
// import android.util.Log;

import java.util.Locale;
import java.util.concurrent.atomic.AtomicLong;

import io.nekohasekai.libbox.BridgeOptions;
import io.nekohasekai.libbox.BridgeSession;
import io.nekohasekai.libbox.CommandServer;
import io.nekohasekai.libbox.CommandServerHandler;
import io.nekohasekai.libbox.ConnectionOwner;
import io.nekohasekai.libbox.LocalDNSTransport;
import io.nekohasekai.libbox.NeighborUpdateListener;
import io.nekohasekai.libbox.InterfaceUpdateListener;
import io.nekohasekai.libbox.Libbox;
import io.nekohasekai.libbox.NetworkInterface;
import io.nekohasekai.libbox.NetworkInterfaceIterator;
import io.nekohasekai.libbox.PlatformInterface;
import io.nekohasekai.libbox.PlatformUser;
import io.nekohasekai.libbox.ShellSession;
import io.nekohasekai.libbox.RoutePrefix;
import io.nekohasekai.libbox.RoutePrefixIterator;
import io.nekohasekai.libbox.SetupOptions;
import io.nekohasekai.libbox.StringIterator;
import io.nekohasekai.libbox.SystemProxyStatus;
import io.nekohasekai.libbox.TunOptions;
import io.nekohasekai.libbox.WIFIState;

import java.net.Inet4Address;
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
    private static final long TRAFFIC_POLL_MS = 10000;

    // ── HTTP ping ──
    // Proses aplikasi ini dikecualikan dari VPN (addDisallowedApplication), jadi
    // trafiknya TIDAK masuk TUN. Ping karena itu dikirim ke inbound loopback di
    // bawah ini supaya sing-box yang meneruskannya lewat socks-out: hasilnya
    // benar-benar menguji jalur app -> sing-box -> SOCKS -> internet.
    private static final int PING_PORT = 2081;
    private static final long PING_INTERVAL_MS = 30000;
    private static final String PING_PREFS_KEY = "ping_enabled";
    private static final String PING_RESULT_KEY = "ping_result";
    private static final String[] PING_TARGETS = {
            "http://connectivitycheck.gstatic.com/generate_204",
            "http://cp.cloudflare.com/generate_204",
    };
    private static final long HEARTBEAT_MS = 10000;

    private final ExecutorService worker = Executors.newSingleThreadExecutor();
    private final Object lock = new Object();
    private volatile boolean running;
    private volatile boolean connecting;
    private volatile boolean stopping;
    private CommandServer commandServer;
    private ParcelFileDescriptor vpnFd;
    private Thread heartbeatThread;

    // S2: last used server + network watchdog state
    private volatile String serverHost;
    private volatile int serverPort;
    private volatile String serverUser;
    private volatile String serverPass;
    private ConnectivityManager.NetworkCallback networkCallback;
    private volatile boolean pingRunning;
    private Thread pingThread;
    private volatile long lastNetworkRestart;

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
                try {
            startForeground(NOTIF_ID, buildNotification("Connecting..."));
        } catch (Throwable e) {
            // Jangan matikan tunnel hanya karena notifikasi ditolak sistem.
            logE("startForeground ditolak, lanjut tanpa foreground", e);
        }
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
    // S2: network watchdog
    // ──────────────────────────────────────────────

    /** Reload the tunnel when the underlying network changes or disappears. */
    private void registerNetworkWatchdog() {
        if (networkCallback != null) return;
        ConnectivityManager cm = (ConnectivityManager) getSystemService(CONNECTIVITY_SERVICE);
        if (cm == null) return;
        networkCallback = new ConnectivityManager.NetworkCallback() {
            @Override
            public void onAvailable(Network network) {
                onNetworkChanged("tersedia");
            }

            @Override
            public void onLost(Network network) {
                onNetworkChanged("hilang");
            }
        };
        try {
            cm.registerDefaultNetworkCallback(networkCallback);
            logI("network watchdog registered");
        } catch (Exception e) {
            networkCallback = null;
            logE("registerDefaultNetworkCallback failed", e);
        }
    }

    private void unregisterNetworkWatchdog() {
        ConnectivityManager cm = (ConnectivityManager) getSystemService(CONNECTIVITY_SERVICE);
        if (cm != null && networkCallback != null) {
            try {
                cm.unregisterNetworkCallback(networkCallback);
            } catch (Exception ignored) {
            }
        }
        networkCallback = null;
    }

    /**
     * WiFi -> seluler atau hotspot putus-nyambung membuat koneksi SOCKS mati
     * sementara VPN-nya masih "Connected". Callback ini memuat ulang sing-box
     * dengan config yang sama (server berupa IP, jadi tidak ada yang perlu
     * di-resolve ulang). Diberi jeda 15 detik supaya jaringan yang berkedip tidak
     * memicu restart berulang.
     */
    private void onNetworkChanged(String reason) {
        if (!running || stopping || connecting || commandServer == null) return;
        long now = System.currentTimeMillis();
        if (now - lastNetworkRestart < 15000) return;
        lastNetworkRestart = now;
        logI("network changed (" + reason + ") - reloading sing-box");

        worker.execute(() -> {
            try {
                String config = buildSingBoxConfig(serverHost, serverPort, serverUser, serverPass, null);
                commandServer.startOrReloadService(config, null);
                setStatus(true, "Connected");
                notifyStatus("Connected");
                logI("reloaded after network change");
            } catch (Throwable t) {
                logE("reload after network change failed, reconnecting", t);
                synchronized (lock) {
                    running = false;
                }
                disconnectCoreOnly();
                connectInternal(serverHost, serverPort, serverUser, serverPass);
            }
        });
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
            Libbox.setup(setupOptions);

            // Start command server (holds the platform bridge and the box instance)
            commandServer = Libbox.newCommandServer(this, this);
            commandServer.start();

            // Build config dan start service (bind_interface = null, pakai auto_detect_interface)
            String config = buildSingBoxConfig(host.trim(), port, user, pass, null);
            logI("starting sing-box, config-length=" + config.length() + ", auto_detect_interface");
            if (BuildConfig.DEBUG) {
                logI("config: " + redactSecrets(config));
            }
            commandServer.startOrReloadService(config, null);

            serverHost = host.trim();
            serverPort = port;
            serverUser = user;
            serverPass = pass;

            synchronized (lock) {
                running = true;
                connecting = false;
            }

            setStatus(true, "Connected");
            notifyStatus("Connected");
            registerNetworkWatchdog();
            startHeartbeat();
            startPingMonitor();
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
        unregisterNetworkWatchdog();
        stopHeartbeat();
        stopPingMonitor();
        stopTrafficMonitor();
        disconnectCoreOnly();
        setStatus(false, "Disconnected");
        // Clear traffic stats and last_seen on disconnect
        SharedPreferences sp = getSharedPreferences(PREFS, MODE_PRIVATE);
        sp.edit()
                .putLong(KEY_UPLOAD_BYTES, 0)
                .putLong(KEY_DOWNLOAD_BYTES, 0)
                .putLong(KEY_LAST_SEEN, 0)
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
    // Heartbeat — agar UI tahu service masih hidup.
    // Interval sengaja longgar (10s): tiap tulisan ke SharedPreferences adalah
    // wakeup, dan layar mati tidak boleh berarti VPN mati. UI memakai jendela
    // LAST_SEEN_WINDOW_MS = 3x interval ini supaya tidak salah anggap mati.
    // ──────────────────────────────────────────────

    private void startHeartbeat() {
        stopHeartbeat();
        heartbeatThread = new Thread(() -> {
            while (running && !stopping) {
                try {
                    SharedPreferences sp = getSharedPreferences(PREFS, MODE_PRIVATE);
                    sp.edit().putLong(KEY_LAST_SEEN, System.currentTimeMillis()).apply();
                    Thread.sleep(HEARTBEAT_MS);
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

                    // Update notification with latest traffic, but only when it
                    // changed: re-posting the same text every poll is a needless
                    // wakeup for the notification service.
                    if (running && (curTx != prevTx || curRx != prevRx)) {
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
    // Sing-box Config Builder
    // ──────────────────────────────────────────────

    /** Anti-loop rule format depends on whether the server is an IP or a name. */
    private static boolean isIpLiteral(String host) {
        if (host == null) return false;
        String h = host.trim();
        if (h.matches("^\\d{1,3}(\\.\\d{1,3}){3}$")) return true;
        return h.contains(":") && h.matches("^[0-9a-fA-F:]+$");
    }

    private String buildSingBoxConfig(String host, int port, String user, String pass, String bindIface) {
        StringBuilder sb = new StringBuilder();
        sb.append("{");

        // ── Log ──
        String logLevel = BuildConfig.DEBUG ? "debug" : "warn";
        sb.append("\"log\":{\"level\":\"").append(logLevel).append("\",\"timestamp\":true},");

        // ── DNS ──
        // New server format (sing-box >= 1.12; the legacy "address" field was
        // removed in 1.14, which is the core this app ships). Everything is
        // resolved through the SOCKS tunnel; "local" is only the bootstrap that
        // resolves the server hostname itself.
        sb.append("\"dns\":{");
        sb.append("\"servers\":[");
        sb.append("{\"tag\":\"remote\",\"type\":\"tcp\",\"server\":\"1.1.1.1\",\"detour\":\"socks-out\"},");
        sb.append("{\"tag\":\"remote-udp\",\"type\":\"udp\",\"server\":\"1.1.1.1\",\"detour\":\"socks-out\"},");
        // Backup resolver. sing-box has no automatic failover between servers, so
        // this one is here to be promoted by changing "final" when 1.1.1.1 is
        // blocked on the network you are on.
        sb.append("{\"tag\":\"backup\",\"type\":\"tcp\",\"server\":\"8.8.8.8\",\"detour\":\"socks-out\"},");
        sb.append("{\"tag\":\"local\",\"type\":\"local\"}");
        sb.append("],");
        sb.append("\"final\":\"remote\",");
        sb.append("\"strategy\":\"ipv4_only\"");
        sb.append("},");

        // ── Inbounds (TUN) ──
        sb.append("\"inbounds\":[{\"type\":\"tun\",\"tag\":\"tun-in\",\"interface_name\":\"sb-tun\",");
        sb.append("\"address\":[\"172.19.0.1/30\"],\"mtu\":1400,");
        sb.append("\"auto_route\":true,\"strict_route\":true,");
        // gVisor stack: L3-L4 translation happens in userspace, so the tunnel
        // does not depend on the device's kernel/driver quirks. Same choice as
        // the desktop client.
        sb.append("\"stack\":\"gvisor\"");
        // Inbound loopback untuk HTTP ping (PING_PORT). Hanya 127.0.0.1, jadi
        // tidak ada yang bisa menyentuhnya dari luar perangkat.
        sb.append("},{\"type\":\"mixed\",\"tag\":\"ping-in\",\"listen\":\"127.0.0.1\",\"listen_port\":").append(PING_PORT);
        // NOTE: "sniff"/"sniff_override_destination" lived here until sing-box
        // 1.13 removed legacy inbound fields - sniffing is a route action now.
        sb.append("}],");

        // ── Outbounds ──
        sb.append("\"outbounds\":[");

        // SOCKS5 outbound
        sb.append("{\"type\":\"socks\",\"tag\":\"socks-out\",");
        sb.append("\"server\":\"").append(escapeJson(host)).append("\",");
        sb.append("\"server_port\":").append(port).append(",");
        sb.append("\"version\":\"5\"");
        if (user != null && !user.trim().isEmpty() && pass != null && !pass.isEmpty()) {
            sb.append(",\"username\":\"").append(escapeJson(user.trim())).append("\"");
            sb.append(",\"password\":\"").append(escapeJson(pass)).append("\"");
        }
        // bind_interface agar koneksi ke SOCKS server keluar dari interface
        // fisik, bukan berbalik masuk ke TUN.
        if (bindIface != null && !bindIface.isEmpty()) {
            sb.append(",\"bind_interface\":\"").append(escapeJson(bindIface)).append("\"");
        }
        sb.append("},");

        // Direct outbound (bootstrap + bypass IP server SOCKS)
        sb.append("{\"type\":\"direct\",\"tag\":\"direct\"");
        if (bindIface != null && !bindIface.isEmpty()) {
            sb.append(",\"bind_interface\":\"").append(escapeJson(bindIface)).append("\"");
        }
        sb.append("}");

        sb.append("],");

        // ── Route ──
        sb.append("\"route\":{");
        sb.append("\"auto_detect_interface\":true,");
        // Resolver untuk hostname server (bootstrap) - lihat blok dns di atas.
        sb.append("\"default_domain_resolver\":{\"server\":\"local\"},");
        sb.append("\"rules\":[");
        // Lapis anti-DNS-leak: SEMUA paket DNS (termasuk yang menembak resolver
        // LAN/ISP) ditangkap di sini dan dijawab server "remote" lewat socks-out.
        // Sebelumnya memakai block port-53 yang khas sing-box 1.10; "action"
        // hijack-dns tersedia sejak 1.11 dan core kita 1.14.
        // Sniff only DNS: the tun inbound auto-hijacks UDP DNS aimed at the VPN
        // DNS address (tun address + 1), but queries to any other resolver - a
        // hardcoded one in some app, or DNS over TCP after a truncated answer -
        // are only recognised as DNS by sniffing. In sing-box 1.14 the sniff
        // action carries no destination override, so this cannot rewrite where a
        // connection goes.
        sb.append("{\"action\":\"sniff\",\"sniffer\":[\"dns\"]},");
        sb.append("{\"protocol\":\"dns\",\"action\":\"hijack-dns\"},");

        // IPv6: the route still sends ::/0 into the tunnel (that is what stops a
        // v6 leak), but DNS is ipv4_only so no AAAA ever resolves. Reject v6
        // here so apps fail instantly instead of hanging on a dead socket.
        sb.append("{\"ip_cidr\":[\"::/0\"],\"action\":\"reject\"},");

        // Server SOCKS -> direct (anti routing loop)
        if (isIpLiteral(host)) {
            sb.append("{\"ip_cidr\":[\"").append(escapeJson(host)).append("/32\"],\"outbound\":\"direct\"}");
        } else {
            sb.append("{\"domain\":[\"").append(escapeJson(host)).append("\"],\"outbound\":\"direct\"}");
        }

        sb.append("],");
        sb.append("\"final\":\"socks-out\"");
        sb.append("},");

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
        Intent openIntent = new Intent(this, MainActivity.class);
        PendingIntent contentIntent = PendingIntent.getActivity(
                this, 0, openIntent, PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);

        Notification.Builder builder;
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            createChannel();
            builder = new Notification.Builder(this, CHANNEL_ID);
        } else {
            // API 24-25 tidak mengenal channel; konstruktor dua argumen hanya ada
            // sejak API 26 dan memanggilnya di sini akan crash.
            builder = new Notification.Builder(this);
            builder.setPriority(Notification.PRIORITY_LOW);
        }

        return builder
                .setSmallIcon(R.drawable.app_icon_foreground)
                .setContentTitle("Socks Client")
                .setContentText(content)
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

    // ──────────────────────────────────────────────
    // HTTP ping (tombol On/Off di UI)
    // ──────────────────────────────────────────────

    private void startPingMonitor() {
        if (pingRunning) return;
        pingRunning = true;
        pingThread = new Thread(this::pingLoop, "http-ping");
        pingThread.start();
        logI("http ping monitor started");
    }

    private void stopPingMonitor() {
        pingRunning = false;
        if (pingThread != null) {
            pingThread.interrupt();
            pingThread = null;
        }
        getSharedPreferences(PREFS, MODE_PRIVATE).edit().remove(PING_RESULT_KEY).apply();
    }

    // Loop tipis: cek pref tiap 2 detik (supaya tombol On langsung bekerja) tapi
    // hanya mengirim ping tiap PING_INTERVAL_MS. Saat Off, hasil lama dihapus
    // supaya UI tidak menampilkan angka basi.
    private void pingLoop() {
        long next = 0;
        while (pingRunning) {
            SharedPreferences sp = getSharedPreferences(PREFS, MODE_PRIVATE);
            boolean enabled = sp.getBoolean(PING_PREFS_KEY, false) && running;
            if (!enabled) {
                sp.edit().remove(PING_RESULT_KEY).apply();
                next = 0;
            } else if (System.currentTimeMillis() >= next) {
                String result = pingOnce();
                if (pingRunning) sp.edit().putString(PING_RESULT_KEY, result).apply();
                next = System.currentTimeMillis() + PING_INTERVAL_MS;
            }
            try {
                Thread.sleep(2000);
            } catch (InterruptedException e) {
                return;
            }
        }
    }

    /**
     * Satu percobaan ping lewat inbound loopback (PING_PORT), jadi permintaan ini
     * benar-benar keluar melalui socks-out dan bukan internet langsung.
     */
    private String pingOnce() {
        String lastErr = "timeout";
        for (String target : PING_TARGETS) {
            HttpURLConnection conn = null;
            long start = System.currentTimeMillis();
            try {
                Proxy proxy = new Proxy(Proxy.Type.HTTP, new InetSocketAddress("127.0.0.1", PING_PORT));
                conn = (HttpURLConnection) new URL(target).openConnection(proxy);
                conn.setRequestMethod("GET");
                conn.setConnectTimeout(6000);
                conn.setReadTimeout(6000);
                conn.setUseCaches(false);
                int code = conn.getResponseCode();
                long ms = System.currentTimeMillis() - start;
                // Body 204 memang kosong; tutup saja supaya koneksi bisa keep-alive.
                InputStream in = code >= 400 ? conn.getErrorStream() : conn.getInputStream();
                if (in != null) in.close();
                if (code == 204) return "204 " + ms + "ms";
                lastErr = "HTTP " + code;
            } catch (Throwable t) {
                lastErr = safeMessage(t);
            } finally {
                if (conn != null) conn.disconnect();
            }
        }
        return "gagal (" + lastErr + ")";
    }

    // logcat tidak boleh memuat kredensial SOCKS dari config yang dicetak.
    private static String redactSecrets(String config) {
        return config.replaceAll("(\"(?:password|username)\"\\s*:\\s*\")[^\"]*(\")", "$1***$2");
    }

    private void logI(String m) {
        // Log.i(TAG, m);
    }

    private void logE(String m, Throwable t) {
        // Log.e(TAG, m, t);
    }

    // ══════════════════════════════════════════════
    // PlatformInterface implementation
    // ══════════════════════════════════════════════

    @Override
    public void autoDetectInterfaceControl(int fd) throws Exception {
        protect(fd);
    }

    @Override
    public void clearDNSCache() {
    }

    @Override
    public void closeDefaultInterfaceMonitor(InterfaceUpdateListener listener) throws Exception {
    }

    @Override
    public ConnectionOwner findConnectionOwner(int protocol, String source, int sourcePort,
                                                String destination, int destinationPort) throws Exception {
        // Only used to label connections in the (unused) dashboard API.
        return new ConnectionOwner();
    }

    @Override
    public NetworkInterfaceIterator getInterfaces() throws Exception {
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
            // VPN DNS. IMPORTANT: no public fallback here. Adding 8.8.8.8 /
            // 1.1.1.1 at the OS layer would give apps a resolver OUTSIDE the
            // core's DNS path - the classic DNS leak. Queries may only go to
            // the tunnel DNS (which the core hijacks and answers via socks-out).
            io.nekohasekai.libbox.StringIterator dnsServers = options.getDNSServerAddress();
            while (dnsServers != null && dnsServers.hasNext()) {
                String dns = dnsServers.next();
                if (dns == null || dns.trim().isEmpty()) continue;
                try {
                    builder.addDnsServer(dns);
                } catch (Exception e) {
                    // Log.w(TAG, "addDnsServer failed: " + dns, e);
                }
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
    public void sendNotification(io.nekohasekai.libbox.Notification notification) throws Exception {
        String body = notification == null ? "Running" : notification.getBody();
        notifyStatus(body == null || body.isEmpty() ? "Running" : body);
    }

    @Override
    public void startDefaultInterfaceMonitor(InterfaceUpdateListener listener) throws Exception {
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
            }

            // LAPOR ke sing-box — ini yang bikin auto_detect_interface bekerja!
            listener.updateDefaultInterface(
                    ifaceName != null ? ifaceName : "",
                    ifaceIndex,
                    false,
                    false
            );
            logI("startDefaultInterfaceMonitor: reported iface=" + ifaceName
                    + " index=" + ifaceIndex);

        } catch (Exception e) {
            // Log.e(TAG, "startDefaultInterfaceMonitor failed", e);
            try {
                listener.updateDefaultInterface("", 0, false, false);
            } catch (Exception ignored) {}
        }
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

    @Override
    public boolean usePlatformBridge() {
        return false;
    }

    @Override
    public boolean usePlatformShell() {
        return false;
    }

    @Override
    public String tailscaleHostname() {
        return "";
    }

    @Override
    public String lookupSFTPServer() {
        return "";
    }

    @Override
    public String readSystemSSHHostKey() {
        return "";
    }

    @Override
    public void registerMyInterface(String name) {
    }

    @Override
    public PlatformUser lookupUser(String username) {
        return null;
    }

    @Override
    public ShellSession openShellSession(PlatformUser user, String command, StringIterator args,
                                         String environment, int columns, int rows) {
        return null;
    }

    @Override
    public LocalDNSTransport localDNSTransport() {
        return null;
    }

    @Override
    public BridgeSession createBridge(BridgeOptions options) {
        return null;
    }

    @Override
    public void checkPlatformShell() {
    }

    @Override
    public void startNeighborMonitor(NeighborUpdateListener listener) {
    }

    @Override
    public void closeNeighborMonitor(NeighborUpdateListener listener) {
    }

    @Override
    public void cancelNotification(String tag, int id) {
    }

    // ══════════════════════════════════════════════
    // CommandServerHandler implementation
    // ══════════════════════════════════════════════

    @Override
    public SystemProxyStatus getSystemProxyStatus() {
        return new SystemProxyStatus();
    }

    @Override
    public void serviceStop() {
        logI("serviceStop requested by sing-box");
        new Thread(this::disconnectInternal, "sb-serviceStop").start();
    }

    @Override
    public int connectSSHAgent() {
        return -1;
    }

    @Override
    public void triggerNativeCrash() {
    }

    @Override
    public void writeDebugMessage(String message) {
        logI("libbox: " + message);
    }

    @Override
    public void serviceReload() throws Exception {
        logI("serviceReload requested");
    }

    @Override
    public void setSystemProxyEnabled(boolean b) throws Exception {
    }
}
