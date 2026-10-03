import pathlib
import sys

id_ = sys.argv[1]
email = sys.argv[2]
remaining = int(sys.argv[3])
hours = int(sys.argv[4])
plan = sys.argv[5]
out = pathlib.Path(sys.argv[6])

title = "多用提醒"
body = "\n".join(
    [
        f"套餐：{plan} · 隐藏五小时额度及提醒",
        f"剩余额度：{remaining}%（提醒线 {remaining}%）",
        f"临近重置：{hours} 小时内",
        "来源：电脑采集器 · 我的电脑",
        "---",
        "模拟器强提醒弹窗测试",
    ]
)
body_sql = body.replace("'", "''")
title_sql = title.replace("'", "''")
sql = f"""SET client_encoding TO 'UTF8';
INSERT INTO notifications (id,user_id,title,body,severity,dedupe_key,status,created_at)
SELECT '{id_}', id, '{title_sql}', '{body_sql}', 'warning', '{id_}', 'unread', now()
FROM users WHERE email = '{email}';
"""
out.write_text(sql, encoding="utf-8")
print(title)
print(body)
