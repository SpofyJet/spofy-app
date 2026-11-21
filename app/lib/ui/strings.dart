import '../core/protocol.dart';
import '../state/apps.dart';
import '../state/model.dart';

abstract final class S {
  static const app = 'Spofy';
  static const auto = 'Авто';
  static const autoHint = 'Самый быстрый сервер';
  static const server = 'Сервер';
  static const servers = 'Серверы';
  static const settings = 'Настройки';
  static const autoConnect = 'Включать VPN при запуске';
  static const autoConnectHint = 'Открыли приложение — VPN уже работает';
  static const killSwitch = 'Без VPN — без интернета';
  static const alwaysOn = 'Постоянная VPN';
  static const alwaysOnHint = 'В настройках Android: VPN включается сама, без неё интернет закрыт';
  static const killSwitchHint = 'Если серверы не отвечают, интернет останавливается, а не идёт напрямую';

  static String version(String number) => 'Версия $number';
  static const apps = 'Приложения';
  static const appsAll = 'Все';
  static const appsExcept = 'Все, кроме';
  static const appsOnly = 'Только';
  static const appsSearch = 'Поиск';
  static const appsClear = 'Очистить поиск';
  static const appsNone = 'Ничего не найдено';
  static const appsFailed = 'Не удалось получить список приложений';
  static const appsLocked =
      'В Android включено «Блокировать соединения без VPN». Приложения, которые работают без VPN, останутся без интернета';
  static const appsOnlyEmpty = 'Отметьте хотя бы одно приложение. Пока действует прежний выбор';

  static String appsHint(AppsMode mode) => switch (mode) {
    AppsMode.all => 'Все приложения работают через VPN',
    AppsMode.except => 'Отмеченные приложения работают без VPN',
    AppsMode.only => 'Через VPN работают только отмеченные приложения',
  };

  static String appsSummary(AppsChoice choice) => switch (choice.mode) {
    AppsMode.except when choice.list.isNotEmpty => 'Без VPN: ${choice.list.length}',
    AppsMode.only when choice.list.isNotEmpty => 'Только через VPN: ${choice.list.length}',
    _ => 'Все через VPN',
  };
  static const subscription = 'Подписка';
  static const addSubscription = 'Добавить подписку';
  static const noSubscriptionTitle = 'Подписки пока нет';
  static const noSubscriptionHint = 'Скопируйте ссылку в боте или в кабинете Spofy и вставьте её здесь';
  static const pasteLink = 'Вставить ссылку';
  static const manual = 'Ввести вручную';
  static const buyAsk = 'Нет подписки?';
  static const buy = 'Купить в боте Spofy';
  static const stuckAsk = 'Не получается?';
  static const stuck = 'Напишите в поддержку';
  static const pinned = 'Выбран вручную';
  static const removeShort = 'Удалить';
  static const noLink = 'В буфере нет ссылки. Скопируйте её в боте или в кабинете Spofy';
  static const linkHint = 'Ссылка на подписку или ключи vless://, vmess://, trojan://, ss://, hy2://';
  static const paste = 'Вставить из буфера';
  static const add = 'Добавить';
  static const refresh = 'Обновить';
  static const checkAgain = 'Проверить ещё раз';
  static const renew = 'Продлить подписку';
  static const manage = 'Управление подпиской';

  static String stopTitle(Subscription s) => switch (s.stop) {
    'ended' => 'Подписка закончилась',
    'spent' => 'Трафик закончился',
    'devices' => 'Все места заняты',
    _ => 'Подписка не работает',
  };

  static String stopHint(Subscription s) => switch (s.stop) {
    'ended' => 'VPN выключен до продления. Оплата по СБП или картой',
    'spent' => 'Докупите трафик, чтобы продолжить пользоваться VPN',
    'devices' => 'Это устройство не подключится, пока вы не отключите одно из своих или не добавите место',
    _ => _said(s.notes),
  };

  static String stopAction(Subscription s) => switch (s.stop) {
    'ended' => renew,
    'spent' => 'Докупить трафик',
    'devices' => 'Добавить устройство',
    _ => manage,
  };

  static String pageAction(Subscription s) =>
      s.stopped ? stopAction(s) : (lifetime(s) || s.expire <= 0 ? manage : renew);

  static String _said(List<String> notes) {
    final lines = said(notes).take(3);
    return lines.isEmpty ? 'Причина — на странице подписки' : lines.join('. ');
  }

  static List<String> said(List<String> notes) =>
      [for (final note in notes) note.replaceFirst(RegExp(r'^[^\p{L}\p{N}@«"(]+', unicode: true), '').trim()]
          .where((line) => line.isNotEmpty)
          .toList();

  static const remove = 'Удалить подписку';
  static const removeConfirm = 'Удалить подписку с этого устройства?';
  static const cancel = 'Отмена';
  static const offeredHint = 'Ссылка получена. Проверьте её и нажмите «Добавить»';
  static const replaceConfirm = 'Добавить подписку по ссылке?';
  static const replace = 'Добавить';
  static const addAnother = 'Добавить ещё подписку';
  static const subscriptions = 'Подписки';
  static const ownKeys = 'Свои ключи';
  static const switchHint = 'VPN переключится на выбранную подписку';
  static const chosenSubscription = 'Выбранная';

  static String replaceFrom(String address) => 'Новая ссылка: ${Uri.tryParse(address)?.host ?? ''}';

  static String briefTitle(Brief b) => b.title.trim().isNotEmpty ? b.title.trim() : (b.links ? ownKeys : subscription);

  static String serversCount(int n) => '$n ${plural(n, 'сервер', 'сервера', 'серверов')}';

  static Subscription briefed(Brief b) => Subscription(
    title: b.title,
    upload: 0,
    download: b.used,
    total: b.total,
    expire: b.expire,
    supportUrl: '',
    webPageUrl: '',
    announce: '',
    fetchedAt: 0,
    updateEvery: 0,
    stop: b.stop,
  );

  static String briefLine(Brief b, DateTime now) {
    final s = briefed(b);
    return [
      if (s.stopped) stopTitle(s) else serversCount(b.servers),
      if (s.limited) usage(s),
      if (s.expire > 0 && !s.stopped) until(s, now),
    ].join(' · ');
  }

  static const support = 'Поддержка';
  static const connection = 'Подключение';
  static const checking = 'Проверка серверов';
  static const probeTarget = 'Адрес проверки';
  static const probeAuto = 'Google и Cloudflare';
  static const probeAutoHint = 'Ответ первого из двух. Подходит почти всем';
  static const probeOwn = 'Свой адрес';
  static const probeOwnHint = 'Адрес, который отвечает кодом 204 или 200';
  static const probeWait = 'Ждать ответа';
  static const probeWaitHint = 'Сколько ждать сервер, прежде чем считать, что он не отвечает';

  static String seconds(int n) => '$n с';
  static const tunnel = 'Туннель';
  static const fragment = 'Фрагментация';
  static const fragmentHint =
      'Делит начало защищённого соединения на мелкие части. Помогает, когда провайдер мешает подключиться. Немного замедляет первое соединение';
  static const dns = 'DNS';
  static const dnsDefault = 'Как в подписке';
  static const dnsDefaultHint = 'Настройки провайдера VPN';
  static const dnsOwn = 'Свой DNS';
  static const dnsOwnHint = 'Адрес IPv4 или IPv6, например 1.1.1.1';
  static const ipv6 = 'IPv6';
  static const ipv6Hint = 'Выключите, если сайты открываются долго: приложения сразу пойдут по IPv4';
  static const lan = 'Домашняя сеть без VPN';
  static const lanHint = 'Принтеры, телевизоры и роутер доступны при включённом VPN';
  static const ownValue = 'Ввести';
  static const save = 'Сохранить';
  static const badValue = 'Такое значение не подходит';
  static const logs = 'Журнал';
  static const logsHint = 'Что делало приложение. Адресов серверов, ключей и ссылки на подписку в нём нет';
  static const logsDetailed = 'Подробный журнал';
  static const logsDetailedHint = 'Записывает и адреса сайтов. Включайте, только чтобы найти проблему';
  static const logsEmpty = 'Записей пока нет';
  static const copy = 'Скопировать';
  static const clear = 'Очистить';
  static const about = 'О приложении';
  static const engine = 'Ядро';
  static const device = 'Устройство';
  static const sourceCode = 'Исходный код';
  static const licenses = 'Лицензии';
  static const licensesHint = 'Открытые библиотеки внутри приложения';
  static const sortSpeed = 'По скорости';
  static const sortName = 'По стране';
  static const search = 'Поиск страны или сервера';
  static const nothingFound = 'Ничего не найдено';

  static String best(int ms) => ms > 0 ? 'лучший ${delay(ms)}' : '';

  static const _countries = {
    'AE': 'ОАЭ',
    'AL': 'Албания',
    'AM': 'Армения',
    'AR': 'Аргентина',
    'AT': 'Австрия',
    'AU': 'Австралия',
    'AZ': 'Азербайджан',
    'BA': 'Босния и Герцеговина',
    'BE': 'Бельгия',
    'BG': 'Болгария',
    'BR': 'Бразилия',
    'BY': 'Беларусь',
    'CA': 'Канада',
    'CH': 'Швейцария',
    'CL': 'Чили',
    'CN': 'Китай',
    'CO': 'Колумбия',
    'CY': 'Кипр',
    'CZ': 'Чехия',
    'DE': 'Германия',
    'DK': 'Дания',
    'EE': 'Эстония',
    'EG': 'Египет',
    'ES': 'Испания',
    'FI': 'Финляндия',
    'FR': 'Франция',
    'GB': 'Великобритания',
    'GE': 'Грузия',
    'GR': 'Греция',
    'HK': 'Гонконг',
    'HR': 'Хорватия',
    'HU': 'Венгрия',
    'ID': 'Индонезия',
    'IE': 'Ирландия',
    'IL': 'Израиль',
    'IN': 'Индия',
    'IS': 'Исландия',
    'IT': 'Италия',
    'JP': 'Япония',
    'KG': 'Киргизия',
    'KR': 'Южная Корея',
    'KZ': 'Казахстан',
    'LT': 'Литва',
    'LU': 'Люксембург',
    'LV': 'Латвия',
    'MD': 'Молдова',
    'ME': 'Черногория',
    'MK': 'Северная Македония',
    'MN': 'Монголия',
    'MT': 'Мальта',
    'MX': 'Мексика',
    'MY': 'Малайзия',
    'NL': 'Нидерланды',
    'NO': 'Норвегия',
    'NZ': 'Новая Зеландия',
    'PE': 'Перу',
    'PH': 'Филиппины',
    'PL': 'Польша',
    'PT': 'Португалия',
    'QA': 'Катар',
    'RO': 'Румыния',
    'RS': 'Сербия',
    'RU': 'Россия',
    'SA': 'Саудовская Аравия',
    'SE': 'Швеция',
    'SG': 'Сингапур',
    'SI': 'Словения',
    'SK': 'Словакия',
    'TH': 'Таиланд',
    'TJ': 'Таджикистан',
    'TR': 'Турция',
    'TW': 'Тайвань',
    'UA': 'Украина',
    'UK': 'Великобритания',
    'US': 'США',
    'UZ': 'Узбекистан',
    'VN': 'Вьетнам',
    'ZA': 'ЮАР',
  };

  static String country(String code) => _countries[code.toUpperCase()] ?? code.toUpperCase();

  static String detail(Server s) {
    var text = s.label.replaceAll(RegExp(r'\(\s*[A-Za-z]{2}\s*\)'), '').trim();
    if (text.startsWith('(') && text.endsWith(')') && !text.substring(1, text.length - 1).contains(RegExp(r'[()]'))) {
      text = text.substring(1, text.length - 1).trim();
    }
    return text;
  }

  static String place(Server s) {
    final rest = detail(s);
    if (s.country.isEmpty) {
      return rest.isEmpty ? serverName(s) : rest;
    }
    final name = country(s.country);
    if (rest.isEmpty) {
      return name;
    }
    return rest.contains(RegExp(r'[\p{L}\p{N}]', unicode: true)) ? rest : '$name $rest';
  }

  static const _protocols = {
    'vless': 'VLESS',
    'vmess': 'VMess',
    'trojan': 'Trojan',
    'shadowsocks': 'Shadowsocks',
    'hysteria': 'Hysteria 2',
    'hysteria2': 'Hysteria 2',
    'wireguard': 'WireGuard',
  };

  static const _transports = {
    'xhttp': 'XHTTP',
    'splithttp': 'XHTTP',
    'ws': 'WebSocket',
    'grpc': 'gRPC',
    'httpupgrade': 'HTTPUpgrade',
    'kcp': 'mKCP',
  };

  static String via(Server s) {
    final hysteria = s.protocol.startsWith('hysteria');
    return [
      _protocols[s.protocol] ?? s.protocol.toUpperCase(),
      ?_transports[s.transport],
      if (s.security == 'reality') 'Reality' else if (s.security == 'tls' && !hysteria) 'TLS',
    ].where((p) => p.isNotEmpty).join(' · ');
  }

  static String uptime(Duration d) {
    final seconds = d.isNegative ? 0 : d.inSeconds;
    String two(int n) => n.toString().padLeft(2, '0');
    final clock = '${two(seconds ~/ 3600 % 24)}:${two(seconds ~/ 60 % 60)}:${two(seconds % 60)}';
    final days = seconds ~/ 86400;
    return days > 0 ? '$days д $clock' : clock;
  }

  static const uptimeLabel = 'VPN работает';
  static const update = 'Обновление';
  static const updateNow = 'Обновить';
  static const updateLater = 'Позже';
  static const updateInstall = 'Установить';
  static const updateCheck = 'Проверить обновления';
  static const updateChecking = 'Проверяем';
  static const updateLatest = 'У вас последняя версия';
  static const updateSigned = 'Файл проверяется по подписи Spofy, прежде чем его установить';
  static const updateRequired = 'Эта версия больше не поддерживается. Обновите приложение, чтобы VPN работал';

  static const updateVpn = 'На время установки VPN выключится';

  static String release(String number, int build) => build > 0 ? 'Версия $number, сборка $build' : version(number);

  static String licenseCount(int n) => '$n ${plural(n, 'лицензия', 'лицензии', 'лицензий')}';

  static String updateTitle(Update u) => 'Вышла версия ${u.version}';

  static String updateBanner(Update u) => 'Доступна версия ${u.version}';

  static String updateProgress(Update u) =>
      u.ready ? 'Скачано и проверено' : (u.done > 0 ? 'Скачиваем ${(u.share * 100).floor()} %' : bytes(u.size));
  static const supportHint =
      'Если что-то не работает, скопируйте отчёт и отправьте его в поддержку. Ссылки на подписку и адресов серверов в нём нет';
  static const copyReport = 'Скопировать отчёт';
  static const copied = 'Скопировано';
  static const writeSupport = 'Написать в поддержку';
  static const reportDevice = 'Устройство';
  static const reportJournal = 'Журнал, UTC';

  static String yes(bool on) => on ? 'да' : 'нет';
  static const measure = 'Проверить скорость';
  static const measuring = 'Проверяем серверы';
  static const forever = 'Навсегда';
  static const unlimited = 'Без ограничений';
  static const traffic = 'Трафик';
  static const validUntil = 'Действует';
  static const endedOn = 'Закончилась';
  static const updated = 'Обновлено';
  static const back = 'Назад';
  static const noDelay = '—';
  static const silent = 'Не отвечает';
  static const connect = 'Включить VPN';
  static const disconnect = 'Выключить VPN';
  static const stop = 'Остановить подключение';
  static const coreClosed = 'Приложение потеряло связь со своим ядром. Перезапустите Spofy';
  static const coreProtocol = 'Части приложения разных версий. Обновите Spofy';

  static const _months = [
    'января',
    'февраля',
    'марта',
    'апреля',
    'мая',
    'июня',
    'июля',
    'августа',
    'сентября',
    'октября',
    'ноября',
    'декабря',
  ];

  static String plural(int n, String one, String few, String many) {
    final tens = n.abs() % 100;
    final ones = tens % 10;
    if (tens >= 11 && tens <= 14) {
      return many;
    }
    if (ones == 1) {
      return one;
    }
    if (ones >= 2 && ones <= 4) {
      return few;
    }
    return many;
  }

  static String days(int n) => '$n ${plural(n, 'день', 'дня', 'дней')}';

  static String delay(int ms) => ms > 0 ? '$ms мс' : noDelay;
  static String answer(Server s) => s.silent ? silent : delay(s.delayMs);

  static String bytes(int value) {
    const units = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ'];
    var amount = value.toDouble();
    var unit = 0;
    while (amount >= 1024 && unit < units.length - 1) {
      amount /= 1024;
      unit++;
    }
    final text = unit == 0 || amount >= 100
        ? amount.round().toString()
        : amount.toStringAsFixed(1).replaceAll('.', ',');
    return '${text.endsWith(',0') ? text.substring(0, text.length - 2) : text} ${units[unit]}';
  }

  static String date(DateTime day, DateTime now) {
    final text = '${day.day} ${_months[day.month - 1]}';
    return day.year == now.year ? text : '$text ${day.year}';
  }

  static bool lifetime(Subscription s) => s.expire >= 4070908800;

  static int daysLeft(Subscription s, DateTime now) {
    final left = DateTime.fromMillisecondsSinceEpoch(s.expire * 1000).difference(now);
    return left.isNegative ? -1 : left.inHours ~/ 24;
  }

  static String expiry(Subscription s, DateTime now) {
    if (s.expire <= 0) {
      return '';
    }
    if (lifetime(s)) {
      return forever;
    }
    final left = daysLeft(s, now);
    final until = date(DateTime.fromMillisecondsSinceEpoch(s.expire * 1000), now);
    if (left < 0) {
      return 'Закончилась $until';
    }
    if (left == 0) {
      return 'Заканчивается сегодня';
    }
    return left <= 7 ? 'Ещё ${days(left)}' : 'Ещё ${days(left)} · до $until';
  }

  static double remainder(Subscription s, DateTime now) {
    var left = 1.0;
    if (s.limited) {
      left = 1 - s.used / s.total;
    }
    if (s.expire > 0 && !lifetime(s)) {
      final hours = DateTime.fromMillisecondsSinceEpoch(s.expire * 1000).difference(now).inHours;
      final share = hours / (30 * 24);
      if (share < left) {
        left = share;
      }
    }
    return left < 0 ? 0 : (left > 1 ? 1 : left);
  }

  static bool low(Subscription s, DateTime now) {
    final ending = s.expire > 0 && !lifetime(s) && daysLeft(s, now) <= 3;
    return ending || (s.limited && s.used >= s.total * 0.9);
  }

  static String usage(Subscription s) {
    if (!s.limited) {
      return unlimited;
    }
    final total = bytes(s.total);
    final unit = total.substring(total.indexOf(' ') + 1);
    var scale = 1.0;
    for (final name in const ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ']) {
      if (name == unit) {
        break;
      }
      scale *= 1024;
    }
    final used = s.used / scale;
    final text = used >= 100 || used == used.roundToDouble()
        ? used.round().toString()
        : used.toStringAsFixed(1).replaceAll('.', ',');
    return '${text.endsWith(',0') ? text.substring(0, text.length - 2) : text} из $total';
  }

  static const _short = ['янв', 'фев', 'мар', 'апр', 'мая', 'июн', 'июл', 'авг', 'сен', 'окт', 'ноя', 'дек'];

  static String day(Subscription s, DateTime now) {
    final day = DateTime.fromMillisecondsSinceEpoch(s.expire * 1000);
    final text = '${day.day} ${_short[day.month - 1]}';
    return day.year == now.year ? text : '$text ${day.year}';
  }

  static String until(Subscription s, DateTime now) {
    if (s.expire <= 0) {
      return '';
    }
    if (lifetime(s)) {
      return 'навсегда';
    }
    final left = daysLeft(s, now);
    if (left < 0) {
      return 'закончилась';
    }
    if (left == 0) {
      return 'до конца дня';
    }
    if (left <= 7) {
      return 'ещё ${days(left)}';
    }
    return 'до ${day(s, now)}';
  }

  static String serverName(Server s) {
    final label = s.label.trim();
    return label.isEmpty ? s.id : label;
  }

  static String title(VpnState state) => switch (state) {
    VpnState.idle => 'VPN выключен',
    VpnState.measuring => 'Выбираем сервер',
    VpnState.connecting => 'Подключаемся',
    VpnState.connected => 'VPN работает',
    VpnState.switching => 'Меняем сервер',
    VpnState.stalled => 'Сервер не отвечает',
    VpnState.offline => 'Нет интернета',
    VpnState.failed => 'Нет подключения',
  };

  static String hint(Connection c, {required Traffic traffic}) => switch (c.state) {
    VpnState.idle => 'Нажмите, чтобы включить',
    VpnState.measuring => 'Проверяем, какой быстрее',
    VpnState.connecting => 'Ждём ответа сервера',
    VpnState.connected =>
      traffic.down > 0 || traffic.up > 0
          ? '↓ ${bytes(traffic.down)}   ↑ ${bytes(traffic.up)}'
          : 'Нажмите, чтобы выключить',
    VpnState.switching => 'Прежний перестал отвечать',
    VpnState.stalled => 'Трафик не идёт мимо VPN. Пробуем снова',
    VpnState.offline => 'VPN включится сам, когда сеть вернётся',
    VpnState.failed => reason(c.code),
  };

  static String reason(String code) => switch (code) {
    'no_server' => 'Ни один сервер не ответил. Проверьте интернет',
    'server_unreachable' => 'Этот сервер не отвечает. Выберите другой или «Авто»',
    'server_not_found' => 'Такого сервера больше нет в подписке',
    'no_subscription' => 'Сначала добавьте подписку',
    'sub_bad_url' =>
      'Это не ссылка подписки и не ключ. Ссылка начинается с https://, ключ — с vless://, trojan:// или ss://',
    'sub_unreachable' => 'Сервер подписки не отвечает. Проверьте интернет',
    'sub_status' => 'Ссылка не принята. Скопируйте её заново',
    'sub_format' => 'По ссылке нет подписки Spofy',
    'sub_ended' => 'Подписка закончилась',
    'sub_spent' => 'Трафик закончился',
    'sub_devices' => 'Все места заняты',
    'sub_other' => 'Подписка не работает',
    'store_failed' => 'Не удалось сохранить данные на устройстве',
    'connected' => 'Проверка скорости доступна, когда VPN выключен',
    'update_none' => updateLatest,
    'update_unreachable' => 'Сервер обновлений не отвечает. Попробуйте позже',
    'update_broken' => 'Файл обновления не прошёл проверку подписи и не будет установлен',
    'update_file' => 'Файл обновления не скачался целиком. Попробуйте ещё раз',
    'update_busy' => 'Обновление уже скачивается',
    'update_off' => 'В этой сборке обновления выключены',
    'update_install' => 'Не удалось открыть установку. Разрешите Spofy устанавливать приложения',
    'sub_unknown' => 'Такой подписки больше нет',
    'vpn_denied' || 'guard_failed' => 'Нужно разрешение на VPN. Нажмите ещё раз',
    'tun_permission' => 'Нет прав, чтобы создать туннель',
    '' => '',
    _ => 'Ошибка: $code',
  };

  static String failure(Failure f) => reason(f.code);
}
