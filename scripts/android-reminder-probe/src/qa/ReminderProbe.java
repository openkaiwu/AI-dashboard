package qa;

import android.app.*;
import android.content.*;
import android.os.*;
import android.security.keystore.*;
import android.util.Base64;
import org.json.*;
import java.io.*;
import java.net.*;
import java.security.*;
import java.time.Instant;
import javax.crypto.*;

/** Runs the real native dispatcher against an in-process HTTP fixture; no real account. */
public class ReminderProbe extends Instrumentation {
    Context context;
    SharedPreferences prefs;
    Object sync;
    Class<?> syncClass;
    Activity activity;
    String mode;
    String event;
    boolean reserved, submitted, failRelease, disableOnReserve;
    int releases, reserves;
    java.util.concurrent.CountDownLatch entered=new java.util.concurrent.CountDownLatch(1);
    java.util.concurrent.CountDownLatch proceed=new java.util.concurrent.CountDownLatch(1);

    public void onCreate(Bundle args) { mode=args.getString("mode","normal"); super.onCreate(args); start(); }
    void require(boolean value,String message) { if(!value)throw new AssertionError(message); }
    Object invoke(String name,Class<?>[] types,Object... args)throws Exception {
        return syncClass.getMethod(name,types).invoke(sync,args);
    }
    void runCheck()throws Exception { invoke("run",new Class<?>[]{Context.class},context); }
    JSONObject response(String path,String body)throws Exception {
        if(!path.endsWith("/reminders"))throw new IOException("Deliberate cache failure");
        JSONObject e=new JSONObject().put("id",event).put("title","AI Hub 提醒验收")
            .put("body","本地合成事件：验证通知与去重，不影响真实额度。");
        if(body.isEmpty() && mode.equals("join")){entered.countDown();require(proceed.await(10,java.util.concurrent.TimeUnit.SECONDS),"check gate timed out");}
        if(body.isEmpty())return new JSONObject().put("primary",true).put("quiet",mode.equals("quiet") || mode.equals("foreground_quiet"))
            .put("generated_at",Instant.now().toString()).put("events",new JSONArray().put(e));
        String action=new JSONObject(body).getString("action");
        if(action.equals("reserve")) {
            reserves++;
            if(reserved)return new JSONObject().put("claimed",false).put("reason","duplicate");
            reserved=true;
            if(disableOnReserve){prefs.edit().putBoolean("enabled",false).commit();disableOnReserve=false;}
            if(mode.equals("channel_race"))context.getSystemService(NotificationManager.class)
                .createNotificationChannel(new NotificationChannel("aihub_popup_reminders_v2","QA",NotificationManager.IMPORTANCE_NONE));
            return new JSONObject().put("claimed",true).put("batch_id",event).put("events",new JSONArray().put(e));
        }
        if(action.equals("release")) {
            releases++;
            if(failRelease){failRelease=false;throw new IOException("Deliberate lost release");}
            reserved=false;return new JSONObject().put("ok",true);
        }
        if(action.equals("submitted")) {submitted=true;return new JSONObject().put("ok",true);}
        throw new IOException("Unexpected action");
    }
    void configure()throws Exception {
        // Test only on a disposable emulator. Save and restore preferences below.
        KeyStore store=KeyStore.getInstance("AndroidKeyStore");store.load(null);
        SecretKey key=(SecretKey)store.getKey("aihub-background",null);
        if(key==null){KeyGenerator g=KeyGenerator.getInstance("AES","AndroidKeyStore");g.init(new KeyGenParameterSpec.Builder("aihub-background",3).setBlockModes("GCM").setEncryptionPaddings("NoPadding").build());key=g.generateKey();}
        Cipher cipher=Cipher.getInstance("AES/GCM/NoPadding");cipher.init(Cipher.ENCRYPT_MODE,key);
        byte[] encrypted=cipher.doFinal("synthetic-test-credential-only-123456789".getBytes());
        byte[] raw=new byte[12+encrypted.length];System.arraycopy(cipher.getIV(),0,raw,0,12);System.arraycopy(encrypted,0,raw,12,encrypted.length);
        prefs.edit().putString("server","https://reminder.invalid").putString("scope",event)
            .putString("credential",Base64.encodeToString(raw,2)).putBoolean("enabled",true).remove("auth_paused").remove("diagnostics").commit();
    }
    public void onStart() {
        Bundle result=new Bundle(); java.util.Map<String,?> before=null;int code=-1;
        try {
            context=getTargetContext();prefs=context.getSharedPreferences("background-sync",0);
            before=prefs.getAll();event="qa:"+System.currentTimeMillis();
            syncClass=context.getClassLoader().loadClass("dev.aihub.aihub_mobile.BackgroundSync");
            sync=syncClass.getField("INSTANCE").get(null);
            URL.setURLStreamHandlerFactory(protocol -> protocol.equals("https") ? new URLStreamHandler(){
                protected URLConnection openConnection(URL url){return new HttpURLConnection(url){
                    ByteArrayOutputStream body=new ByteArrayOutputStream();
                    public void connect(){} public void disconnect(){} public boolean usingProxy(){return false;}
                    public OutputStream getOutputStream(){return body;}
                    public int getResponseCode(){return 200;}
                    public InputStream getInputStream()throws IOException {
                        try{return new ByteArrayInputStream(response(url.getPath(),body.toString("UTF-8")).toString().getBytes("UTF-8"));}
                        catch(Exception e){throw new IOException(e);}
                    }
                };}
            } : null);
            configure();
            if(mode.startsWith("foreground")) {
                activity=startActivitySync(new Intent().setClassName(context,"dev.aihub.aihub_mobile.ReminderProbeActivity").addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
                waitForIdleSync();
                if(mode.equals("foreground_paused")) {
                    runOnMainSync(()->activity.finish());
                    waitForIdleSync();
                }
            }
            if(mode.equals("auth")) {
                Exception expired=(Exception)context.getClassLoader().loadClass("dev.aihub.aihub_mobile.BackgroundAuthException").getConstructor().newInstance();
                invoke("recordError",new Class<?>[]{Context.class,Exception.class},context,expired);
                invoke("ensureScheduled",new Class<?>[]{Context.class},context);
                require(context.getSystemService(android.app.job.JobScheduler.class).getPendingJob(11501)==null,"expired authorization must not reschedule");
                runCheck();require(reserves==0,"expired authorization must not fetch reminders");
                result.putString("result","PASS auth");
            } else {
            disableOnReserve=mode.equals("release_retry");failRelease=disableOnReserve;
            if(disableOnReserve){
                try{runCheck();throw new AssertionError("release should fail");}catch(java.lang.reflect.InvocationTargetException expected){}
                require(prefs.contains("release:"+event),"unsent release must survive network failure");
                require(!submitted,"must not mark unsent as submitted");
                prefs.edit().putBoolean("enabled",true).commit();
            }
            if(mode.equals("join")) {
                Class<?> callback=context.getClassLoader().loadClass("kotlin.jvm.functions.Function0");
                Object unit=context.getClassLoader().loadClass("kotlin.Unit").getField("INSTANCE").get(null);
                java.util.concurrent.CountDownLatch done=new java.util.concurrent.CountDownLatch(2);
                Object cb=java.lang.reflect.Proxy.newProxyInstance(context.getClassLoader(),new Class<?>[]{callback},(p,m,a)->{if(m.getName().equals("invoke")){done.countDown();return unit;}return null;});
                invoke("checkNow",new Class<?>[]{Context.class,callback},context,cb);
                require(entered.await(10,java.util.concurrent.TimeUnit.SECONDS),"first check did not start");
                invoke("checkNow",new Class<?>[]{Context.class,callback},context,cb);
                require(done.getCount()==2,"callback must wait for delivery");proceed.countDown();
                require(done.await(20,java.util.concurrent.TimeUnit.SECONDS),"both joined callbacks must complete");
            } else runCheck();
            if(mode.equals("quiet") || mode.equals("foreground_quiet")){require(reserves==0,"quiet must not reserve");}
            else if(mode.equals("denied")) {
                require(reserves==0 && !submitted,"denied permission must not consume event");
                require("permission_denied".equals(prefs.getString("last_reason","")),"permission failure must remain visible");
            }
            else if(mode.equals("channel_race")){
                require(releases==1 && !submitted,"blocked channel must release unsent event");
                require(!prefs.contains("presented:"+event+":"+event),"blocked event must remain eligible");
            } else {
                require(submitted,"notification must be submitted despite cache errors");
                require(!prefs.contains("release:"+event),"pending release must be cleared");
                boolean inApp=mode.equals("foreground") || mode.equals("foreground_preview");
                require((inApp?"foreground_presented":"notification_submitted").equals(prefs.getString("last_reason","")),"cache error must not hide delivery result");
                if(inApp) {
                    Class<?> host=context.getClassLoader().loadClass("dev.aihub.aihub_mobile.ForegroundReminders");
                    var field=host.getDeclaredField("dialog");field.setAccessible(true);
                    runOnMainSync(()->{try{require(((AlertDialog)field.get(null)).isShowing(),"foreground dialog missing");}catch(IllegalAccessException e){throw new RuntimeException(e);}});
                    boolean duplicate=false;
                    for(var n:context.getSystemService(NotificationManager.class).getActiveNotifications())if(n.getId()==(event.hashCode()&0x7fffffff))duplicate=true;
                    require(!duplicate,"foreground event must not also post a system notification");
                    if(mode.equals("foreground_preview"))Thread.sleep(45000);
                }
                int count=reserves;runCheck();require(reserves==count,"repeat check must not reserve again");
                if(mode.equals("release_retry"))require(releases==2,"failed release must retry before next reserve");
                if(!inApp)require(context.getSystemService(NotificationManager.class).getActiveNotifications().length>0,"system notification missing");
            }
            result.putString("result","PASS "+mode);result.putInt("reserves",reserves);result.putInt("releases",releases);
            }
        } catch(Throwable e){result.putString("result","FAIL "+mode+": "+e);code=1;}
        finally {
            if(activity!=null)runOnMainSync(()->activity.finish());
            if(before!=null){
                SharedPreferences.Editor ed=prefs.edit().clear();
                for(var entry:before.entrySet()) {Object v=entry.getValue();String k=entry.getKey();if(v instanceof String)ed.putString(k,(String)v);else if(v instanceof Boolean)ed.putBoolean(k,(Boolean)v);else if(v instanceof Long)ed.putLong(k,(Long)v);else if(v instanceof Integer)ed.putInt(k,(Integer)v);}
                ed.commit();
            }
        }
        finish(code,result);
    }
}
