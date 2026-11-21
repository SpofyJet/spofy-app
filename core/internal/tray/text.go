package tray

import (
	"fmt"
	"slices"
	"strings"
)

type Lang int

const (
	Ru Lang = iota
	En
)

const App = "Spofy"

var russian = []string{"ru", "uk", "be", "kk"}

func Pick(setting string, system []string) Lang {
	switch setting {
	case "ru":
		return Ru
	case "en":
		return En
	}
	for _, tag := range system {
		code, _, _ := strings.Cut(strings.ToLower(strings.ReplaceAll(tag, "_", "-")), "-")
		if slices.Contains(russian, code) {
			return Ru
		}
		if code == "en" {
			return En
		}
	}
	return En
}

type words struct {
	connect, disconnect, stop, addSubscription, renew, manage string
	stale, lost, quit, quitOff, open, failure, stopped        string
	titles, hints, reasons, stopTitles, stopActions           map[string]string
	remindToday, remindTomorrow, remindEnded                  string
	remindSoonHint, remindEndedHint                           string
	remindSoon                                                [3]string
}

var texts = [...]words{
	Ru: {
		connect:         "Включить VPN",
		disconnect:      "Выключить VPN",
		stop:            "Остановить подключение",
		addSubscription: "Добавить подписку",
		renew:           "Продлить подписку",
		manage:          "Управление подпиской",
		stale:           "Части приложения разных версий. Обновите Spofy",
		lost:            "Служба Spofy не отвечает",
		quit:            "Выйти",
		quitOff:         "Выключить VPN и выйти",
		open:            "Открыть Spofy",
		failure:         "Ошибка: ",
		stopped:         "Подписка не работает",
		titles: map[string]string{
			"idle":       "VPN выключен",
			"measuring":  "Выбираем сервер",
			"connecting": "Подключаемся",
			"connected":  "VPN работает",
			"switching":  "Меняем сервер",
			"stalled":    "Сервер не отвечает",
			"offline":    "Нет интернета",
			"failed":     "Нет подключения",
		},
		hints: map[string]string{
			"stalled": "Трафик не идёт мимо VPN. Пробуем снова",
		},
		reasons: map[string]string{
			"no_server":          "Ни один сервер не ответил. Проверьте интернет",
			"server_unreachable": "Этот сервер не отвечает. Выберите другой или «Авто»",
			"server_not_found":   "Такого сервера больше нет в подписке",
			"no_subscription":    "Сначала добавьте подписку",
			"sub_bad_url":        "Это не ссылка подписки и не ключ. Ссылка начинается с https://, ключ — с vless://, trojan:// или ss://",
			"sub_unknown":        "Такой подписки больше нет",
			"sub_unreachable":    "Сервер подписки не отвечает. Проверьте\u00a0интернет",
			"sub_status":         "Ссылка не принята. Скопируйте её заново",
			"sub_format":         "По ссылке нет подписки Spofy",
			"sub_ended":          "Подписка закончилась",
			"sub_spent":          "Трафик закончился",
			"sub_devices":        "Все места заняты",
			"sub_other":          "Подписка не работает",
			"store_failed":       "Не удалось сохранить данные на устройстве",
			"connected":          "Проверка скорости доступна, когда VPN выключен",
			"update_none":        "У вас последняя версия",
			"update_unreachable": "Сервер обновлений не отвечает. Попробуйте позже",
			"update_broken":      "Файл обновления не прошёл проверку подписи и не будет установлен",
			"update_file":        "Файл обновления не скачался целиком. Попробуйте ещё раз",
			"update_busy":        "Обновление уже скачивается",
			"update_off":         "В этой сборке обновления выключены",
			"update_install":     "Не удалось открыть установку. Разрешите Spofy устанавливать приложения",
			"vpn_denied":         "Нужно разрешение на VPN. Нажмите ещё раз",
			"guard_failed":       "Нужно разрешение на VPN. Нажмите ещё раз",
			"tun_permission":     "Нет прав, чтобы создать туннель",
		},
		stopTitles: map[string]string{
			"ended":   "Подписка закончилась",
			"spent":   "Трафик закончился",
			"devices": "Все места заняты",
		},
		stopActions: map[string]string{
			"ended":   "Продлить подписку",
			"spent":   "Докупить трафик",
			"devices": "Добавить устройство",
		},
		remindToday:     "Подписка закончится сегодня",
		remindTomorrow:  "Подписка закончится завтра",
		remindEnded:     "Подписка закончилась",
		remindSoonHint:  "Продлите её, чтобы VPN работал без перерыва",
		remindEndedHint: "Продлите её, чтобы VPN снова заработал",
		remindSoon:      [3]string{"Подписка закончится через %d день", "Подписка закончится через %d дня", "Подписка закончится через %d дней"},
	},
	En: {
		connect:         "Turn VPN on",
		disconnect:      "Turn VPN off",
		stop:            "Stop connecting",
		addSubscription: "Add subscription",
		renew:           "Renew subscription",
		manage:          "Manage subscription",
		stale:           "Parts of the app are different versions. Update Spofy",
		lost:            "The Spofy service isn’t responding",
		quit:            "Quit",
		quitOff:         "Turn VPN off and quit",
		open:            "Open Spofy",
		failure:         "Error: ",
		stopped:         "Subscription isn’t working",
		titles: map[string]string{
			"idle":       "VPN is off",
			"measuring":  "Choosing a server",
			"connecting": "Connecting",
			"connected":  "VPN is on",
			"switching":  "Switching server",
			"stalled":    "Server not responding",
			"offline":    "No internet",
			"failed":     "Not connected",
		},
		hints: map[string]string{
			"stalled": "Nothing leaks past the VPN. Trying again",
		},
		reasons: map[string]string{
			"no_server":          "No server responded. Check your internet",
			"server_unreachable": "This server isn’t responding. Choose another one or “Auto”",
			"server_not_found":   "This server is no longer in the subscription",
			"no_subscription":    "Add a subscription first",
			"sub_bad_url":        "This isn’t a subscription link or a key. A link starts with https://, a key with vless://, trojan:// or ss://",
			"sub_unknown":        "This subscription no longer exists",
			"sub_unreachable":    "The subscription server isn’t responding. Check your internet",
			"sub_status":         "The link wasn’t accepted. Copy it again",
			"sub_format":         "There’s no Spofy subscription at this link",
			"sub_ended":          "Subscription has ended",
			"sub_spent":          "Out of data",
			"sub_devices":        "All device slots are taken",
			"sub_other":          "Subscription isn’t working",
			"store_failed":       "Couldn’t save data on this device",
			"connected":          "Speed test works when the VPN is off",
			"update_none":        "You have the latest version",
			"update_unreachable": "The update server isn’t responding. Try again later",
			"update_broken":      "The update file failed the signature check and won’t be installed",
			"update_file":        "The update file didn’t download completely. Try again",
			"update_busy":        "The update is already downloading",
			"update_off":         "Updates are turned off in this build",
			"update_install":     "Couldn’t open the installer. Allow Spofy to install apps",
			"vpn_denied":         "VPN permission is needed. Press again",
			"guard_failed":       "VPN permission is needed. Press again",
			"tun_permission":     "No permission to create the tunnel",
		},
		stopTitles: map[string]string{
			"ended":   "Subscription has ended",
			"spent":   "Out of data",
			"devices": "All device slots are taken",
		},
		stopActions: map[string]string{
			"ended":   "Renew subscription",
			"spent":   "Buy more data",
			"devices": "Add a device",
		},
		remindToday:     "Your subscription ends today",
		remindTomorrow:  "Your subscription ends tomorrow",
		remindEnded:     "Your subscription has ended",
		remindSoonHint:  "Renew it to keep the VPN running without a break",
		remindEndedHint: "Renew it to turn the VPN back on",
		remindSoon:      [3]string{"Your subscription ends in %d day", "Your subscription ends in %d days", "Your subscription ends in %d days"},
	},
}

func (l Lang) Remind(r Reminder) Note {
	w := texts[l]
	switch {
	case r.Ended:
		return Note{Title: w.remindEnded, Text: w.remindEndedHint}
	case r.Left == 0:
		return Note{Title: w.remindToday, Text: w.remindSoonHint}
	case r.Left == 1:
		return Note{Title: w.remindTomorrow, Text: w.remindSoonHint}
	}
	return Note{Title: fmt.Sprintf(w.remindSoon[l.form(r.Left)], r.Left), Text: w.remindSoonHint}
}

func (l Lang) form(n int) int {
	switch {
	case l == En && n == 1, l == Ru && n%10 == 1 && n%100 != 11:
		return 0
	case l == En, n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return 1
	}
	return 2
}

func (l Lang) Title(state string) string {
	return texts[l].titles[state]
}

func (l Lang) Hint(state string) string {
	return texts[l].hints[state]
}

func (l Lang) Reason(code string) string {
	if code == "" {
		return ""
	}
	if r, ok := texts[l].reasons[code]; ok {
		return r
	}
	return texts[l].failure + code
}

func (l Lang) StopTitle(stop string) string {
	if t, ok := texts[l].stopTitles[stop]; ok {
		return t
	}
	return texts[l].stopped
}

func (l Lang) StopAction(stop string) string {
	if a, ok := texts[l].stopActions[stop]; ok {
		return a
	}
	return texts[l].manage
}
