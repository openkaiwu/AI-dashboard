package cn.local.timezoneswitch;

import java.util.List;
import java.util.Locale;

/** Re-evaluates the visible settings page rather than assuming a click changed screens. */
public final class AutomationPlanner {
    public enum Phase { DATE, PICKER, REGION, CITY }
    public enum Kind { CLICK, TURN_OFF, SEARCH, OPEN_SEARCH, SCROLL_BACKWARD, SCROLL, WAIT }
    public record Item(String text, String id, boolean enabled, boolean actionable,
                       boolean toggle, boolean checked, boolean editable, boolean scrollable) {}
    public record Action(Kind kind, int index, String value, String description) {}
    private Phase phase = Phase.DATE;
    private boolean regionChosen, atTop;
    private String lastQuery = "";
    private int scrolls;
    public AutomationPlanner() {}
    public AutomationPlanner(AutomationPlanner other) {
        phase = other.phase; regionChosen = other.regionChosen; atTop = other.atTop;
        lastQuery = other.lastQuery; scrolls = other.scrolls;
    }
    public Phase phase() { return phase; }
    public void scrollBoundary(boolean backward) {
        if (backward) { atTop = true; scrolls = 0; }
        else scrolls = 65;
    }
    private void observe(Phase value) {
        if (phase != value) { phase = value; lastQuery = ""; scrolls = 0; atTop = false; }
    }
    public Action next(List<Item> items, ZoneTarget target, boolean english) {
        int region = find(items, "区域", "地区", "地區", "區域", "Region");
        int pickerEntry = find(items, "选择时区", "设置时区", "選擇時區", "設定時區", "Select time zone", "Set time zone");
        // The auto-zone row may be offscreen or temporarily missing during a cold launch.
        // An actionable picker entry still identifies the date page after a failed click.
        boolean dateVisible = items.stream().anyMatch(n -> isAutoZone(n.text) || isAutoClock(n.text))
                || (region < 0 && pickerEntry >= 0);
        boolean hasCityOptions = items.stream().anyMatch(n -> !summary(n) && n.enabled && !n.editable
                && (ZoneTarget.LOS_ANGELES.matchesCity(n.text) || ZoneTarget.CHINA.matchesCity(n.text)
                || label(n.text, "檀香山", "埃达克", "安克雷奇", "梅特拉卡特拉", "凤凰城", "丹佛", "芝加哥", "纽约", "New York", "Denver", "Honolulu")));
        if (dateVisible) observe(Phase.DATE);
        else if (region >= 0) observe(Phase.PICKER);
        else if (hasCityOptions) observe(Phase.CITY);
        else if (looksLikeCountries(items)) observe(Phase.REGION);

        if (phase == Phase.DATE) {
            for (int i = 0; i < items.size(); i++) {
                Item n = items.get(i);
                if (isAutoZone(n.text) && n.toggle && n.checked && n.enabled)
                    return action(Kind.TURN_OFF, i, "", "关闭自动设置时区");
            }
            if (items.stream().anyMatch(n -> isAutoZone(n.text) && n.checked))
                return waitAction("等待自动时区开关就绪");
            if (pickerEntry < 0 && items.stream().anyMatch(n -> label(n.text, "选择时区", "设置时区", "選擇時區", "設定時區", "Select time zone", "Set time zone")))
                return waitAction("等待选择时区入口启用");
            int row = find(items, "选择时区", "设置时区", "選擇時區", "設定時區", "Select time zone", "Set time zone", "Time zone", "时区", "時區");
            if (row >= 0) return action(Kind.CLICK, row, "", "打开选择时区页面");
            if (dateVisible) {
                // Only scroll a recognized date page. Never search arbitrary Settings screens.
                if (scrolls < 12) for (int i = 0; i < items.size(); i++) {
                    if (items.get(i).enabled && items.get(i).scrollable) {
                        scrolls++;
                        return action(atTop ? Kind.SCROLL : Kind.SCROLL_BACKWARD, i, "",
                                atTop ? "查找选择时区入口" : "从日期与时间页面顶部查找");
                    }
                }
                return waitAction("等待日期与时间页面加载选择时区入口");
            }
            int date = find(items, "日期和时间", "日期与时间", "日期與時間", "Date & time", "Date and time");
            if (date >= 0) return action(Kind.CLICK, date, "", "进入日期与时间");
            int more = find(items, "更多设置", "其他设置", "其他設定", "Additional settings");
            if (more >= 0) return action(Kind.CLICK, more, "", "进入更多设置");
            return waitAction("未识别到可操作的选择时区行");
        }
        if (phase == Phase.PICKER && region >= 0) {
            boolean selected = selectedCountry(items, target);
            if (!selected && (!regionChosen || selectedDifferentCountry(items, target)))
                return action(Kind.CLICK, region, "", "打开区域列表");
            int zone = find(items, "时区", "時區", "Time zone");
            if (zone >= 0) return action(Kind.CLICK, zone, "", "打开城市时区列表");
            if (target == ZoneTarget.CHINA && selected)
                return waitAction("已选中国大陆，城市行置灰时等待系统应用上海时区");
            return waitAction("等待区域选择结果");
        }
        if (phase == Phase.PICKER && region < 0) observe(Phase.CITY);
        if (phase == Phase.REGION) {
            for (int i = 0; i < items.size(); i++) {
                Item n = items.get(i);
                if (eligible(n) && target.matchesCountry(primary(n.text))) {
                    regionChosen = true;
                    return action(Kind.CLICK, i, "", "选择" + (target == ZoneTarget.CHINA ? "中国大陆" : "美国"));
                }
            }
        } else {
            for (int i = 0; i < items.size(); i++) {
                Item n = items.get(i);
                if (eligible(n) && target.matchesCity(n.text))
                    return action(Kind.CLICK, i, "", "选择" + target.title + "时区");
            }
        }
        String query = phase == Phase.REGION ? (english ? target.countryEn : target.countryZh)
                : english ? (target == ZoneTarget.LOS_ANGELES ? "Los Angeles" : "Shanghai")
                : target == ZoneTarget.LOS_ANGELES ? "洛杉矶" : "上海";
        for (int i = 0; i < items.size(); i++) {
            Item n = items.get(i);
            if (n.editable && n.enabled && !query.equals(lastQuery)) {
                lastQuery = query;
                return action(Kind.SEARCH, i, query, "搜索" + query);
            }
        }
        for (int i = 0; i < items.size(); i++) {
            Item n = items.get(i);
            String id = n.id.toLowerCase(Locale.ROOT);
            if (n.enabled && n.actionable && !n.editable
                    && (label(n.text, "搜索", "搜尋", "Search") || id.endsWith("/search") || id.endsWith("/search_button")))
                return action(Kind.OPEN_SEARCH, i, "", "展开搜索框");
        }
        if (scrolls < 65) for (int i = 0; i < items.size(); i++) if (items.get(i).scrollable) {
            scrolls++;
            return action(atTop ? Kind.SCROLL : Kind.SCROLL_BACKWARD, i, "",
                    atTop ? "查找" + (phase == Phase.REGION ? target.countryZh : target.title) : "从列表顶部开始查找");
        }
        return waitAction("未找到目标控件，等待页面或手动操作");
    }
    /** A screen observation can override this provisional transition on the next tick. */
    public void dispatched(Action action) {
        if (action.description.equals("打开选择时区页面")) observe(Phase.PICKER);
        else if (action.description.equals("打开区域列表")) observe(Phase.REGION);
        else if (action.description.equals("打开城市时区列表")) observe(Phase.CITY);
    }
    private static boolean selectedCountry(List<Item> items, ZoneTarget target) {
        return items.stream().anyMatch(n -> summary(n) && target.matchesCountry(primary(n.text))
                || label(n.text, "区域", "地区", "Region") && java.util.Arrays.stream(n.text.split("[\\n\\r]")).skip(1).anyMatch(t -> target.matchesCountry(t.trim())));
    }
    private static boolean selectedDifferentCountry(List<Item> items, ZoneTarget target) {
        return items.stream().anyMatch(n -> summary(n) && (ZoneTarget.CHINA.matchesCountry(primary(n.text))
                || ZoneTarget.LOS_ANGELES.matchesCountry(primary(n.text))) && !target.matchesCountry(primary(n.text)));
    }
    private static boolean eligible(Item n) { return n.enabled && n.actionable && !n.editable && !n.toggle && !summary(n); }
    private static boolean summary(Item n) { return n.id.endsWith("/summary"); }
    public static String primary(String text) { return text.trim().split("[\\n\\r，,]", 2)[0].trim(); }
    public static boolean label(String text, String... names) {
        for (String name : names) {
            String t = text.trim();
            if (t.equalsIgnoreCase(name)) return true;
            if (t.regionMatches(true, 0, name, 0, name.length()) && t.length() > name.length()
                    && (Character.isWhitespace(t.charAt(name.length())) || "，,：:".indexOf(t.charAt(name.length())) >= 0)) return true;
        }
        return false;
    }
    public static boolean isAutoZone(String text) {
        return label(text, "自动设置时区", "自动确定时区", "自动选择时区", "自动获取时区", "自动时区",
                "使用网络提供的时区", "使用网络时区", "通过网络确定时区", "通过网络自动设置时区",
                "自動設定時區", "自動設置時區", "自動判斷時區", "使用網路提供的時區",
                "Set time zone automatically", "Automatic time zone", "Use network-provided time zone");
    }
    private static boolean isAutoClock(String text) {
        return label(text, "自动设置时间", "自动确定日期和时间", "自动设置日期和时间", "自动日期和时间",
                "自動設定時間", "自動設定日期和時間", "Set time automatically", "Automatic date & time", "Use network-provided time");
    }
    public static boolean knownTapLabel(String text) {
        return isAutoZone(text) || label(text, "选择时区", "设置时区", "区域", "地区", "Region", "Select time zone",
                "Time zone", "时区", "日期和时间", "日期与时间", "更多设置", "Additional settings", "Date & time", "搜索", "Search")
                || ZoneTarget.CHINA.matchesCountry(primary(text)) || ZoneTarget.LOS_ANGELES.matchesCountry(primary(text))
                || ZoneTarget.CHINA.matchesCity(text) || ZoneTarget.LOS_ANGELES.matchesCity(text);
    }
    private static boolean looksLikeCountries(List<Item> items) {
        return items.stream().filter(n -> label(n.text, "中国", "中国大陆", "美国", "英国", "日本", "阿富汗", "阿尔巴尼亚", "阿尔及利亚",
                "China", "United States", "Japan", "Afghanistan", "Albania", "Algeria")).count() >= 2;
    }
    private static int find(List<Item> items, String... names) {
        for (String name : names) for (int i = 0; i < items.size(); i++)
            if (eligible(items.get(i)) && label(items.get(i).text, name)) return i;
        return -1;
    }
    private static Action action(Kind kind, int i, String value, String desc) { return new Action(kind, i, value, desc); }
    private static Action waitAction(String desc) { return action(Kind.WAIT, -1, "", desc); }
}
