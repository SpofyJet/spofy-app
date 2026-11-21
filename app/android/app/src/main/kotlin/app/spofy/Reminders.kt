package app.spofy

import android.app.AlarmManager
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.Build
import java.util.Calendar
import kotlin.math.roundToLong

object Reminders {
    const val FIRE = "app.spofy.REMIND"
    const val DEFAULT = 3
    private const val STORE = "reminders"
    private const val EXPIRE = "expire"
    private const val DAYS = "days"
    private const val SHOWN = "shown"
    private const val KIND = "kind"
    private const val SOON = 0
    private const val ENDED = 1
    private const val CHANNEL = "reminders"
    private const val NOTICE = 3
    private const val DAY = 86_400_000L
    private const val LIFETIME = 4_070_908_800L
    private const val LATE = 3 * DAY

    fun plan(context: Context, expire: Long, days: Int) {
        val store = context.getSharedPreferences(STORE, Context.MODE_PRIVATE)
        if (store.getLong(EXPIRE, -1) == expire && store.getInt(DAYS, -1) == days) {
            return
        }
        store.edit().putLong(EXPIRE, expire).putInt(DAYS, days).apply()
        arm(context)
    }

    fun arm(context: Context) {
        val store = context.getSharedPreferences(STORE, Context.MODE_PRIVATE)
        val expire = store.getLong(EXPIRE, 0)
        val days = store.getInt(DAYS, DEFAULT)
        val alarms = context.getSystemService(AlarmManager::class.java)
        for (kind in intArrayOf(SOON, ENDED)) {
            alarms.cancel(pending(context, kind, expire))
        }
        if (expire <= 0 || expire >= LIFETIME || days <= 0) {
            return
        }
        val now = System.currentTimeMillis()
        val end = expire * 1000
        if (end > now) {
            alarms.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, maxOf(noon(end - days * DAY), now), pending(context, SOON, expire))
            alarms.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, end, pending(context, ENDED, expire))
        } else if (now - end < LATE) {
            alarms.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, now, pending(context, ENDED, expire))
        }
    }

    fun fire(context: Context, intent: Intent) {
        val store = context.getSharedPreferences(STORE, Context.MODE_PRIVATE)
        val expire = intent.getLongExtra(EXPIRE, 0)
        val kind = intent.getIntExtra(KIND, -1)
        val key = "$kind:$expire"
        val shown = store.getString(SHOWN, "").orEmpty().split(",").filter { it.isNotEmpty() }
        if (expire <= 0 || store.getLong(EXPIRE, 0) != expire || store.getInt(DAYS, DEFAULT) <= 0 || key in shown) {
            return
        }
        val words = Language.words(context)
        val res = words.resources
        val left = between(System.currentTimeMillis(), expire * 1000)
        val title = when {
            kind == ENDED -> res.getString(R.string.remind_ended)
            kind == SOON && left == 0L -> res.getString(R.string.remind_today)
            kind == SOON && left == 1L -> res.getString(R.string.remind_tomorrow)
            kind == SOON && left > 1L -> res.getQuantityString(R.plurals.remind_soon, left.toInt(), left.toInt())
            else -> return
        }
        val text = res.getString(if (kind == ENDED) R.string.remind_ended_hint else R.string.remind_soon_hint)
        store.edit().putString(SHOWN, (shown + key).takeLast(4).joinToString(",")).apply()
        post(context, words, title, text)
    }

    private fun post(context: Context, words: Context, title: String, text: String) {
        val manager = context.getSystemService(NotificationManager::class.java)
        val builder = if (Build.VERSION.SDK_INT >= 26) {
            manager.createNotificationChannel(NotificationChannel(CHANNEL, words.getString(R.string.reminders), NotificationManager.IMPORTANCE_DEFAULT))
            Notification.Builder(context, CHANNEL)
        } else {
            @Suppress("DEPRECATION")
            Notification.Builder(context)
        }
        val flags = PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT
        val open = PendingIntent.getActivity(context, 0, Intent(context, MainActivity::class.java), flags)
        val notice = builder.setSmallIcon(R.drawable.ic_notice)
            .setContentTitle(title)
            .setContentText(text)
            .setAutoCancel(true)
            .setCategory(Notification.CATEGORY_REMINDER)
            .setContentIntent(open)
            .build()
        try {
            manager.notify(NOTICE, notice)
        } catch (e: SecurityException) {
        }
    }

    private fun pending(context: Context, kind: Int, expire: Long): PendingIntent {
        val intent = Intent(context, ReminderReceiver::class.java).setAction(FIRE).putExtra(KIND, kind).putExtra(EXPIRE, expire)
        return PendingIntent.getBroadcast(context, kind, intent, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
    }

    private fun day(at: Long, hour: Int): Long = Calendar.getInstance().apply {
        timeInMillis = at
        set(Calendar.HOUR_OF_DAY, hour)
        set(Calendar.MINUTE, 0)
        set(Calendar.SECOND, 0)
        set(Calendar.MILLISECOND, 0)
    }.timeInMillis

    private fun noon(at: Long) = day(at, 12)

    private fun between(from: Long, to: Long) = ((day(to, 0) - day(from, 0)).toDouble() / DAY).roundToLong()
}

class ReminderReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action == Reminders.FIRE) {
            Reminders.fire(context, intent)
        }
    }
}

class RearmReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        when (intent.action) {
            Intent.ACTION_BOOT_COMPLETED, Intent.ACTION_TIME_CHANGED, Intent.ACTION_TIMEZONE_CHANGED -> Reminders.arm(context)
            Intent.ACTION_MY_PACKAGE_REPLACED -> {
                Reminders.arm(context)
                resume(context)
            }
        }
    }

    private fun resume(context: Context) {
        if (!CoreService.resumed(context).isFile || VpnService.prepare(context) != null) {
            return
        }
        val start = Intent(VpnService.SERVICE_INTERFACE).setClass(context, CoreService::class.java)
        try {
            if (Build.VERSION.SDK_INT >= 26) {
                context.startForegroundService(start)
            } else {
                context.startService(start)
            }
        } catch (e: Exception) {
        }
    }
}
