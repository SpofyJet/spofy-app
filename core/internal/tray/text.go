package tray

const (
	textApp             = "Spofy"
	textConnect         = "Включить VPN"
	textDisconnect      = "Выключить VPN"
	textStop            = "Остановить подключение"
	textAddSubscription = "Добавить подписку"
	textRenew           = "Продлить подписку"
	textManage          = "Управление подпиской"
	textStale           = "Части приложения разных версий. Обновите Spofy"
	textLost            = "Служба Spofy не отвечает"
	textQuit            = "Выйти"
	textQuitOff         = "Выключить VPN и выйти"
	TextOpen            = "Открыть Spofy"
)

var titles = map[string]string{
	"idle":       "VPN выключен",
	"measuring":  "Выбираем сервер",
	"connecting": "Подключаемся",
	"connected":  "VPN работает",
	"switching":  "Меняем сервер",
	"stalled":    "Сервер не отвечает",
	"offline":    "Нет интернета",
	"failed":     "Нет подключения",
}

var hints = map[string]string{
	"stalled": "Трафик не идёт мимо VPN. Пробуем снова",
}

var reasons = map[string]string{
	"no_server":          "Ни один сервер не ответил. Проверьте интернет",
	"server_unreachable": "Этот сервер не отвечает. Выберите другой или «Авто»",
	"server_not_found":   "Такого сервера больше нет в подписке",
	"no_subscription":    "Сначала добавьте подписку",
	"sub_bad_url":        "Это не похоже на ссылку подписки. Она начинается с https://",
	"sub_unreachable":    "Сервер подписки не отвечает. Проверьте интернет",
	"sub_status":         "Ссылка не принята. Скопируйте её заново",
	"sub_format":         "По ссылке нет подписки Spofy",
	"sub_ended":          "Подписка закончилась",
	"sub_spent":          "Трафик закончился",
	"sub_devices":        "Все места заняты",
	"sub_other":          "Подписка не работает",
	"store_failed":       "Не удалось сохранить данные на устройстве",
	"connected":          "Проверка скорости доступна, когда VPN выключен",
	"vpn_denied":         "Нужно разрешение на VPN. Нажмите ещё раз",
	"guard_failed":       "Нужно разрешение на VPN. Нажмите ещё раз",
	"tun_permission":     "Нет прав, чтобы создать туннель",
}

var stopTitles = map[string]string{
	"ended":   "Подписка закончилась",
	"spent":   "Трафик закончился",
	"devices": "Все места заняты",
}

var stopActions = map[string]string{
	"ended":   textRenew,
	"spent":   "Докупить трафик",
	"devices": "Добавить устройство",
}

func Title(state string) string {
	return titles[state]
}

func Hint(state string) string {
	return hints[state]
}

func Reason(code string) string {
	if code == "" {
		return ""
	}
	if r, ok := reasons[code]; ok {
		return r
	}
	return "Ошибка: " + code
}

func StopTitle(stop string) string {
	if t, ok := stopTitles[stop]; ok {
		return t
	}
	return "Подписка не работает"
}

func StopAction(stop string) string {
	if a, ok := stopActions[stop]; ok {
		return a
	}
	return textManage
}
