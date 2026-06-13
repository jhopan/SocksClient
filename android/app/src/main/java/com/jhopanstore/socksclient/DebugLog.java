package com.jhopanstore.socksclient;

import android.content.Context;
import android.content.SharedPreferences;

import java.text.SimpleDateFormat;
import java.util.Date;
import java.util.Locale;

final class DebugLog {
    private static final String PREFS = "socks_client_debug";
    private static final String KEY_LOG = "log";
    private static final int MAX_CHARS = 12000;

    private DebugLog() {}

    static void append(Context context, String message) {
        if (context == null || message == null) return;
        String ts = new SimpleDateFormat("HH:mm:ss.SSS", Locale.US).format(new Date());
        String line = ts + " | " + message + "\n";
        SharedPreferences sp = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        String old = sp.getString(KEY_LOG, "");
        String merged = old + line;
        if (merged.length() > MAX_CHARS) {
            merged = merged.substring(merged.length() - MAX_CHARS);
        }
        sp.edit().putString(KEY_LOG, merged).apply();
    }

    static String read(Context context) {
        if (context == null) return "";
        return context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).getString(KEY_LOG, "");
    }

    static void clear(Context context) {
        if (context == null) return;
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit().remove(KEY_LOG).apply();
    }
}
