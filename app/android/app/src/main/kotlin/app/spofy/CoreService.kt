package app.spofy

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.ServiceInfo
import android.net.ConnectivityManager
import android.net.LocalSocket
import android.net.LocalSocketAddress
import android.net.Network
import android.net.VpnService
import android.os.Binder
import android.os.Build
import android.os.IBinder
import android.util.Log
import java.io.File
import java.io.FileDescriptor
import java.io.IOException
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import kotlin.concurrent.thread
import org.json.JSONObject

class CoreService : VpnService() {
    private val local = Binder()
    private val writing = Any()
    private val asks = Executors.newSingleThreadExecutor()
    private var core: Process? = null
    private var link: LocalSocket? = null
    private var sent = 100

    @Volatile
    private var closing = false

    @Volatile
    private var shown = false

    @Volatile
    private var wanted = false

    @Volatile
    private var state = "idle"
    private var watching = false
    private var ready = false
    private val waiting = ArrayDeque<JSONObject>()

    private val network = object : ConnectivityManager.NetworkCallback() {
        private var current: Network? = null

        override fun onAvailable(network: Network) {
            note("network $network is the default")
            current = network
            request("online") { it.put("online", true) }
            again()
        }

        override fun onLost(network: Network) {
            note("network $network is lost, the default was $current")
            if (network == current) {
                current = null
                request("online") { it.put("online", false) }
            }
        }
    }

    private val screen = object : BroadcastReceiver() {
        override fun onReceive(context: Context, intent: Intent) {
            request("check")
            again()
        }
    }

    override fun onCreate() {
        super.onCreate()
        thread(name = "core") {
            var failures = 0
            var runs = 0
            while (!closing && failures < RESTARTS) {
                val began = System.nanoTime()
                try {
                    serve(runs++ > 0)
                } catch (e: Exception) {
                    if (BuildConfig.DEBUG) {
                        Log.e(TAG, "the core stopped", e)
                    }
                }
                release()
                failures = if (System.nanoTime() - began > TimeUnit.MINUTES.toNanos(1)) 0 else failures + 1
                if (!closing) {
                    Thread.sleep(300L * failures)
                }
            }
            wanted = false
            lower()
        }
    }

    override fun onBind(intent: Intent): IBinder? {
        return if (intent.action == SERVICE_INTERFACE) super.onBind(intent) else local
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            DISCONNECT -> {
                wanted = false
                request("disconnect")
            }
            SERVICE_INTERFACE -> {
                wanted = true
                front(getString(R.string.state_connecting), "")
                watch(true)
                request("connect")
            }
        }
        return START_NOT_STICKY
    }

    override fun onRevoke() {
        wanted = false
        request("disconnect")
    }

    override fun onDestroy() {
        closing = true
        release()
        super.onDestroy()
    }

    private fun serve(restarted: Boolean) {
        val folder = File(filesDir, "state")
        val path = socket(this)
        path.delete()
        val launcher = ProcessBuilder(
            File(applicationInfo.nativeLibraryDir, "libspofycore.so").path,
            "serve", "-state", folder.path, "-listen", path.path, "-platform", "-bypass", "-log", if (BuildConfig.DEBUG) "info" else "warning",
            "-hwid", Keys.device(this), "-os", "Android", "-os-version", Build.VERSION.RELEASE ?: "", "-model", Build.MODEL ?: "",
        )
        val env = launcher.environment()
        env["SPOFY_STATE_KEY"] = Keys.state(this, folder)
        env["TMPDIR"] = cacheDir.path
        env["HOME"] = filesDir.path
        val roots = ROOTS.filter { File(it).isDirectory }
        if (roots.isNotEmpty()) {
            env["SSL_CERT_DIR"] = roots.joinToString(":")
        }
        if (BuildConfig.DEBUG) {
            val extra = File(filesDir, "test-ca.pem")
            if (extra.isFile) {
                env["SSL_CERT_FILE"] = extra.path
            }
        }
        val process = launcher.start()
        core = process
        thread(name = "core-log") {
            try {
                process.errorStream.bufferedReader().forEachLine {
                    if (BuildConfig.DEBUG) {
                        Log.d(TAG, it)
                    }
                }
            } catch (e: IOException) {
            }
        }
        val first = process.inputStream.bufferedReader().readLine()
        if (first == null || !first.contains("listening")) {
            throw IOException("the core did not start")
        }
        val socket = LocalSocket()
        socket.connect(LocalSocketAddress(path.path, LocalSocketAddress.Namespace.FILESYSTEM))
        link = socket
        send(JSONObject().put("id", 1).put("op", "platform"))
        asks.execute {
            ready = true
            while (true) {
                send(waiting.removeFirstOrNull() ?: break)
            }
        }
        if (restarted && wanted) {
            request("connect")
        }
        val lines = socket.inputStream.bufferedReader()
        while (true) {
            val line = lines.readLine() ?: break
            handle(JSONObject(line))
        }
    }

    private fun release() {
        asks.execute { ready = false }
        if (closing || !wanted) {
            lower()
        }
        try {
            link?.close()
        } catch (e: IOException) {
        }
        link = null
        val process = core ?: return
        core = null
        try {
            process.outputStream.close()
        } catch (e: IOException) {
        }
        thread(name = "core-end") {
            if (Build.VERSION.SDK_INT >= 26) {
                if (!process.waitFor(3, TimeUnit.SECONDS)) {
                    process.destroyForcibly()
                }
            } else {
                Thread.sleep(3000)
                process.destroy()
            }
        }
    }

    private fun handle(message: JSONObject) {
        if (!message.has("ask")) {
            if (message.optString("ev") == "connection") {
                state = message.getJSONObject("connection").optString("state")
                note("state $state")
                if (state == "idle") {
                    wanted = false
                    lower()
                } else {
                    show(state)
                }
            } else if (message.has("re") && !message.optBoolean("ok")) {
                note("request ${message.optInt("re")} failed: ${message.optString("code")}")
            }
            return
        }
        note("the core asks for ${message.optString("op")}")
        val answer = JSONObject().put("answer", message.getLong("ask"))
        when (message.optString("op")) {
            "tun" -> tunnel(answer, message.getJSONObject("tun"))
            "untun" -> {
                if (!wanted) {
                    lower()
                }
                send(answer.put("ok", true))
            }
            else -> send(answer.put("ok", false).put("error", "the platform does not know this request"))
        }
    }

    private fun tunnel(answer: JSONObject, spec: JSONObject) {
        val allowed = try {
            prepare(this) == null
        } catch (e: Exception) {
            false
        }
        if (!allowed) {
            send(answer.put("ok", false).put("error", "the user did not allow the tunnel"))
            return
        }
        val tunnel = try {
            val builder = Builder().setSession(getString(R.string.app)).setMtu(spec.optInt("mtu", 1500))
            each(spec, "addresses") { address, prefix -> builder.addAddress(address, prefix) }
            each(spec, "routes") { address, prefix -> builder.addRoute(address, prefix) }
            val dns = spec.optJSONArray("dns")
            for (i in 0 until (dns?.length() ?: 0)) {
                builder.addDnsServer(dns!!.getString(i))
            }
            builder.addDisallowedApplication(packageName)
            if (Build.VERSION.SDK_INT >= 29) {
                builder.setMetered(false)
            }
            builder.establish()
        } catch (e: Exception) {
            if (BuildConfig.DEBUG) {
                Log.e(TAG, "no tunnel", e)
            }
            null
        }
        if (tunnel == null) {
            send(answer.put("ok", false).put("error", "the system did not give the tunnel"))
            return
        }
        raise()
        send(answer.put("ok", true).put("file", true), tunnel.fileDescriptor)
        tunnel.close()
    }

    private fun each(spec: JSONObject, key: String, add: (String, Int) -> Unit) {
        val list = spec.optJSONArray(key) ?: return
        for (i in 0 until list.length()) {
            val (address, prefix) = list.getString(i).split("/")
            add(address, prefix.toInt())
        }
    }

    private fun raise() {
        try {
            startService(Intent(this, CoreService::class.java))
        } catch (e: Exception) {
            if (BuildConfig.DEBUG) {
                Log.e(TAG, "not started", e)
            }
        }
        if (!shown) {
            front(getString(R.string.state_connecting), "")
        }
        watch(true)
    }

    private fun front(title: String, text: String) {
        try {
            val notice = notice(title, text)
            if (Build.VERSION.SDK_INT >= 34) {
                startForeground(NOTICE, notice, ServiceInfo.FOREGROUND_SERVICE_TYPE_SYSTEM_EXEMPTED)
            } else {
                startForeground(NOTICE, notice)
            }
            shown = true
        } catch (e: Exception) {
            if (BuildConfig.DEBUG) {
                Log.e(TAG, "no foreground", e)
            }
        }
    }

    private fun lower() {
        watch(false)
        if (shown) {
            shown = false
            stopForeground(STOP_FOREGROUND_REMOVE)
        }
        stopSelf()
    }

    private fun again() {
        if (wanted && state == "failed") {
            request("connect")
        }
    }

    @Synchronized
    private fun watch(on: Boolean) {
        if (on == watching) {
            return
        }
        watching = on
        val connectivity = getSystemService(ConnectivityManager::class.java)
        try {
            if (on) {
                connectivity.registerDefaultNetworkCallback(network)
                val filter = IntentFilter(Intent.ACTION_SCREEN_ON)
                if (Build.VERSION.SDK_INT >= 33) {
                    registerReceiver(screen, filter, RECEIVER_NOT_EXPORTED)
                } else {
                    registerReceiver(screen, filter)
                }
            } else {
                connectivity.unregisterNetworkCallback(network)
                unregisterReceiver(screen)
            }
        } catch (e: Exception) {
            if (BuildConfig.DEBUG) {
                Log.e(TAG, "watch", e)
            }
        }
    }

    private fun show(state: String) {
        val title = when (state) {
            "connected" -> R.string.state_connected
            "connecting", "measuring" -> R.string.state_connecting
            "switching" -> R.string.state_switching
            "stalled" -> R.string.state_stalled
            "offline" -> R.string.state_offline
            "failed" -> R.string.state_failed
            else -> return
        }
        if (shown) {
            val text = when (state) {
                "stalled" -> getString(R.string.stalled_hint)
                "failed" -> getString(R.string.failed_hint)
                else -> ""
            }
            getSystemService(NotificationManager::class.java).notify(NOTICE, notice(getString(title), text))
        }
    }

    private fun notice(title: String, text: String): Notification {
        val manager = getSystemService(NotificationManager::class.java)
        val builder = if (Build.VERSION.SDK_INT >= 26) {
            if (manager.getNotificationChannel(CHANNEL) == null) {
                val channel = NotificationChannel(CHANNEL, getString(R.string.channel), NotificationManager.IMPORTANCE_LOW)
                channel.setShowBadge(false)
                manager.createNotificationChannel(channel)
            }
            Notification.Builder(this, CHANNEL)
        } else {
            @Suppress("DEPRECATION")
            Notification.Builder(this)
        }
        val flags = PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
        val open = PendingIntent.getActivity(this, 0, Intent(this, MainActivity::class.java), flags)
        val stop = PendingIntent.getService(this, 1, Intent(this, CoreService::class.java).setAction(DISCONNECT), flags)
        builder.setSmallIcon(R.drawable.ic_notice)
            .setContentTitle(title)
            .setOngoing(true)
            .setShowWhen(false)
            .setOnlyAlertOnce(true)
            .setCategory(Notification.CATEGORY_SERVICE)
            .setContentIntent(open)
            .addAction(Notification.Action.Builder(null, getString(R.string.disconnect), stop).build())
        if (text.isNotEmpty()) {
            builder.setContentText(text)
        }
        return builder.build()
    }

    private fun request(op: String, fill: (JSONObject) -> Unit = {}) {
        asks.execute {
            val id = synchronized(writing) { ++sent }
            val message = JSONObject().put("id", id).put("op", op).also(fill)
            note("request $message")
            if (ready) {
                send(message)
            } else {
                waiting.addLast(message)
                while (waiting.size > PENDING) {
                    waiting.removeFirst()
                }
            }
        }
    }

    private fun note(text: String) {
        if (BuildConfig.DEBUG) {
            Log.d(TAG, text)
        }
    }

    private fun send(message: JSONObject, descriptor: FileDescriptor? = null) {
        val socket = link ?: return
        synchronized(writing) {
            try {
                if (descriptor != null) {
                    socket.setFileDescriptorsForSend(arrayOf(descriptor))
                }
                socket.outputStream.write((message.toString() + "\n").toByteArray())
                socket.outputStream.flush()
            } catch (e: IOException) {
            } finally {
                if (descriptor != null) {
                    socket.setFileDescriptorsForSend(null)
                }
            }
        }
    }

    companion object {
        const val DISCONNECT = "app.spofy.DISCONNECT"
        private const val TAG = "spofy"
        private const val CHANNEL = "state"
        private const val NOTICE = 1
        private const val RESTARTS = 5
        private const val PENDING = 8
        private val ROOTS = listOf("/apex/com.android.conscrypt/cacerts", "/system/etc/security/cacerts")

        fun socket(context: Context) = File(context.filesDir, "core.sock")
    }
}
