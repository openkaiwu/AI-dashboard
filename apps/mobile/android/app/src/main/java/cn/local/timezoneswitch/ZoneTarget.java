package cn.local.timezoneswitch;

import java.time.Instant;
import java.time.ZoneId;
import java.util.Locale;

public enum ZoneTarget {
    LOS_ANGELES("America/Los_Angeles", "洛杉矶", "美国", "United States",
            new String[]{"洛杉矶", "洛杉磯", "Los Angeles", "America/Los_Angeles"}),
    CHINA("Asia/Shanghai", "中国大陆", "中国", "China",
            new String[]{"上海", "北京", "Shanghai", "Beijing", "Asia/Shanghai", "中国标准时间", "China Standard Time"});

    public final String id, title, countryZh, countryEn;
    final String[] cityLabels;
    ZoneTarget(String id, String title, String countryZh, String countryEn, String[] cityLabels) {
        this.id = id; this.title = title; this.countryZh = countryZh; this.countryEn = countryEn;
        this.cityLabels = cityLabels;
    }
    public boolean matchesZone(String zone) {
        if (id.equals(zone)) return true;
        if (this == LOS_ANGELES) return "US/Pacific".equals(zone) || "US/Pacific-New".equals(zone);
        return "PRC".equals(zone) || "Asia/Chongqing".equals(zone)
                || "Asia/Chungking".equals(zone) || "Asia/Harbin".equals(zone);
    }
    public boolean matchesCity(String label) {
        String text = label.trim().toLowerCase(Locale.ROOT);
        for (String name : cityLabels) {
            String n = name.toLowerCase(Locale.ROOT);
            if (text.equals(n) || text.startsWith(n + " ") || text.startsWith(n + "\n") || text.startsWith(n + "\r") || text.startsWith(n + "，")
                    || text.startsWith(n + ",") || text.startsWith(n + "（") || text.startsWith(n + "(")) return true;
        }
        return false;
    }
    public boolean matchesCountry(String text) {
        return countryZh.equals(text) || countryEn.equalsIgnoreCase(text)
                || (this == LOS_ANGELES && ("美國".equals(text) || "美国（美利坚合众国）".equals(text)))
                || (this == CHINA && ("中國".equals(text) || "中国大陆".equals(text)));
    }
    public String offsetAt(Instant now) { return "UTC" + ZoneId.of(id).getRules().getOffset(now).getId(); }
}
