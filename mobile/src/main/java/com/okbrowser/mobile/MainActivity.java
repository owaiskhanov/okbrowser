package com.okbrowser.mobile;

import android.app.Activity;
import android.content.Intent;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.net.Uri;
import android.os.Bundle;
import android.view.Gravity;
import android.view.KeyEvent;
import android.view.View;
import android.view.Window;
import android.view.WindowInsets;
import android.view.WindowInsetsController;
import android.webkit.DownloadListener;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.ProgressBar;
import android.widget.TextView;

import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;

public class MainActivity extends Activity {
    private WebView web;
    private EditText address;
    private ProgressBar progress;
    private LinearLayout glass;

    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        Window w = getWindow();
        w.setStatusBarColor(Color.TRANSPARENT);
        w.setNavigationBarColor(Color.TRANSPARENT);
        hideSystemBars();

        FrameLayout root = new FrameLayout(this);
        root.setBackgroundColor(Color.rgb(20, 20, 24));
        web = new WebView(this);
        root.addView(web, new FrameLayout.LayoutParams(-1, -1));

        progress = new ProgressBar(this, null, android.R.attr.progressBarStyleHorizontal);
        progress.setMax(100);
        progress.setProgressTintList(android.content.res.ColorStateList.valueOf(Color.rgb(10,132,255)));
        FrameLayout.LayoutParams pp = new FrameLayout.LayoutParams(-1, dp(2));
        pp.topMargin = 0;
        root.addView(progress, pp);

        glass = new LinearLayout(this);
        glass.setGravity(Gravity.CENTER_VERTICAL);
        glass.setPadding(dp(7), dp(5), dp(7), dp(5));
        GradientDrawable bg = new GradientDrawable(GradientDrawable.Orientation.TOP_BOTTOM,
                new int[]{0xEAF8F8FA, 0xDDE8E8ED});
        bg.setCornerRadius(dp(26));
        bg.setStroke(dp(1), 0x88FFFFFF);
        glass.setBackground(bg);
        glass.setElevation(dp(16));

        glass.addView(button("‹", v -> { if (web.canGoBack()) web.goBack(); }));
        address = new EditText(this);
        address.setSingleLine(true);
        address.setHint("Search or enter address");
        address.setTextColor(0xFF17171B);
        address.setHintTextColor(0xFF74747C);
        address.setTextSize(15);
        address.setBackgroundColor(Color.TRANSPARENT);
        address.setPadding(dp(7), 0, dp(7), 0);
        address.setSelectAllOnFocus(true);
        address.setImeOptions(android.view.inputmethod.EditorInfo.IME_ACTION_GO);
        address.setOnEditorActionListener((v, id, e) -> { navigate(address.getText().toString()); address.clearFocus(); return true; });
        glass.addView(address, new LinearLayout.LayoutParams(0, dp(42), 1));
        glass.addView(button("↻", v -> web.reload()));
        glass.addView(button("⋮", this::showMenu));

        FrameLayout.LayoutParams gp = new FrameLayout.LayoutParams(-1, dp(54));
        gp.gravity = Gravity.TOP;
        gp.setMargins(dp(10), dp(9), dp(10), 0);
        root.addView(glass, gp);
        setContentView(root);
        configureWebView();

        Intent intent = getIntent();
        Uri data = intent == null ? null : intent.getData();
        if (state != null) web.restoreState(state);
        else if (data != null) web.loadUrl(data.toString());
        else showHome();
    }

    private TextView button(String text, View.OnClickListener click) {
        TextView b = new TextView(this);
        b.setText(text); b.setTextColor(0xFF242429); b.setTextSize(23); b.setGravity(Gravity.CENTER);
        b.setTypeface(Typeface.DEFAULT, Typeface.NORMAL); b.setOnClickListener(click);
        b.setBackground(ripple());
        b.setContentDescription(text);
        b.setLayoutParams(new LinearLayout.LayoutParams(dp(40), dp(40)));
        return b;
    }

    private android.graphics.drawable.RippleDrawable ripple() {
        GradientDrawable mask = new GradientDrawable(); mask.setColor(Color.WHITE); mask.setShape(GradientDrawable.OVAL);
        return new android.graphics.drawable.RippleDrawable(android.content.res.ColorStateList.valueOf(0x25787880), null, mask);
    }

    private void configureWebView() {
        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true); s.setDomStorageEnabled(true); s.setDatabaseEnabled(true);
        s.setSupportZoom(true); s.setBuiltInZoomControls(true); s.setDisplayZoomControls(false);
        s.setLoadWithOverviewMode(true); s.setUseWideViewPort(true); s.setMediaPlaybackRequiresUserGesture(false);
        s.setSupportMultipleWindows(false); s.setUserAgentString(s.getUserAgentString() + " OKBrowserMobile/1.0");
        web.setWebViewClient(new WebViewClient() {
            @Override public boolean shouldOverrideUrlLoading(WebView v, WebResourceRequest r) {
                Uri u = r.getUrl(); String scheme = u.getScheme();
                if ("http".equals(scheme) || "https".equals(scheme)) return false;
                try { startActivity(new Intent(Intent.ACTION_VIEW, u)); } catch (Exception ignored) {}
                return true;
            }
            @Override public void onPageFinished(WebView v, String url) { address.setText(url.startsWith("data:") ? "" : url); }
        });
        web.setWebChromeClient(new WebChromeClient() {
            @Override public void onProgressChanged(WebView v, int p) {
                progress.setProgress(p); progress.setVisibility(p >= 100 ? View.INVISIBLE : View.VISIBLE);
            }
            @Override public void onReceivedTitle(WebView v, String title) { setTitle(title == null ? "OK Browser" : title); }
        });
        web.setDownloadListener((url, ua, disposition, mime, length) -> {
            try { startActivity(new Intent(Intent.ACTION_VIEW, Uri.parse(url))); } catch (Exception ignored) {}
        });
    }

    private void showMenu(View anchor) {
        android.widget.PopupMenu menu = new android.widget.PopupMenu(this, anchor);
        menu.getMenu().add("New tab"); menu.getMenu().add("Share"); menu.getMenu().add("Open in another app");
        menu.setOnMenuItemClickListener(item -> {
            String t = item.getTitle().toString();
            if (t.equals("New tab")) showHome();
            else if (t.equals("Share")) startActivity(Intent.createChooser(new Intent(Intent.ACTION_SEND).setType("text/plain").putExtra(Intent.EXTRA_TEXT, web.getUrl()), "Share page"));
            else try { startActivity(new Intent(Intent.ACTION_VIEW, Uri.parse(web.getUrl()))); } catch (Exception ignored) {}
            return true;
        }); menu.show();
    }

    private void showHome() {
        String html = "<html><meta name='viewport' content='width=device-width,initial-scale=1'><style>" +
                "*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;background:radial-gradient(circle at 30% 20%,#253a65,#111218 55%,#09090c);color:white;font-family:system-ui;text-align:center}" +
                ".o{padding:32px}.logo{width:78px;height:78px;border-radius:24px;margin:auto;background:linear-gradient(145deg,#54b6ff,#0865df);box-shadow:0 20px 60px #006eff66;display:grid;place-items:center;font-size:32px;font-weight:800}.t{font-size:27px;font-weight:700;margin-top:22px}.s{color:#aaaab4;margin-top:7px}</style>" +
                "<body><div class=o><div class=logo>OK</div><div class=t>Browse beautifully.</div><div class=s>Tap the address bar to begin</div></div></body></html>";
        web.loadDataWithBaseURL("https://okbrowser.local/", html, "text/html", "UTF-8", null);
        address.setText(""); address.requestFocus();
    }

    private void navigate(String raw) {
        String q = raw.trim(); if (q.isEmpty()) return;
        if (q.matches("^[a-zA-Z][a-zA-Z0-9+.-]*://.*")) web.loadUrl(q);
        else if (q.contains(".") && !q.contains(" ")) web.loadUrl("https://" + q);
        else web.loadUrl("https://www.google.com/search?q=" + URLEncoder.encode(q, StandardCharsets.UTF_8));
    }

    /**
     * Keep both Android system bars hidden. A deliberate edge swipe reveals
     * transient controls, and Android automatically hides them again.
     */
    private void hideSystemBars() {
        Window w = getWindow();
        if (android.os.Build.VERSION.SDK_INT >= 30) {
            w.setDecorFitsSystemWindows(false);
            WindowInsetsController controller = w.getInsetsController();
            if (controller != null) {
                controller.hide(WindowInsets.Type.statusBars() | WindowInsets.Type.navigationBars());
                controller.setSystemBarsBehavior(
                        WindowInsetsController.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE);
            }
        } else {
            w.getDecorView().setSystemUiVisibility(
                    View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY |
                    View.SYSTEM_UI_FLAG_FULLSCREEN |
                    View.SYSTEM_UI_FLAG_HIDE_NAVIGATION |
                    View.SYSTEM_UI_FLAG_LAYOUT_STABLE |
                    View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN |
                    View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION);
        }
    }

    @Override protected void onResume() { super.onResume(); hideSystemBars(); }
    @Override public void onWindowFocusChanged(boolean focused) {
        super.onWindowFocusChanged(focused);
        if (focused) hideSystemBars();
    }
    @Override public void onBackPressed() { if (web.canGoBack()) web.goBack(); else super.onBackPressed(); }
    @Override protected void onSaveInstanceState(Bundle out) { web.saveState(out); super.onSaveInstanceState(out); }
    @Override protected void onDestroy() { web.destroy(); super.onDestroy(); }
    private int dp(int n) { return Math.round(n * getResources().getDisplayMetrics().density); }
}
