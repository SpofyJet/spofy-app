package app.spofy

import android.Manifest
import android.app.Activity
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.ServiceConnection
import android.content.pm.PackageManager
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import android.os.IBinder
import android.provider.Settings
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel

class MainActivity : FlutterActivity() {
    private val starting = mutableListOf<MethodChannel.Result>()
    private var asking: MethodChannel.Result? = null
    private var channel: MethodChannel? = null
    private var offered: String? = null
    private var binding = false
    private var bound = false

    private val link = object : ServiceConnection {
        override fun onServiceConnected(name: ComponentName, binder: IBinder) {
            bound = true
            starting.forEach { it.success(CoreService.socket(this@MainActivity).path) }
            starting.clear()
        }

        override fun onServiceDisconnected(name: ComponentName) {
            bound = false
        }
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
        if (binding) {
            unbindService(link)
            binding = false
            bound = false
        }
        super.onDestroy()
    }

    private companion object {
        const val CONSENT = 7101
        const val NOTICES = 7102
    }
}
