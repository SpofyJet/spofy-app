package app.spofy

import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import android.os.Build
import java.io.File

class InstallReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE) != PackageInstaller.STATUS_PENDING_USER_ACTION) {
            return
        }
        val confirm = if (Build.VERSION.SDK_INT >= 33) {
            intent.getParcelableExtra(Intent.EXTRA_INTENT, Intent::class.java)
        } else {
            @Suppress("DEPRECATION")
            intent.getParcelableExtra(Intent.EXTRA_INTENT)
        } ?: return
        try {
            context.startActivity(confirm.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        } catch (e: Exception) {
        }
    }

    companion object {
        fun install(context: Context, path: String): Boolean {
            val root = File(context.filesDir, "state/updates").canonicalFile
            val file = File(path).canonicalFile
            if (file.parentFile != root || !file.name.endsWith(".apk") || !file.isFile) {
                return false
            }
            val installer = context.packageManager.packageInstaller
            val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL)
            params.setAppPackageName(context.packageName)
            params.setSize(file.length())
            val id = installer.createSession(params)
            val session = installer.openSession(id)
            try {
                file.inputStream().use { input ->
                    session.openWrite("update.apk", 0, file.length()).use { output ->
                        input.copyTo(output)
                        session.fsync(output)
                    }
                }
                val mutable = if (Build.VERSION.SDK_INT >= 31) PendingIntent.FLAG_MUTABLE else 0
                val back = PendingIntent.getBroadcast(
                    context,
                    id,
                    Intent(context, InstallReceiver::class.java),
                    PendingIntent.FLAG_UPDATE_CURRENT or mutable,
                )
                session.commit(back.intentSender)
                return true
            } catch (e: Exception) {
                session.abandon()
                return false
            } finally {
                session.close()
            }
        }
    }
}
