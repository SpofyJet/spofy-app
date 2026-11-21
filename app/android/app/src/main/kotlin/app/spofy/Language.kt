package app.spofy

import android.content.Context
import android.content.res.Configuration
import java.util.Locale

object Language {
    private const val PLACE = "look"
    private const val KEY = "language"
    private val russian = setOf("ru", "uk", "be", "kk")

    fun pick(setting: String, system: List<Locale>): Locale {
        if (setting == "ru" || setting == "en") {
            return Locale.forLanguageTag(setting)
        }
        for (locale in system) {
            val code = locale.language.lowercase(Locale.ROOT)
            if (code in russian) {
                return Locale.forLanguageTag("ru")
            }
            if (code == "en") {
                return Locale.ENGLISH
            }
        }
        return Locale.ENGLISH
    }

    fun saved(context: Context): String = context.getSharedPreferences(PLACE, Context.MODE_PRIVATE).getString(KEY, "") ?: ""

    fun save(context: Context, setting: String) {
        context.getSharedPreferences(PLACE, Context.MODE_PRIVATE).edit().putString(KEY, setting).apply()
    }

    fun words(context: Context, setting: String = saved(context)): Context {
        val config = Configuration(context.resources.configuration)
        val system = config.locales
        config.setLocale(pick(setting, (0 until system.size()).map { system[it] }))
        return context.createConfigurationContext(config)
    }
}
