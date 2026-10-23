package dev.aihub.aihub_mobile

import android.app.Activity
import android.app.AlertDialog
import android.app.NotificationManager
import android.os.Handler
import android.os.Looper
import java.lang.ref.WeakReference
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

/** Presents only on the resumed app window, never starts a background activity. */
object ForegroundReminders {
    private var activity = WeakReference<Activity>(null)
    private var dialog: AlertDialog? = null

    fun resume(value: Activity) { activity = WeakReference(value) }
    fun clear() {
        if (Looper.myLooper() != Looper.getMainLooper()) {
            Handler(Looper.getMainLooper()).post { clear() }
            return
        }
        dialog?.dismiss()
        dialog = null
    }
    fun pause(value: Activity) {
        if (activity.get() === value) {
            activity.clear()
            clear()
        }
    }

    // Called on the delivery worker. The UI runnable and timeout share a lock:
    // after fallback is chosen a late runnable cannot also display a dialog.
    fun show(title: String, body: String, stillEligible: () -> Boolean): Boolean {
        check(Looper.myLooper() != Looper.getMainLooper())
        val lock = Any()
        val done = CountDownLatch(1)
        var decided = false
        var shown = false
        Handler(Looper.getMainLooper()).post {
            synchronized(lock) {
                if (!decided) {
                    decided = true
                    val current = activity.get()
                    val filter = current?.getSystemService(NotificationManager::class.java)?.currentInterruptionFilter
                    if (current != null && !current.isFinishing && !current.isDestroyed &&
                        dialog == null && filter == NotificationManager.INTERRUPTION_FILTER_ALL && stillEligible()) {
                        try {
                            val next = AlertDialog.Builder(current)
                                .setTitle(title).setMessage(body)
                                .setPositiveButton("我知道了", null).create()
                            next.setCanceledOnTouchOutside(false)
                            next.setOnDismissListener { if (dialog === next) dialog = null }
                            next.show()
                            dialog = next
                            shown = true
                        } catch (_: android.view.WindowManager.BadTokenException) {
                            // Activity lost its window; caller uses a system notification.
                        }
                    }
                }
                done.countDown()
            }
        }
        try {
            done.await(3, TimeUnit.SECONDS)
        } finally {
            synchronized(lock) { decided = true }
        }
        return synchronized(lock) { shown }
    }
}
