package dev.aihub.aihub_mobile

import android.content.ActivityNotFoundException
import android.content.Intent
import cn.local.timezoneswitch.TimeZoneAccessibilityService
import cn.local.timezoneswitch.ZoneTarget
import android.net.Uri
import android.provider.Settings
import android.os.Build
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel

class MainActivity : FlutterActivity() {
    private fun zoneClock(zone:String):String { val f=java.text.SimpleDateFormat("HH:mm:ss",java.util.Locale.ROOT); f.timeZone=java.util.TimeZone.getTimeZone(zone); return f.format(java.util.Date()) }
    override fun onResume() {
        super.onResume()
        val service=TimeZoneAccessibilityService.current()
        if(service?.running()==true) service.cancel("已返回 AI Hub，操作停止")
        val prefs=getSharedPreferences("state",MODE_PRIVATE)
        if(prefs.getBoolean("running",false)) prefs.edit().putBoolean("running",false).putString("result","上次操作已中断，请核对当前系统时区后重试。").apply()
        BackgroundSync.foreground=true; ForegroundReminders.resume(this); BackgroundSync.ensureScheduled(this); BackgroundSync.checkNow(this)
    }
    override fun onPause() { ForegroundReminders.pause(this); BackgroundSync.foreground=false; super.onPause() }
    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger,"dev.aihub/timezone").setMethodCallHandler { call,result ->
            try {
                val service=TimeZoneAccessibilityService.current()
                when(call.method) {
                    "status" -> { java.util.TimeZone.setDefault(null); val prefs=getSharedPreferences("state",MODE_PRIVATE); result.success(mapOf("zone" to java.util.TimeZone.getDefault().id,"cnClock" to zoneClock("Asia/Shanghai"),"laClock" to zoneClock("America/Los_Angeles"),"auto" to (Settings.Global.getInt(contentResolver,Settings.Global.AUTO_TIME_ZONE,1)!=0),"ready" to (service!=null),"running" to (service?.running() == true),"message" to (prefs.getString("result","") ?: ""),"diagnostic" to (prefs.getString("diagnostic","") ?: ""))) }
                    "settings" -> { val intent=Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS); startActivity(intent); result.success(true) }
                    "switch" -> { if(Build.VERSION.SDK_INT<30) throw IllegalStateException("时区自动切换需要 Android 11 或更高版本"); val target=when(call.argument<String>("target")){"cn"->ZoneTarget.CHINA;"la"->ZoneTarget.LOS_ANGELES;else->throw IllegalArgumentException("不支持的时区")}; if(service==null) throw IllegalStateException("请先开启 AI Hub 时区切换助手"); if(service.running()) throw IllegalStateException("正在切换，请等待完成"); service.start(target); result.success(true) }
                    else -> result.notImplemented()
                }
            } catch(e:Exception){ result.error("timezone_error",e.message,null) }
        }
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger,"dev.aihub/background")
            .setMethodCallHandler { call,result ->
                try {
                    when(call.method) {
                        "check" -> BackgroundSync.checkNow(this){result.success(BackgroundSync.status(this))}
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
