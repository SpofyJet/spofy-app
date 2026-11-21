package app.spofy

import android.Manifest
import android.app.Activity
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.ServiceConnection
import android.content.pm.PackageManager
import android.graphics.Bitmap
import android.graphics.Canvas
import android.net.Uri
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import android.os.IBinder
import android.os.Parcel
import android.provider.Settings
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel
import java.io.ByteArrayOutputStream
import java.text.Collator
import java.util.concurrent.Executors

class MainActivity : FlutterActivity() {
    private val starting = mutableListOf<MethodChannel.Result>()
    private var asking: MethodChannel.Result? = null
    private var channel: MethodChannel? = null
    private var offered: String? = null
    private var waiting: Pair<String, MethodChannel.Result>? = null
    private var binding = false
    private var bound = false
    private var remote: IBinder? = null
    private val worker = Executors.newSingleThreadExecutor()

    private val link = object : ServiceConnection {
        override fun onServiceConnected(name: ComponentName, binder: IBinder) {
            bound = true
            remote = binder
            starting.forEach { it.success(CoreService.socket(this@MainActivity).path) }
            starting.clear()
        }

        override fun onServiceDisconnected(name: ComponentName) {
            bound = false
            remote = null
        }
    }

    override fun onResume() {
        super.onResume()
        val (path, result) = waiting ?: return
        waiting = null
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O && !packageManager.canRequestPackageInstalls()) {
            result.success(false)
            return
        }
        install(path, result)
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        keep(intent)
        super.onCreate(savedInstanceState)
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        keep(intent)
        val fresh = offered ?: return
        val live = channel ?: return
        offered = null
        live.invokeMethod("link", fresh)
    }

    private fun keep(intent: Intent?) {
        val data = intent?.data ?: return
        if (intent?.action == Intent.ACTION_VIEW && data.scheme == "spofy") {
            offered = data.toString()
        }
    }

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        val host = MethodChannel(flutterEngine.dartExecutor.binaryMessenger, "app.spofy/host")
        channel = host
        host.setMethodCallHandler { call, result ->
            when (call.method) {
                "start" -> start(result)
                "prepare" -> prepare(result)
                "link" -> {
                    result.success(offered)
                    offered = null
                }
                "apps" -> apps(result)
                "icon" -> icon(call.arguments as? String, result)
                "locked" -> result.success(locked())
                "browse" -> {
                    val target = Uri.parse(call.arguments as? String ?: "")
                    if (target.scheme == "https") {
                        try {
                            startActivity(Intent(Intent.ACTION_VIEW, target))
                        } catch (e: Exception) {
                        }
                    }
                    result.success(null)
                }
                "install" -> install(call.arguments as? String, result)
                "drawn" -> {
                    try {
                        reportFullyDrawn()
                    } catch (e: Exception) {
                    }
                    result.success(null)
                }
                "always" -> {
                    try {
                        startActivity(Intent(Settings.ACTION_VPN_SETTINGS))
                    } catch (e: Exception) {
                    }
                    result.success(null)
                }
                else -> result.notImplemented()
            }
        }
    }

    private fun install(path: String?, result: MethodChannel.Result) {
        if (path == null) {
            result.success(false)
            return
        }
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O && !packageManager.canRequestPackageInstalls()) {
            waiting?.second?.success(false)
            waiting = null
            try {
                startActivity(Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:$packageName")))
                waiting = path to result
            } catch (e: Exception) {
                result.success(false)
            }
            return
        }
        worker.execute {
            val ok = try {
                InstallReceiver.install(this, path)
            } catch (e: Exception) {
                false
            }
            runOnUiThread { result.success(ok) }
        }
    }

    private fun start(result: MethodChannel.Result) {
        if (bound) {
            result.success(CoreService.socket(this).path)
            return
        }
        starting.add(result)
        if (!binding) {
            binding = bindService(Intent(this, CoreService::class.java), link, Context.BIND_AUTO_CREATE)
            if (!binding) {
                starting.forEach { it.error("no_service", "the core service did not start", null) }
                starting.clear()
            }
        }
    }

    private fun apps(result: MethodChannel.Result) {
        worker.execute {
            val found = ArrayList<Map<String, String>>()
            try {
                val seen = HashSet<String>()
                val launch = Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER)
                for (info in packageManager.queryIntentActivities(launch, 0)) {
                    val name = info.activityInfo.packageName
                    if (name != packageName && seen.add(name)) {
                        found.add(mapOf("id" to name, "name" to info.activityInfo.applicationInfo.loadLabel(packageManager).toString()))
                    }
                }
                val order = Collator.getInstance()
                found.sortWith { a, b -> order.compare(a["name"] ?: "", b["name"] ?: "") }
            } catch (e: Exception) {
                found.clear()
            }
            runOnUiThread { result.success(found) }
        }
    }

    private fun icon(name: String?, result: MethodChannel.Result) {
        worker.execute {
            val bytes = try {
                val drawable = packageManager.getApplicationIcon(name!!)
                val side = (40 * resources.displayMetrics.density).toInt().coerceIn(48, 144)
                val bitmap = Bitmap.createBitmap(side, side, Bitmap.Config.ARGB_8888)
                drawable.setBounds(0, 0, side, side)
                drawable.draw(Canvas(bitmap))
                val out = ByteArrayOutputStream()
                bitmap.compress(Bitmap.CompressFormat.PNG, 100, out)
                bitmap.recycle()
                out.toByteArray()
            } catch (e: Exception) {
                null
            }
            runOnUiThread { result.success(bytes) }
        }
    }

    private fun locked(): Boolean {
        val service = remote ?: return false
        val data = Parcel.obtain()
        val reply = Parcel.obtain()
        return try {
            service.transact(CoreService.LOCKED, data, reply, 0) && reply.readInt() == 1
        } catch (e: Exception) {
            false
        } finally {
            data.recycle()
            reply.recycle()
        }
    }

    private fun prepare(result: MethodChannel.Result) {
        val consent = VpnService.prepare(this)
        if (consent == null) {
            notices()
            result.success(true)
            return
        }
        asking?.success(false)
        asking = result
        startActivityForResult(consent, CONSENT)
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        if (requestCode != CONSENT) {
            super.onActivityResult(requestCode, resultCode, data)
            return
        }
        asking?.success(resultCode == Activity.RESULT_OK)
        asking = null
        if (resultCode == Activity.RESULT_OK) {
            notices()
        }
    }

    private fun notices() {
        if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), NOTICES)
        }
    }

    override fun onDestroy() {
        channel = null
        worker.shutdown()
        if (binding) {
            unbindService(link)
            binding = false
            bound = false
            remote = null
        }
        super.onDestroy()
    }

    private companion object {
        const val CONSENT = 7101
        const val NOTICES = 7102
    }
}
