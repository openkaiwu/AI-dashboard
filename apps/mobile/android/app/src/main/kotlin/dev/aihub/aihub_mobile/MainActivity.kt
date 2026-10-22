package dev.aihub.aihub_mobile

import android.content.ActivityNotFoundException
import android.content.Intent
import android.net.Uri
import android.provider.Settings
import android.os.Build
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel

class MainActivity : FlutterActivity() {
    override fun onResume() { super.onResume(); BackgroundSync.foreground=true; BackgroundSync.ensureScheduled(this); BackgroundSync.checkNow(this) }
    override fun onPause() { BackgroundSync.foreground=false; super.onPause() }
    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger,"dev.aihub/background")
            .setMethodCallHandler { call,result ->
                try {
                    when(call.method) {
                        "check" -> {BackgroundSync.checkNow(this);result.success(true)}
                        "status" -> result.success(BackgroundSync.status(this))
                        "configure" -> {BackgroundSync.configure(this,call.argument<String>("server")!!,call.argument<String>("scope")!!,call.argument<String>("token")!!,call.argument<String>("expires_at")!!);result.success(BackgroundSync.status(this))}
                        "disable" -> {BackgroundSync.disable(this);result.success(true)}
                        "bind" -> {BackgroundSync.bind(this,call.argument<String>("scope")!!);result.success(true)}
                        "seen" -> result.success(BackgroundSync.seen(this,call.argument<String>("scope")!!,call.argument<String>("id")!!,call.argument<String>("token")!!))
                        "mark" -> {BackgroundSync.mark(this,call.argument<String>("scope")!!,call.argument<String>("id")!!,call.argument<String>("token")!!);result.success(true)}
                        "cache" -> result.success(BackgroundSync.cached(this,call.argument<String>("scope")!!,call.argument<String>("kind")!!))
                        "notificationSettings" -> {
                            val intent=if(Build.VERSION.SDK_INT>=26)Intent(Settings.ACTION_CHANNEL_NOTIFICATION_SETTINGS).putExtra(Settings.EXTRA_APP_PACKAGE,packageName).putExtra(Settings.EXTRA_CHANNEL_ID,BackgroundSync.CHANNEL) else Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS,Uri.parse("package:$packageName"))
                            startActivity(intent);result.success(true)
                        }
                        "batterySettings" -> {startActivity(Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS,Uri.parse("package:$packageName")));result.success(true)}
                        else -> result.notImplemented()
                    }
                } catch(e:Exception) {result.error("background_error",e.message,null)}
            }
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, "dev.aihub/mobile_links")
            .setMethodCallHandler { call, result ->
                if (call.method != "openOriginal") {
                    result.notImplemented()
                    return@setMethodCallHandler
                }
                val raw = call.argument<String>("url") ?: ""
                val uri = Uri.parse(raw)
                if (uri.scheme != "https" || uri.host != "x.com" ||
                    uri.userInfo != null || uri.query != null || uri.fragment != null ||
                    !Regex("^/thsottiaux/status/[0-9]+$").matches(uri.path ?: "")) {
                    result.error("invalid_url", "Unsupported source link", null)
                    return@setMethodCallHandler
                }
                try {
                    startActivity(Intent(Intent.ACTION_VIEW, uri).addCategory(Intent.CATEGORY_BROWSABLE))
                    result.success(true)
                } catch (_: ActivityNotFoundException) {
                    result.error("no_browser", "No browser can open this link", null)
                }
            }
    }
}
