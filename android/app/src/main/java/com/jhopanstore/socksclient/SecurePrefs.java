package com.jhopanstore.socksclient;

import android.content.Context;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.util.Base64;

import java.security.KeyStore;

import javax.crypto.Cipher;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;

/**
 * Menyimpan password SOCKS dalam bentuk terenkripsi.
 *
 * Kunci AES-256 dibuat dan disimpan di Android Keystore (tidak bisa diekspor),
 * jadi membaca SharedPreferences mentah tidak cukup untuk membuka password -
 * penyerang harus bisa menjalankan kode sebagai app ini di perangkat yang sama.
 * Tanpa dependency baru: hanya API platform.
 */
final class SecurePrefs {
    private static final String KEYSTORE = "AndroidKeyStore";
    private static final String KEY_ALIAS = "socks_client_pass_v1";
    private static final String TRANSFORMATION = "AES/GCM/NoPadding";
    private static final int IV_LENGTH = 12;
    private static final int TAG_BITS = 128;

    private SecurePrefs() {
    }

    /** @return base64(iv || ciphertext) atau null bila gagal. */
    static String encrypt(Context context, String plain) {
        if (plain == null) return null;
        try {
            SecretKey key = getOrCreateKey();
            Cipher cipher = Cipher.getInstance(TRANSFORMATION);
            cipher.init(Cipher.ENCRYPT_MODE, key);
            byte[] encrypted = cipher.doFinal(plain.getBytes("UTF-8"));
            byte[] iv = cipher.getIV();
            byte[] blob = new byte[iv.length + encrypted.length];
            System.arraycopy(iv, 0, blob, 0, iv.length);
            System.arraycopy(encrypted, 0, blob, iv.length, encrypted.length);
            return Base64.encodeToString(blob, Base64.NO_WRAP);
        } catch (Throwable t) {
            return null;
        }
    }

    /** @return password asli, atau null bila blob tidak bisa dibuka. */
    static String decrypt(Context context, String blob) {
        if (blob == null || blob.isEmpty()) return null;
        try {
            byte[] raw = Base64.decode(blob, Base64.NO_WRAP);
            if (raw.length <= IV_LENGTH) return null;
            byte[] iv = new byte[IV_LENGTH];
            System.arraycopy(raw, 0, iv, 0, IV_LENGTH);
            byte[] body = new byte[raw.length - IV_LENGTH];
            System.arraycopy(raw, IV_LENGTH, body, 0, body.length);

            SecretKey key = getOrCreateKey();
            Cipher cipher = Cipher.getInstance(TRANSFORMATION);
            cipher.init(Cipher.DECRYPT_MODE, key, new GCMParameterSpec(TAG_BITS, iv));
            return new String(cipher.doFinal(body), "UTF-8");
        } catch (Throwable t) {
            return null;
        }
    }

    private static SecretKey getOrCreateKey() throws Exception {
        KeyStore keyStore = KeyStore.getInstance(KEYSTORE);
        keyStore.load(null);
        KeyStore.Entry entry = keyStore.getEntry(KEY_ALIAS, null);
        if (entry instanceof KeyStore.SecretKeyEntry) {
            return ((KeyStore.SecretKeyEntry) entry).getSecretKey();
        }
        KeyGenerator generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, KEYSTORE);
        generator.init(new KeyGenParameterSpec.Builder(KEY_ALIAS,
                KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .build());
        return generator.generateKey();
    }
}
