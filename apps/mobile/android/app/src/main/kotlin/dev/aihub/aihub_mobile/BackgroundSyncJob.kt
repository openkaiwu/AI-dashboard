package dev.aihub.aihub_mobile

import android.app.*
import android.app.job.*
import android.content.*
import android.os.Build
import android.os.PowerManager
import android.os.Handler
import android.os.Looper
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import org.json.JSONObject
import org.json.JSONArray
import java.io.File
import java.net.HttpURLConnection
import java.net.URI
import java.net.URL
import java.net.URLEncoder
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import java.time.Instant
import java.time.OffsetDateTime
import java.util.concurrent.Executors
import java.util.concurrent.Future
import java.util.concurrent.atomic.AtomicBoolean

object BackgroundSync {
    const val JOB = 11501
    const val CHANNEL = "aihub_popup_reminders_v2"
    @Volatile var foreground = false
    private fun prefs(c: Context) = c.getSharedPreferences("background-sync", Context.MODE_PRIVATE)
    private fun key(): SecretKey {
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (store.getKey("aihub-background", null) as? SecretKey)?.let { return it }
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore").apply {
            init(KeyGenParameterSpec.Builder("aihub-background", KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build())
        }.generateKey()
    }
    private fun encrypt(token: String): String {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding").apply { init(Cipher.ENCRYPT_MODE, key()) }
        return Base64.encodeToString(cipher.iv + cipher.doFinal(token.toByteArray()), Base64.NO_WRAP)
    }
    private fun token(c: Context): String {
        val raw = Base64.decode(prefs(c).getString("credential", ""), Base64.NO_WRAP)
        return String(Cipher.getInstance("AES/GCM/NoPadding").apply { init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, raw.copyOfRange(0,12))) }.doFinal(raw.copyOfRange(12,raw.size)))
    }
    fun configure(c: Context, server: String, scope: String, token: String, expiry: String) {
        val uri = URI(server)
        require(uri.scheme == "https" && uri.host != null && uri.rawUserInfo == null && uri.rawQuery == null && uri.rawFragment == null && (uri.path.isNullOrEmpty() || uri.path == "/"))
        require(token.length in 32..256 && scope.length in 1..1024)
        if(prefs(c).getString("scope","")!=scope){c.getSystemService(NotificationManager::class.java).cancelAll();ForegroundReminders.clear()}
        check(prefs(c).edit().putString("server",server.trimEnd('/')).putString("scope",scope).putString("credential",encrypt(token)).putString("expires_at",expiry).putBoolean("enabled",true).remove("error").remove("auth_paused").commit())
        ensureScheduled(c)
        checkNow(c)
    }
    fun ensureScheduled(c:Context) {
        if(!prefs(c).getBoolean("enabled",false) || prefs(c).getBoolean("auth_paused",false))return
        val scheduler=c.getSystemService(JobScheduler::class.java)
        if(scheduler.getPendingJob(JOB)==null) {
            val job=JobInfo.Builder(JOB,ComponentName(c,BackgroundSyncJob::class.java)).setPeriodic(15*60*1000L).setPersisted(true).setRequiredNetworkType(JobInfo.NETWORK_TYPE_ANY).build()
            check(scheduler.schedule(job)==JobScheduler.RESULT_SUCCESS)
        }
    }
    private val checks=Executors.newSingleThreadExecutor()
    private val checking=AtomicBoolean(false)
    // Manual checks join an in-flight check and observe its completed result.
    private val completionLock=Any()
    private val completions=mutableListOf<() -> Unit>()
    fun checkNow(c:Context, completed:(() -> Unit)?=null) {
        synchronized(completionLock) {
            if(completed!=null)completions.add(completed)
            if(!checking.compareAndSet(false,true))return
        }
        val app=c.applicationContext
        checks.submit {
            try{run(app,!foreground)}catch(e:Exception){recordError(app,e)}finally{
                val callbacks=synchronized(completionLock){checking.set(false);completions.toList().also{completions.clear()}}
                Handler(Looper.getMainLooper()).post{callbacks.forEach{it()}}
            }
        }
    }
    fun disable(c: Context) { c.getSystemService(JobScheduler::class.java).cancel(JOB);c.getSystemService(NotificationManager::class.java).cancelAll();prefs(c).edit().putBoolean("enabled",false).remove("credential").commit();ForegroundReminders.clear() }
    fun bind(c: Context, scope: String) { if(prefs(c).getString("scope","") != scope) disable(c) else ensureScheduled(c) }
    private fun channel(c:Context):NotificationChannel? {
        val manager=c.getSystemService(NotificationManager::class.java)
        if(Build.VERSION.SDK_INT>=26) {
            manager.createNotificationChannel(NotificationChannel(CHANNEL,"额度与到期提醒",NotificationManager.IMPORTANCE_HIGH).apply{description="仅首次出现的新风险，已提醒事件不会重发";enableVibration(true)})
            return manager.getNotificationChannel(CHANNEL)
        };return null
    }
    fun status(c: Context): Map<String, Any?> {
        val manager=c.getSystemService(NotificationManager::class.java)
        val channel=channel(c)
        val enabled=manager.areNotificationsEnabled()
        val dnd=when(manager.currentInterruptionFilter) {
            NotificationManager.INTERRUPTION_FILTER_NONE,NotificationManager.INTERRUPTION_FILTER_ALARMS -> true
            NotificationManager.INTERRUPTION_FILTER_PRIORITY -> channel?.canBypassDnd()!=true
            else -> false
        }
        val condition=if(!enabled)"permission_denied" else if(Build.VERSION.SDK_INT>=26&&channel==null)"channel_missing" else if(channel!=null&&channel.importance==NotificationManager.IMPORTANCE_NONE)"channel_blocked" else if(channel!=null&&channel.importance<NotificationManager.IMPORTANCE_HIGH)"tray_only" else if(dnd)"system_dnd" else "heads_up_conditions"
        return mapOf("enabled" to prefs(c).getBoolean("enabled",false),"scope" to prefs(c).getString("scope",""),"expires_at" to prefs(c).getString("expires_at",null),"last_sync" to prefs(c).getString("last_sync",null),"last_run" to prefs(c).getString("last_run",null),"source_observed_at" to prefs(c).getString("source_observed_at",null),"last_notification" to prefs(c).getString("last_notification",null),"error" to prefs(c).getString("error",null),"analysis_error" to prefs(c).getString("analysis_error",null),"last_reason" to prefs(c).getString("last_reason",null),"primary" to prefs(c).getBoolean("primary",false),"notifications" to enabled,"notification_state" to condition,"heads_up" to (condition=="heads_up_conditions"),"battery_unrestricted" to c.getSystemService(PowerManager::class.java).isIgnoringBatteryOptimizations(c.packageName),"scheduled" to (c.getSystemService(JobScheduler::class.java).getPendingJob(JOB)!=null),"diagnostics" to prefs(c).getString("diagnostics","[]"))
    }
    private fun diagnostic(c:Context,reason:String,event:String="") {
        val now=System.currentTimeMillis();val history=JSONArray(prefs(c).getString("diagnostics","[]"));val next=JSONArray()
        for(i in 0 until history.length()){val row=history.getJSONObject(i);if(now-row.optLong("at")<7*86400000L && i>=history.length()-199)next.put(row)}
        next.put(JSONObject().put("at",now).put("reason",reason).put("event",event))
        val editor=prefs(c).edit().putString("diagnostics",next.toString())
        if(reason!="analysis_sync_failed")editor.putString("last_reason",reason)
        editor.commit()
    }
    private fun receiptKey(scope:String,id:String)="seen:"+scope+":"+id
    fun seen(c:Context,scope:String,id:String,token:String)=prefs(c).getString(receiptKey(scope,id),null)==token
    fun mark(c:Context,scope:String,id:String,token:String){prefs(c).edit().putString(receiptKey(scope,id),token).commit()}
    private fun cacheFile(c:Context,scope:String,kind:String):File {
        val hash=java.security.MessageDigest.getInstance("SHA-256").digest(scope.toByteArray()).joinToString(""){"%02x".format(it)}
        val dir=File(c.filesDir,"background/$hash").apply{mkdirs()};return File(dir,"$kind.json")
    }
    fun cached(c:Context,scope:String,kind:String):String? {require(kind in listOf("overview","local-data"));return cacheFile(c,scope,kind).takeIf{it.exists()}?.readText()}
    private fun save(c:Context,scope:String,kind:String,value:JSONObject) {val file=cacheFile(c,scope,kind);val tmp=File(file.path+".tmp");tmp.writeText(value.toString());check(tmp.renameTo(file))}
    private fun api(server:String,token:String,path:String,body:JSONObject?=null):JSONObject {
        if(Thread.currentThread().isInterrupted)throw InterruptedException()
        val conn=URL(server+"/api/v1/background"+path).openConnection() as HttpURLConnection
        conn.connectTimeout=15000;conn.readTimeout=15000;conn.instanceFollowRedirects=false
        conn.setRequestProperty("Authorization","Bearer $token");conn.setRequestProperty("Content-Type","application/json")
        try {
            if(body!=null){conn.requestMethod="POST";conn.doOutput=true;conn.outputStream.use{it.write(body.toString().toByteArray())}}
            if(conn.responseCode==401)throw BackgroundAuthException()
            check(conn.responseCode in 200..299){"服务器暂不可达 (${conn.responseCode})"}
            val raw=conn.inputStream.use{input->val out=java.io.ByteArrayOutputStream();val buffer=ByteArray(8192);while(true){val n=input.read(buffer);if(n<0)break;check(out.size()+n<=2*1024*1024);out.write(buffer,0,n)};out.toByteArray()}
            return JSONObject(String(raw))
        } finally {conn.disconnect()}
    }
    // The Android desugared Instant parser rejects RFC3339 offsets such as +08:00.
    // Server timestamps can include both an offset and nanoseconds.
    private fun time(value:String):Long=try{OffsetDateTime.parse(value).toInstant().toEpochMilli()}catch(_:Exception){0}
    @JvmOverloads @Synchronized fun run(c:Context,refreshCaches:Boolean=true) {
        val prefs=prefs(c);if(!prefs.getBoolean("enabled",false) || prefs.getBoolean("auth_paused",false))return
        val server=prefs.getString("server","")!!;val scope=prefs.getString("scope","")!!;val credential=token(c)
        prefs.edit().putString("last_run",Instant.now().toString()).commit()
        prefs.getString("release:"+scope,null)?.let { batch ->
            api(server,credential,"/reminders",JSONObject().put("action","release").put("batch_id",batch))
            check(prefs.edit().remove("release:"+scope).commit())
        }
        val feed=api(server,credential,"/reminders")
        prefs.edit().putString("last_sync",Instant.now().toString()).putBoolean("primary",feed.optBoolean("primary")).putString("source_observed_at",feed.optString("source_observed_at")).remove("error").commit()
        try { deliver(c,server,scope,credential,feed) } finally {
            // Analytics and overview caches never block event delivery.
            var cacheFailed=false
            // Flutter already refreshes foreground data. Slow cache downloads must
            // not occupy the reminder worker between its 15-second checks.
            for(kind in if(refreshCaches)listOf("overview","local-data") else emptyList()) {
                try {save(c,scope,kind,api(server,credential,"/"+kind))}
                catch(e:Exception){if(e is InterruptedException)throw e;cacheFailed=true;prefs.edit().putString("analysis_error","数据缓存未更新，提醒独立检查").commit();diagnostic(c,"analysis_sync_failed",kind)}
            }
            if(refreshCaches && !cacheFailed)prefs.edit().remove("analysis_error").commit()
        }
    }
    private fun releaseUnsent(c:Context,server:String,scope:String,credential:String,batch:String,events:JSONArray) {
        // Persist the release before networking, so a transient failure can be retried.
        val edit=prefs(c).edit().putString("release:"+scope,batch)
        for(i in 0 until events.length())edit.remove("presented:"+scope+":"+events.getJSONObject(i).getString("id"))
        check(edit.commit())
        api(server,credential,"/reminders",JSONObject().put("action","release").put("batch_id",batch))
        check(prefs(c).edit().remove("release:"+scope).commit())
    }
    private fun deliver(c:Context,server:String,scope:String,credential:String,feed:JSONObject) {
        if(!feed.optBoolean("primary")){diagnostic(c,"not_primary");return}
        if(feed.optBoolean("quiet")){diagnostic(c,"quiet_hours");return}
        val generated=time(feed.optString("generated_at"));val age=System.currentTimeMillis()-generated
        if(generated==0L || age>60000 || age< -120000){diagnostic(c,"stale_response");return}
        val manager=c.getSystemService(NotificationManager::class.java);val channel=channel(c)
        if(!manager.areNotificationsEnabled()){diagnostic(c,"permission_denied");return}
        if(channel?.importance==NotificationManager.IMPORTANCE_NONE){diagnostic(c,"channel_blocked");return}
        val candidates=feed.optJSONArray("events")?:JSONArray();val ids=JSONArray();val migrated=JSONArray()
        for(i in 0 until candidates.length()) {
            val event=candidates.getJSONObject(i);val id=event.getString("id")
            val legacy=event.optString("legacy_id")
            val encoded=if(legacy.startsWith("codex:"))"codex:"+URLEncoder.encode(legacy.substring(6),"UTF-8").replace("+","%20") else legacy
            if(legacy.isNotEmpty() && (prefs(c).contains(receiptKey(scope,legacy)) || prefs(c).contains(receiptKey(scope,encoded)))) {
                prefs(c).edit().putString("presented:"+scope+":"+id,"legacy").commit();migrated.put(id);continue
            }
            if(prefs(c).contains("presented:"+scope+":"+id)){diagnostic(c,"duplicate",id);continue}
            ids.put(id)
        }
        if(migrated.length()>0)api(server,credential,"/reminders",JSONObject().put("action","migrate").put("ids",migrated))
        if(ids.length()==0){diagnostic(c,feed.optString("reason","no_new_events"));return}
        val reserved=api(server,credential,"/reminders",JSONObject().put("action","reserve").put("ids",ids))
        if(!reserved.optBoolean("claimed")){diagnostic(c,reserved.optString("reason","not_eligible"));return}
        val batch=reserved.getString("batch_id");val events=reserved.getJSONArray("events")
        // User/account changes before submitting are definitely unsent and may release.
        if(Thread.currentThread().isInterrupted || !prefs(c).getBoolean("enabled",false) || prefs(c).getString("scope","")!=scope) {
            releaseUnsent(c,server,scope,credential,batch,events);return
        }
        val first=events.getJSONObject(0)
        val title=if(events.length()==1)first.getString("title") else "AI Hub · "+events.length()+" 条新提醒"
        val body=(0 until events.length()).map{events.getJSONObject(it)}.take(8).joinToString("\n"){it.getString("title")+"："+it.getString("body")}
        val open=Intent(c,MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP)
        val pending=PendingIntent.getActivity(c,batch.hashCode(),open,PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
        val builder=if(Build.VERSION.SDK_INT>=26)Notification.Builder(c,CHANNEL) else Notification.Builder(c)
        val notification=builder.setSmallIcon(c.applicationInfo.icon).setContentTitle(title).setContentText(body).setStyle(Notification.BigTextStyle().bigText(body)).setContentIntent(pending).setAutoCancel(true).setOnlyAlertOnce(true).setCategory(Notification.CATEGORY_REMINDER).setVisibility(Notification.VISIBILITY_PRIVATE).setPriority(Notification.PRIORITY_HIGH).setDefaults(Notification.DEFAULT_ALL).build()
        // Recheck after the network reservation: users can revoke permission meanwhile.
        val blocked=if(!manager.areNotificationsEnabled())"permission_denied" else if(channel(c)?.importance==NotificationManager.IMPORTANCE_NONE)"channel_blocked" else null
        if(blocked!=null){diagnostic(c,blocked);releaseUnsent(c,server,scope,credential,batch,events);return}
        val editor=prefs(c).edit()
        for(i in 0 until events.length()){editor.putString("presented:"+scope+":"+events.getJSONObject(i).getString("id"),"reserved")}
        if(!editor.commit()){releaseUnsent(c,server,scope,credential,batch,events);error("提醒记录保存失败，尚未提交通知")}
        val inApp=ForegroundReminders.show(title,body) {
            prefs(c).getBoolean("enabled",false) && prefs(c).getString("scope","")==scope &&
                manager.areNotificationsEnabled() && channel(c)?.importance!=NotificationManager.IMPORTANCE_NONE
        }
        // A foreground dialog and a system notification are alternative surfaces.
        if(!inApp && (!prefs(c).getBoolean("enabled",false) || prefs(c).getString("scope","")!=scope)) {
            releaseUnsent(c,server,scope,credential,batch,events);return
        }
        if(!inApp && (!manager.areNotificationsEnabled() || channel(c)?.importance==NotificationManager.IMPORTANCE_NONE)) {
            diagnostic(c,if(!manager.areNotificationsEnabled())"permission_denied" else "channel_blocked")
            releaseUnsent(c,server,scope,credential,batch,events);return
        }
        try {if(!inApp)manager.notify(batch.hashCode() and 0x7fffffff,notification)}
        catch(e:SecurityException){diagnostic(c,"permission_denied");releaseUnsent(c,server,scope,credential,batch,events);return}
        prefs(c).edit().putString("last_notification",Instant.now().toString()).commit()
        diagnostic(c,if(inApp)"foreground_presented" else "notification_submitted",batch)
        // Server reservation survives a lost receipt; it cannot cause another popup.
        api(server,credential,"/reminders",JSONObject().put("action","submitted").put("batch_id",batch))
    }
    fun recordError(c:Context,e:Exception){
        if(e is BackgroundAuthException) {prefs(c).edit().putBoolean("auth_paused",true).putString("error","后台授权失效，请打开应用重新启用").commit();diagnostic(c,"auth_expired");c.getSystemService(JobScheduler::class.java).cancel(JOB)}
        else {prefs(c).edit().putString("error",e.message?:"后台检查失败，将重试").commit();diagnostic(c,"network_or_delivery_error")}
    }

}

class BackgroundAuthException: Exception("后台授权失效，请打开应用重新启用")

class BackgroundSyncJob: JobService() {
    private val executor=Executors.newSingleThreadExecutor()
    private var task:Future<*>?=null
    override fun onStartJob(params:JobParameters):Boolean {
        task=executor.submit {var retry=false;try{BackgroundSync.run(this)}catch(e:Exception){BackgroundSync.recordError(this,e);retry=e !is BackgroundAuthException};if(!Thread.currentThread().isInterrupted)jobFinished(params,retry)}
        return true
    }
    override fun onStopJob(params:JobParameters):Boolean {task?.cancel(true);return true}
    override fun onDestroy(){executor.shutdownNow();super.onDestroy()}
}
