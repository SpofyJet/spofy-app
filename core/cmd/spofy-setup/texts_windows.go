package main

type texts struct {
	title, installing, removing, ask, askMore, removed string
	admin, damaged, busy, failed, rolledBack, inUse    string
	removeFailed, removeFailedMore, rolledBackMore     string
}

var ru = texts{
	title:            "Spofy",
	installing:       "Устанавливаем Spofy",
	removing:         "Удаляем Spofy",
	ask:              "Удалить Spofy с компьютера?",
	askMore:          "VPN отключится, настройки на этом компьютере сотрутся. Подписка останется в вашем аккаунте.",
	removed:          "Spofy удалён",
	admin:            "Для установки нужны права администратора",
	damaged:          "Файл установки повреждён или скачан не до конца. Скачайте его заново.",
	busy:             "Установка Spofy уже идёт",
	failed:           "Не получилось установить Spofy",
	rolledBack:       "Новая версия не запустилась",
	rolledBackMore:   "Вернули прежнюю версию, Spofy работает как раньше.",
	inUse:            "Файлы Spofy заняты другой программой. Перезагрузите компьютер и запустите установку снова.",
	removeFailed:     "Не получилось удалить Spofy",
	removeFailedMore: "VPN оставлен как был. Перезагрузите компьютер и попробуйте снова.",
}

var en = texts{
	title:            "Spofy",
	installing:       "Installing Spofy",
	removing:         "Removing Spofy",
	ask:              "Remove Spofy from this computer?",
	askMore:          "The VPN turns off and the settings on this computer are erased. The subscription stays in your account.",
	removed:          "Spofy is removed",
	admin:            "Installing needs administrator rights",
	damaged:          "The setup file is damaged or did not download completely. Download it again.",
	busy:             "Spofy is already being installed",
	failed:           "Spofy could not be installed",
	rolledBack:       "The new version did not start",
	rolledBackMore:   "The previous version is back and Spofy works as before.",
	inUse:            "Another program holds the files of Spofy. Restart the computer and run the setup again.",
	removeFailed:     "Spofy could not be removed",
	removeFailedMore: "The VPN is left as it was. Restart the computer and try again.",
}

func says() texts {
	if russian() {
		return ru
	}
	return en
}
