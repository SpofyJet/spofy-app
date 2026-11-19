package app.spofy

import android.app.Activity
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.ServiceConnection
import android.net.VpnService
import android.os.IBinder
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel

class MainActivity : FlutterActivity() {
    private val starting = mutableListOf<MethodChannel.Result>()
    private var asking: MethodChannel.Result? = null
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

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, "app.spofy/host").setMethodCallHandler { call, result ->
            when (call.method) {
                "start" -> start(result)
                "prepare" -> prepare(result)
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
    }

    override fun onDestroy() {
        if (binding) {
            unbindService(link)
            binding = false
            bound = false
        }
        super.onDestroy()
    }

    private companion object {
        const val CONSENT = 7101
    }
}
