package cn.local.timezoneswitch;

/** One semantic click, one gesture retry, then stop. A reported click alone is not progress. */
public final class InputRetry {
    public enum Decision { NODE, GESTURE, WAIT, STOP }
    private String pendingScreen = "", pendingAction = "";
    private long sentAt;
    private int attempts;
    public Decision decide(String screen, String action, long now) {
        if (!screen.equals(pendingScreen) || !action.equals(pendingAction)) return Decision.NODE;
        // Cold Settings launches and preference refreshes need time to settle.
        if (now - sentAt < (attempts < 2 ? 2200 : 4000)) return Decision.WAIT;
        return attempts < 2 ? Decision.GESTURE : Decision.STOP;
    }
    public void sent(String screen, String action, long now) {
        attempts = screen.equals(pendingScreen) && action.equals(pendingAction) ? attempts + 1 : 1;
        pendingScreen = screen; pendingAction = action; sentAt = now;
    }
}
