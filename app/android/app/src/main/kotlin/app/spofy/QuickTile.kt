package app.spofy

import android.app.PendingIntent
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService

class QuickTile : TileService() {
    override fun onTileAdded() {
        show()
    }

    override fun onStartListening() {
        show()
    }

    override fun onClick() {
        if (isLocked) {
            unlockAndRun { toggle() }
        } else {
            toggle()
        }
    }

    private fun toggle() {
        val service = Intent(this, CoreService::class.java)
        if (on()) {
            startService(service.setAction(CoreService.DISCONNECT))
            return
        }
        val allowed = try {
            VpnService.prepare(this) == null
        } catch (e: Exception) {
            false
        }
        if (!allowed) {
            open()
            return
        }
        service.setAction(VpnService.SERVICE_INTERFACE)
        if (Build.VERSION.SDK_INT >= 26) {
            startForegroundService(service)
        } else {
            startService(service)
        }
    }

    private fun open() {
        val app = Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        if (Build.VERSION.SDK_INT >= 34) {
            startActivityAndCollapse(PendingIntent.getActivity(this, 2, app, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT))
        } else {
            @Suppress("DEPRECATION", "StartActivityAndCollapseDeprecated")
            startActivityAndCollapse(app)
        }
    }

    private fun on(): Boolean = CoreService.now != "idle" && CoreService.now != "failed"

    private fun show() {
        val tile = qsTile ?: return
        tile.state = if (on()) Tile.STATE_ACTIVE else Tile.STATE_INACTIVE
        tile.label = getString(R.string.app)
        if (Build.VERSION.SDK_INT >= 29) {
            tile.subtitle = Language.words(this).getString(
                when (CoreService.now) {
                    "connected" -> R.string.tile_on
                    "connecting", "measuring", "switching" -> R.string.tile_starting
                    "stalled", "offline", "failed" -> R.string.tile_trouble
                    else -> R.string.tile_off
                },
            )
        }
        tile.updateTile()
    }
}
