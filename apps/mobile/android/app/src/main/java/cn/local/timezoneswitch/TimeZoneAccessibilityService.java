package cn.local.timezoneswitch;

import android.accessibilityservice.AccessibilityService;
import android.accessibilityservice.AccessibilityServiceInfo;
import android.accessibilityservice.GestureDescription;
import android.app.KeyguardManager;
import android.content.Intent;
import android.graphics.Path;
import android.graphics.Rect;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.os.SystemClock;
import android.provider.Settings;
import android.view.accessibility.AccessibilityEvent;
import android.view.accessibility.AccessibilityNodeInfo;
import android.view.accessibility.AccessibilityWindowInfo;
import android.widget.Toast;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.TimeZone;

public class TimeZoneAccessibilityService extends AccessibilityService {
    private static TimeZoneAccessibilityService instance;
    private final Handler handler = new Handler(Looper.getMainLooper());
    private ZoneTarget target;
    private AutomationPlanner planner;
    private InputRetry retry;
    private long started, lastInputAt, lastScrollAt;
    private boolean sawSettings, gesturePending, lastScrollBackward;
    private String lastScrollScreen = "", step = "", lastTrace = "";
    private final ArrayList<String> trace = new ArrayList<>();
    private final Runnable ticker = this::tick;
    public static TimeZoneAccessibilityService current() { return instance; }
    public boolean running() { return target != null; }
    public ZoneTarget runningTarget() { return target; }
    public boolean gesturesAvailable() {
        AccessibilityServiceInfo info = getServiceInfo();
        return info != null && (info.getCapabilities() & AccessibilityServiceInfo.CAPABILITY_CAN_PERFORM_GESTURES) != 0;
    }
    @Override protected void onServiceConnected() {
        instance = this;
        AccessibilityServiceInfo info = getServiceInfo();
        info.flags |= AccessibilityServiceInfo.FLAG_REPORT_VIEW_IDS
                | AccessibilityServiceInfo.FLAG_RETRIEVE_INTERACTIVE_WINDOWS
                | AccessibilityServiceInfo.FLAG_INCLUDE_NOT_IMPORTANT_VIEWS;
        setServiceInfo(info);
    }
    @Override public void onAccessibilityEvent(AccessibilityEvent event) {
        if (target != null && !handler.hasCallbacks(ticker)) handler.postDelayed(ticker, 350);
    }
    @Override public void onInterrupt() { cancel("系统中断了操作，请重试"); }
    @Override public void onDestroy() {
        cancel("切换助手服务已停止，操作中断");
        handler.removeCallbacksAndMessages(null); instance = null; target = null;
        super.onDestroy();
    }
    public void start(ZoneTarget value) {
        if (running()) return;
        target = value; planner = new AutomationPlanner(); retry = new InputRetry();
        started = SystemClock.elapsedRealtime(); lastInputAt = 0; lastScrollScreen = "";
        sawSettings = false; gesturePending = false; step = "打开日期与时间";
        trace.clear(); lastTrace = "";
        record("v1.0.2 " + Build.MANUFACTURER + " " + Build.MODEL + " Android " + Build.VERSION.RELEASE);
        record("目标=" + value.id + " 手势能力=" + gesturesAvailable());
        saveStatus("正在切换到" + value.title + "，请等待设置页面操作完成");
        try {
            startActivity(new Intent(Settings.ACTION_DATE_SETTINGS).setPackage("com.android.settings").addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
            handler.postDelayed(ticker, 800);
        } catch (RuntimeException error) {
            try {
                startActivity(new Intent(Settings.ACTION_SETTINGS).setPackage("com.android.settings").addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
                handler.postDelayed(ticker, 800);
            } catch (RuntimeException ignored) { finish(false, "无法打开系统设置，请手动进入日期与时间"); }
        }
    }
    public void cancel(String reason) {
        if (target == null) return;
        target = null; handler.removeCallbacks(ticker);
        record(reason); saveStatus(reason + "。已经完成的设置操作会保留。");
    }
    private void tick() {
        if (target == null) return;
        try { inspect(); }
        catch (RuntimeException error) {
            record("异常=" + error.getClass().getSimpleName());
            finish(false, "无法操作当前设置页面，请回 App 点「复制诊断信息」反馈");
        }
        if (target != null && !handler.hasCallbacks(ticker)) handler.postDelayed(ticker, 600);
    }
    private void inspect() {
        long now = SystemClock.elapsedRealtime();
        if (((KeyguardManager)getSystemService(KEYGUARD_SERVICE)).isKeyguardLocked()) {
            cancel("手机已锁屏，操作停止"); return;
        }
        TimeZone.setDefault(null);
        boolean manual = Settings.Global.getInt(getContentResolver(), Settings.Global.AUTO_TIME_ZONE, 1) == 0;
        if (target.matchesZone(TimeZone.getDefault().getID()) && manual) {
            finish(true, "已切换到" + target.title + "，系统时区已核验"); return;
        }
        if (now - started > 60_000) {
            finish(false, "未能自动完成：" + step + "。请返回 App 点「复制诊断信息」反馈。已完成的设置操作会保留。"); return;
        }
        if (gesturePending || now - lastInputAt < 900) return;
        AccessibilityNodeInfo root = getRootInActiveWindow();
        if (root == null) {
            for (AccessibilityWindowInfo window : getWindows()) if (window.isActive()) {
                root = window.getRoot(); if (root != null) break;
            }
        }
        if (root == null) { record("无法读取活动窗口，请检查无障碍服务是否允许读取屏幕内容"); return; }
        if (!root.refresh()) { record("系统页面正在更新，等待控件就绪"); return; }
        String pkg = packageOf(root, 0);
        if (!"com.android.settings".equals(pkg)) {
            if (sawSettings && !"com.android.systemui".equals(pkg) && !"android".equals(pkg)) cancel("已离开设置，操作停止");
            else record("活动窗口尚不是系统设置，等待系统页面");
            return;
        }
        sawSettings = true;
        List<AccessibilityNodeInfo> nodes = new ArrayList<>();
        List<AutomationPlanner.Item> items = new ArrayList<>();
        flatten(root, nodes, items, 0, !manual);
        String screen = items.toString();
        if (screen.equals(lastScrollScreen)) {
            if (now - lastScrollAt < 1400) return;
            planner.scrollBoundary(lastScrollBackward); lastScrollScreen = "";
        } else lastScrollScreen = "";
        AutomationPlanner candidate = new AutomationPlanner(planner);
        AutomationPlanner.Action action = candidate.next(items, target, Locale.getDefault().getLanguage().equals("en"));
        if (!step.equals(action.description())) {
            step = action.description();
            saveStatus("正在切换到" + target.title + "：" + step + "。请等待自动返回。");
        }
        if (action.kind() == AutomationPlanner.Kind.WAIT) {
            planner = candidate;
            String labels = items.stream().filter(n -> AutomationPlanner.knownTapLabel(n.text())).limit(10)
                    .map(n -> AutomationPlanner.primary(n.text()) + (n.enabled() ? "" : "[置灰]"))
                    .collect(java.util.stream.Collectors.joining(" / "));
            record(step + "；识别节点=" + items.size() + "；相关文字=" + labels);
            return;
        }
        AccessibilityNodeInfo node = nodes.get(action.index());
        if (action.kind() == AutomationPlanner.Kind.SCROLL || action.kind() == AutomationPlanner.Kind.SCROLL_BACKWARD) {
            boolean backward = action.kind() == AutomationPlanner.Kind.SCROLL_BACKWARD;
            boolean accepted = node.performAction(backward ? AccessibilityNodeInfo.ACTION_SCROLL_BACKWARD : AccessibilityNodeInfo.ACTION_SCROLL_FORWARD);
            if (!accepted) accepted = swipe(node, backward);
            if (accepted) { lastScrollScreen = screen; lastScrollAt = now; lastScrollBackward = backward; lastInputAt = now; }
            else candidate.scrollBoundary(backward);
            planner = candidate; record(step + " 滚动=" + accepted); return;
        }
        if (action.kind() == AutomationPlanner.Kind.SEARCH) {
            Bundle args = new Bundle();
            args.putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, action.value());
            boolean accepted = node.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, args);
            if (accepted) { planner = candidate; lastInputAt = now; }
            record(step + " 输入=" + accepted); return;
        }
        String actionKey = action.kind() + ":" + action.description();
        // A ticking clock or changing date summary is not navigation progress.
        String inputScreen = candidate.phase().name();
        InputRetry.Decision decision = retry.decide(inputScreen, actionKey, now);
        if (decision == InputRetry.Decision.WAIT) return;
        if (decision == InputRetry.Decision.STOP) {
            finish(false, "页面未响应：" + step + "。请返回 App 点「复制诊断信息」。若刚覆盖安装，请先关闭并重新开启切换助手。"); return;
        }
        AccessibilityNodeInfo clickable = node;
        if (action.kind() == AutomationPlanner.Kind.TURN_OFF) {
            AccessibilityNodeInfo toggle = associatedToggle(node);
            if (toggle != null) { if (!toggle.isChecked()) return; clickable = toggle; }
            else if (Settings.Global.getInt(getContentResolver(), Settings.Global.AUTO_TIME_ZONE, 1) == 0) return;
        }
        boolean accepted = false;
        if (decision == InputRetry.Decision.NODE) accepted = clickNode(clickable);
        String method = "控件点击";
        if (!accepted || decision == InputRetry.Decision.GESTURE) {
            AccessibilityNodeInfo row = clickableParent(clickable);
            accepted = tap(row != null ? row : clickable); method = "手势点击";
        }
        record(step + " " + method + "=" + accepted);
        // Even a rejected input consumes a retry, so a broken page never causes endless tapping.
        retry.sent(inputScreen, actionKey, now); lastInputAt = now;
        if (accepted) { candidate.dispatched(action); planner = candidate; }
    }
    private String packageOf(AccessibilityNodeInfo node, int depth) {
        if (node.getPackageName() != null) return node.getPackageName().toString();
        if (depth >= 3) return "";
        for (int i = 0; i < node.getChildCount(); i++) {
            AccessibilityNodeInfo child = node.getChild(i);
            if (child != null) { String pkg = packageOf(child, depth + 1); if (!pkg.isEmpty()) return pkg; }
        }
        return "";
    }
    private void flatten(AccessibilityNodeInfo node, List<AccessibilityNodeInfo> nodes,
                         List<AutomationPlanner.Item> items, int depth, boolean autoZoneOn) {
        if (depth > 45 || nodes.size() > 1200) return;
        if (node.isVisibleToUser()) {
            String text = node.getText() == null ? "" : node.getText().toString().trim();
            if (text.isEmpty() && node.getContentDescription() != null) text = node.getContentDescription().toString().trim();
            boolean autoZoneLabel = AutomationPlanner.isAutoZone(text);
            AccessibilityNodeInfo toggle = autoZoneLabel ? associatedToggle(node) : null;
            boolean actionable = clickableParent(node) != null || safeTapLabel(node, text);
            boolean scrollable = node.isScrollable() || hasAction(node, AccessibilityNodeInfo.ACTION_SCROLL_FORWARD)
                    || hasAction(node, AccessibilityNodeInfo.ACTION_SCROLL_BACKWARD);
            nodes.add(node);
            items.add(new AutomationPlanner.Item(text, node.getViewIdResourceName() == null ? "" : node.getViewIdResourceName(),
                    node.isEnabled() && !node.isHeading(), actionable, toggle != null || autoZoneLabel,
                    autoZoneLabel ? autoZoneOn : toggle != null && toggle.isChecked(), node.isEditable(), scrollable));
        }
        for (int i = 0; i < node.getChildCount(); i++) {
            AccessibilityNodeInfo child = node.getChild(i);
            if (child != null) flatten(child, nodes, items, depth + 1, autoZoneOn);
        }
    }
    private boolean safeTapLabel(AccessibilityNodeInfo node, String text) {
        if (!node.isEnabled() || node.isHeading() || !AutomationPlanner.knownTapLabel(text)) return false;
        Rect rect = new Rect(); node.getBoundsInScreen(rect);
        if (rect.isEmpty()) return false;
        // Bare 时区 is also a section heading. It needs a real click action or a sibling city summary.
        if (text.equals("时区") || text.equalsIgnoreCase("Time zone")
                || AutomationPlanner.label(text, "区域", "地区", "Region")) {
            if (AutomationPlanner.label(text, "区域", "地区", "Region")
                    && (text.contains("中国大陆") || text.contains("美国") || text.contains("China") || text.contains("United States"))) return true;
            AccessibilityNodeInfo parent = node.getParent();
            for (int level = 0; parent != null && level < 3; level++, parent = parent.getParent()) {
                if (parent.isScrollable() || parent.getChildCount() > 6) break;
                for (int i = 0; i < parent.getChildCount(); i++) {
                    AccessibilityNodeInfo child = parent.getChild(i);
                    if (child != null && child.getText() != null) {
                        String value = child.getText().toString();
                        if (value.contains("GMT") || ZoneTarget.CHINA.matchesCountry(value) || ZoneTarget.LOS_ANGELES.matchesCountry(value)) return true;
                    }
                }
            }
            return false;
        }
        return true;
    }
    private AccessibilityNodeInfo associatedToggle(AccessibilityNodeInfo label) {
        AccessibilityNodeInfo parent = label;
        for (int i = 0; parent != null && i < 7; i++, parent = parent.getParent()) {
            if (parent.isScrollable() || parent.getChildCount() > 6) break;
            ArrayList<AccessibilityNodeInfo> found = new ArrayList<>(); collectToggles(parent, found, 0);
            if (found.size() == 1) return found.get(0);
            if (found.size() > 1) break; // Never associate another row's date/time switch.
        }
        return null;
    }
    private void collectToggles(AccessibilityNodeInfo node, List<AccessibilityNodeInfo> found, int depth) {
        if (node.isCheckable() && node.isVisibleToUser() && node.isEnabled()) { found.add(node); return; }
        if (depth >= 5 || node.isScrollable()) return;
        for (int i = 0; i < node.getChildCount(); i++) {
            AccessibilityNodeInfo child = node.getChild(i); if (child != null) collectToggles(child, found, depth + 1);
        }
    }
    private boolean hasAction(AccessibilityNodeInfo node, int id) {
        for (AccessibilityNodeInfo.AccessibilityAction action : node.getActionList()) if (action.getId() == id) return true;
        return false;
    }
    private AccessibilityNodeInfo clickableParent(AccessibilityNodeInfo node) {
        Rect labelBounds = new Rect(); node.getBoundsInScreen(labelBounds);
        AccessibilityNodeInfo current = node;
        for (int i = 0; current != null && i < 9; i++, current = current.getParent()) {
            if (!current.isEnabled() || current.isScrollable()) return null;
            Rect bounds = new Rect(); current.getBoundsInScreen(bounds);
            if (i > 0 && bounds.height() > Math.max(labelBounds.height() * 6, getResources().getDisplayMetrics().heightPixels / 3)) return null;
            if (current.isClickable() || hasAction(current, AccessibilityNodeInfo.ACTION_CLICK)) return current;
        }
        return null;
    }
    private boolean clickNode(AccessibilityNodeInfo node) {
        AccessibilityNodeInfo clickable = clickableParent(node);
        return clickable != null && clickable.performAction(AccessibilityNodeInfo.ACTION_CLICK);
    }
    private boolean tap(AccessibilityNodeInfo node) {
        if (!gesturesAvailable()) { record("缺少手势能力：请关闭并重新开启无障碍服务"); return false; }
        Rect rect = new Rect(); node.getBoundsInScreen(rect);
        if (rect.isEmpty() || !node.isEnabled() || !node.isVisibleToUser()) return false;
        Path path = new Path(); path.moveTo(rect.exactCenterX(), rect.exactCenterY());
        return gesture(path, 100);
    }
    private boolean swipe(AccessibilityNodeInfo node, boolean backward) {
        // Called only after the planner identifies a date page or a country/city list.
        if (!gesturesAvailable()) return false;
        Rect rect = new Rect(); node.getBoundsInScreen(rect); if (rect.height() < 180) return false;
        float x = rect.exactCenterX(), top = rect.top + rect.height() * .3f, bottom = rect.top + rect.height() * .8f;
        Path path = new Path(); path.moveTo(x, backward ? top : bottom); path.lineTo(x, backward ? bottom : top);
        return gesture(path, 320);
    }
    private boolean gesture(Path path, long duration) {
        AccessibilityNodeInfo active = getRootInActiveWindow();
        if (active == null || !"com.android.settings".equals(packageOf(active, 0))) return false;
        GestureDescription input = new GestureDescription.Builder().addStroke(new GestureDescription.StrokeDescription(path, 0, duration)).build();
        gesturePending = true;
        boolean accepted = dispatchGesture(input, new GestureResultCallback() {
            @Override public void onCompleted(GestureDescription gesture) { gesturePending = false; }
            @Override public void onCancelled(GestureDescription gesture) { gesturePending = false; record("系统取消手势输入"); }
        }, handler);
        if (!accepted) gesturePending = false;
        return accepted;
    }
    private void finish(boolean success, String message) {
        target = null; handler.removeCallbacks(ticker); record(message); saveStatus(message);
        Toast.makeText(this, message, Toast.LENGTH_LONG).show();
        if (success) try { startActivity(new Intent(this, dev.aihub.aihub_mobile.MainActivity.class)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TOP | Intent.FLAG_ACTIVITY_SINGLE_TOP)); }
        catch (RuntimeException ignored) { /* Result is available in the app. */ }
    }
    private void record(String line) {
        if (line.equals(lastTrace)) return;
        lastTrace = line;
        trace.add(Math.max(0, SystemClock.elapsedRealtime() - started) / 1000 + "s " + line);
        while (trace.size() > 35) trace.remove(0);
        getSharedPreferences("state", MODE_PRIVATE).edit().putString("diagnostic", String.join("\n", trace)).apply();
    }
    private void saveStatus(String text) {
        getSharedPreferences("state", MODE_PRIVATE).edit().putString("result", text).putBoolean("running", target != null).apply();
    }
}
