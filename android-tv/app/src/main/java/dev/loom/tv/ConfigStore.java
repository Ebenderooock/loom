package dev.loom.tv;

import android.content.Context;
import android.content.SharedPreferences;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.util.Base64;

import java.nio.charset.StandardCharsets;
import java.security.GeneralSecurityException;
import java.security.KeyStore;

import javax.crypto.Cipher;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;

/** Private preferences with a non-exportable Android Keystore key. Backups are disabled. */
final class ConfigStore {
    private static final String ALIAS = "loom.tv.server.v1";
    private final SharedPreferences prefs;

    ConfigStore(Context context) {
        prefs = context.getSharedPreferences("server", Context.MODE_PRIVATE);
    }

    ServerConfig load() throws GeneralSecurityException {
        String url = prefs.getString("url", "");
        if (url.isEmpty()) return null;
        try {
            String encrypted = prefs.getString("key", "");
            String key = "";
            if (!encrypted.isEmpty()) {
                Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
                byte[] iv = Base64.decode(prefs.getString("iv", ""), Base64.NO_WRAP);
                cipher.init(Cipher.DECRYPT_MODE, secret(), new GCMParameterSpec(128, iv));
                key = new String(cipher.doFinal(Base64.decode(encrypted, Base64.NO_WRAP)), StandardCharsets.UTF_8);
            }
            return new ServerConfig(url, key, prefs.getBoolean("http", false));
        } catch (IllegalArgumentException e) {
            throw new GeneralSecurityException("Stored configuration is invalid.", e);
        }
    }

    void save(ServerConfig config) throws GeneralSecurityException {
        Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
        cipher.init(Cipher.ENCRYPT_MODE, secret());
        String encrypted = Base64.encodeToString(
                cipher.doFinal(config.apiKey.getBytes(StandardCharsets.UTF_8)), Base64.NO_WRAP);
        prefs.edit().putString("url", config.baseUrl).putBoolean("http", config.allowHttp)
                .putString("key", encrypted)
                .putString("iv", Base64.encodeToString(cipher.getIV(), Base64.NO_WRAP)).apply();
    }

    void clear() {
        prefs.edit().clear().apply();
    }

    private SecretKey secret() throws GeneralSecurityException {
        KeyStore store = KeyStore.getInstance("AndroidKeyStore");
        try {
            store.load(null);
        } catch (java.io.IOException e) {
            throw new GeneralSecurityException(e);
        }
        java.security.Key key = store.getKey(ALIAS, null);
        if (key instanceof SecretKey) return (SecretKey) key;
        KeyGenerator generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore");
        generator.init(new KeyGenParameterSpec.Builder(ALIAS,
                KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build());
        return generator.generateKey();
    }
}
