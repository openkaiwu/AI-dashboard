package dev.aihub.aihub_mobile

import android.content.ActivityNotFoundException
import android.content.Intent
import android.net.Uri
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel

class MainActivity : FlutterActivity() {
    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
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
