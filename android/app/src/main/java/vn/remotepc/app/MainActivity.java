package vn.remotepc.app;

import android.annotation.SuppressLint;
import android.app.Activity;
import android.app.AlertDialog;
import android.content.Intent;
import android.content.SharedPreferences;
import android.graphics.Color;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.view.View;
import android.view.WindowInsets;
import android.view.WindowInsetsController;
import android.view.WindowManager;
import android.webkit.JavascriptInterface;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;

import com.google.zxing.integration.android.IntentIntegrator;
import com.google.zxing.integration.android.IntentResult;

/**
 * App RemotePC cho Android: màn hình danh sách máy tính (assets/launcher.html) và WebView
 * toàn màn hình mở giao diện điều khiển do RemotePC trên máy tính phục vụ.
 */
public class MainActivity extends Activity {
    private static final String LAUNCHER = "file:///android_asset/launcher.html";
    private WebView web;
    private SharedPreferences prefs;

    @SuppressLint({"SetJavaScriptEnabled", "AddJavascriptInterface"})
    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        prefs = getSharedPreferences("remotepc", MODE_PRIVATE);
        getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);

        web = new WebView(this);
        web.setBackgroundColor(Color.parseColor("#0b1220"));
        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);
        s.setMediaPlaybackRequiresUserGesture(false);
        s.setMixedContentMode(WebSettings.MIXED_CONTENT_ALWAYS_ALLOW);
        s.setUserAgentString(s.getUserAgentString() + " RemotePCApp/" + BuildConfig.VERSION_NAME);
        web.addJavascriptInterface(new Bridge(), "RemotePCApp");
        web.setWebChromeClient(new WebChromeClient());
        web.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest req) {
                return false; // mọi link mở ngay trong app
            }

            @Override
            public void onReceivedError(WebView view, int code, String desc, String url) {
                if (!url.startsWith("file:")) {
                    showLauncher("Không kết nối được tới máy tính. Kiểm tra RemotePC đang chạy và cùng mạng.");
                }
            }
        });
        setContentView(web);
        immersive();

        if (state != null) {
            web.restoreState(state);
        } else {
            showLauncher(null);
        }
    }

    private void showLauncher(String error) {
        String url = LAUNCHER;
        if (error != null) url += "#err=" + Uri.encode(error);
        web.loadUrl(url);
    }

    private void immersive() {
        if (Build.VERSION.SDK_INT >= 30) {
            WindowInsetsController c = getWindow().getInsetsController();
            if (c != null) {
                c.hide(WindowInsets.Type.statusBars() | WindowInsets.Type.navigationBars());
                c.setSystemBarsBehavior(WindowInsetsController.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE);
            }
        } else {
            getWindow().getDecorView().setSystemUiVisibility(View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY
                    | View.SYSTEM_UI_FLAG_FULLSCREEN | View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
                    | View.SYSTEM_UI_FLAG_LAYOUT_STABLE | View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
                    | View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION);
        }
    }

    @Override
    public void onWindowFocusChanged(boolean hasFocus) {
        super.onWindowFocusChanged(hasFocus);
        if (hasFocus) immersive();
    }

    @Override
    protected void onSaveInstanceState(Bundle out) {
        super.onSaveInstanceState(out);
        web.saveState(out);
    }

    @Override
    public void onBackPressed() {
        String url = web.getUrl();
        if (url != null && !url.startsWith("file:")) {
            new AlertDialog.Builder(this)
                    .setMessage("Ngắt kết nối và quay về danh sách máy tính?")
                    .setPositiveButton("Ngắt", (d, w) -> showLauncher(null))
                    .setNegativeButton("Ở lại", null)
                    .show();
        } else {
            super.onBackPressed();
        }
    }

    @Override
    protected void onActivityResult(int req, int res, Intent data) {
        IntentResult r = IntentIntegrator.parseActivityResult(req, res, data);
        if (r != null) {
            String text = r.getContents();
            if (text != null) {
                String js = "window.onScanned && window.onScanned(" + org.json.JSONObject.quote(text) + ")";
                web.evaluateJavascript(js, null);
            }
            return;
        }
        super.onActivityResult(req, res, data);
    }

    /** Hàm cho trang launcher.html gọi qua window.RemotePCApp. */
    private class Bridge {
        @JavascriptInterface
        public String getPcs() {
            return prefs.getString("pcs", "[]");
        }

        @JavascriptInterface
        public void savePcs(String json) {
            prefs.edit().putString("pcs", json).apply();
        }

        @JavascriptInterface
        public String version() {
            return BuildConfig.VERSION_NAME;
        }

        @JavascriptInterface
        public void scan() {
            runOnUiThread(() -> {
                IntentIntegrator ii = new IntentIntegrator(MainActivity.this);
                ii.setDesiredBarcodeFormats(IntentIntegrator.QR_CODE);
                ii.setPrompt("Quét mã QR trên cửa sổ RemotePC của máy tính");
                ii.setBeepEnabled(false);
                ii.setOrientationLocked(false);
                ii.initiateScan();
            });
        }

        @JavascriptInterface
        public void open(String url) {
            runOnUiThread(() -> web.loadUrl(url));
        }
    }
}
