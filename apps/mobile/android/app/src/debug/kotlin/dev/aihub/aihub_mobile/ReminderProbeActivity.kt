package dev.aihub.aihub_mobile

import android.app.Activity
import android.os.Bundle
import android.widget.TextView

/** Debug-only window for instrumenting the actual foreground presenter. */
class ReminderProbeActivity : Activity() {
    override fun onCreate(state: Bundle?) {
        super.onCreate(state)
        setContentView(TextView(this).apply {
            text = "AI Hub · 提醒验收\n合成事件，不连接真实账号"
            textSize = 20f
            setPadding(32, 80, 32, 32)
        })
    }
    override fun onResume() { super.onResume(); ForegroundReminders.resume(this) }
    override fun onPause() { ForegroundReminders.pause(this); super.onPause() }
}
