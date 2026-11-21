import 'dart:ui' show Locale;

import '../core/protocol.dart';
import '../state/apps.dart';
import '../state/model.dart';

const languageKey = 'language';

enum Lang { ru, en }

const _russian = {'ru', 'uk', 'be', 'kk'};

Lang langFor(Object? setting, List<Locale> system) {
  if (setting == 'ru') {
    return Lang.ru;
  }
  if (setting == 'en') {
    return Lang.en;
  }
  for (final locale in system) {
    final code = locale.languageCode.toLowerCase();
    if (_russian.contains(code)) {
      return Lang.ru;
    }
    if (code == 'en') {
      return Lang.en;
    }
  }
  return Lang.en;
}

abstract final class S {
  static Lang _lang = Lang.ru;

  static Lang get lang => _lang;

  static void use(Lang lang) => _lang = lang;

  static String _t(String ru, String en) => _lang == Lang.en ? en : ru;

  static const app = 'Spofy';
  static String get auto => _t('Авто', 'Auto');
  static String get autoHint => _t('Самый быстрый сервер', 'Fastest server');
  static String get server => _t('Сервер', 'Server');
  static String get servers => _t('Серверы', 'Servers');
  static String get settings => _t('Настройки', 'Settings');
  static String get autoConnect => _t('VPN при запуске', 'Turn VPN on at launch');
  static String get killSwitch => _t('Без VPN — без сети', 'No VPN, no internet');
  static String get alwaysOn => _t('Постоянная VPN', 'Always-on VPN');
  static String get alwaysOnHint =>
      _t('VPN включается сама. Задаётся в настройках Android', 'The VPN starts by itself. Set in Android settings');
  static String get killSwitchHint => _t(
    'Если VPN оборвётся, интернет остановится, а не пойдёт напрямую',
    'If the VPN drops, the internet stops instead of going around it',
  );
  static String get killSwitchProxy => _t(
    'Не действует в режиме прокси: программы без прокси идут напрямую',
    'Doesn’t apply in proxy mode: programs without the proxy go direct',
  );

  static String version(String number) => _t('Версия $number', 'Version $number');
  static String get apps => _t('Приложения', 'Apps');
  static String get appsAll => _t('Все', 'All');
  static String get appsExcept => _t('Все, кроме', 'All except');
  static String get appsOnly => _t('Только', 'Only');
  static String get appsSearch => _t('Поиск', 'Search');
  static String get appsClear => _t('Очистить поиск', 'Clear search');
  static String get appsNone => _t('Ничего не найдено', 'Nothing found');
  static String get appsFailed => _t('Не удалось получить список приложений', 'Couldn’t get the list of apps');
  static String get appsLocked => _t(
    'В Android включено «Блокировать соединения без VPN». Приложения, которые работают без VPN, останутся без интернета',
    'Android has “Block connections without VPN” turned on. Apps that work outside the VPN will have no internet',
  );
  static String get appsOnlyEmpty => _t(
    'Отметьте хотя бы одно приложение. Пока действует прежний выбор',
    'Select at least one app. Until then, the previous choice stays',
  );

  static String appsHint(AppsMode mode) => switch (mode) {
    AppsMode.all => _t('Все приложения работают через VPN', 'All apps go through the VPN'),
    AppsMode.except => _t('Отмеченные приложения работают без VPN', 'Selected apps bypass the VPN'),
    AppsMode.only => _t('Через VPN работают только отмеченные приложения', 'Only selected apps go through the VPN'),
  };

  static String appsSummary(AppsChoice choice) => switch (choice.mode) {
    AppsMode.except when choice.list.isNotEmpty => _t(
      'Без VPN: ${choice.list.length}',
      'Outside the VPN: ${choice.list.length}',
    ),
    AppsMode.only when choice.list.isNotEmpty => _t(
      'Только через VPN: ${choice.list.length}',
      'Only through the VPN: ${choice.list.length}',
    ),
    _ => _t('Все через VPN', 'All through the VPN'),
  };
  static String get subscription => _t('Подписка', 'Subscription');
  static String get addSubscription => _t('Добавить подписку', 'Add subscription');
  static String get noSubscriptionTitle => _t('Добавьте подписку', 'Add your subscription');
  static String get copyLink => _t(
    'Скопируйте ссылку в\u00A0боте или в\u00A0кабинете\u00A0Spofy',
    'Copy the link in the Spofy bot or your Spofy\u00A0account',
  );
  static String get noSubscriptionHint => _t(
    'Скопируйте ссылку в боте или в кабинете Spofy и вставьте её здесь',
    'Copy the link in the Spofy bot or your Spofy account and paste it here',
  );
  static String get pasteLink => _t('Вставить ссылку', 'Paste link');
  static String get manual => _t('Ввести вручную', 'Enter manually');
  static String get buyAsk => _t('Нет подписки?', 'No subscription?');
  static String get buy => _t('Купить в боте Spofy', 'Buy in the Spofy bot');
  static String get stuckAsk => _t('Не получается?', 'Not working?');
  static String get stuck => _t('Напишите в поддержку', 'Contact support');
  static String get pinned => _t('Выбран вручную', 'Chosen manually');
  static String get removeShort => _t('Удалить', 'Remove');
  static String get noLink => _t(
    'В буфере нет ссылки. Скопируйте её в\u00A0боте или в\u00A0кабинете\u00A0Spofy',
    'There’s no link on the clipboard. Copy it in the Spofy bot or your Spofy\u00A0account',
  );
  static String get linkHint => _t(
    'Ссылка на подписку или ключи vless://, vmess://, trojan://, ss://, hy2://',
    'Subscription link or vless://, vmess://, trojan://, ss://, hy2:// keys',
  );
  static String get paste => _t('Вставить из буфера', 'Paste from clipboard');
  static String get add => _t('Добавить', 'Add');
  static String get refresh => _t('Обновить', 'Refresh');
  static String get checkAgain => _t('Проверить ещё раз', 'Check again');
  static String get renew => _t('Продлить подписку', 'Renew subscription');
  static String get manage => _t('Управление подпиской', 'Manage subscription');

  static String stopTitle(Subscription s) => switch (s.stop) {
    'ended' => _t('Подписка закончилась', 'Subscription has ended'),
    'spent' => _t('Трафик закончился', 'Out of data'),
    'devices' => _t('Все места заняты', 'All device slots are taken'),
    _ => _t('Подписка не работает', 'Subscription isn’t working'),
  };

  static String stopHint(Subscription s) => switch (s.stop) {
    'ended' => _t(
      'VPN выключен до продления. Оплата по СБП или картой',
      'The VPN is off until you renew. Pay by SBP or card',
    ),
    'spent' => _t('Докупите трафик, чтобы продолжить пользоваться VPN', 'Buy more data to keep using the VPN'),
    'devices' => _t(
      'Это устройство не подключится, пока вы не отключите одно из своих или не добавите место',
      'This device can’t connect until you remove one of your devices or add a slot',
    ),
    _ => _said(s.notes),
  };

  static String stopAction(Subscription s) => switch (s.stop) {
    'ended' => renew,
    'spent' => _t('Докупить трафик', 'Buy more data'),
    'devices' => _t('Добавить устройство', 'Add a device'),
    _ => manage,
  };

  static String pageAction(Subscription s) =>
      s.stopped ? stopAction(s) : (lifetime(s) || s.expire <= 0 ? manage : renew);

  static String _said(List<String> notes) {
    final lines = said(notes).take(3);
    return lines.isEmpty
        ? _t('Причина — на странице подписки', 'The reason is on the subscription page')
        : lines.join('. ');
  }

  static List<String> said(List<String> notes) =>
      [for (final note in notes) note.replaceFirst(RegExp(r'^[^\p{L}\p{N}@«"(]+', unicode: true), '').trim()]
          .where((line) => line.isNotEmpty)
          .toList();

  static String get remove => _t('Удалить подписку', 'Remove subscription');
  static String get removeConfirm =>
      _t('Удалить подписку с этого устройства?', 'Remove the subscription from this device?');
  static String get cancel => _t('Отмена', 'Cancel');
  static String get apply => _t('Применить', 'Apply');
  static String get reconnectAsk => _t(
    'VPN переподключится на секунду, чтобы применить настройку',
    'The VPN reconnects for a moment to apply this setting',
  );
  static String get offeredHint =>
      _t('Ссылка получена. Проверьте её и нажмите «Добавить»', 'Link received. Check it and press “Add”');
  static String get replaceConfirm => _t('Добавить подписку по ссылке?', 'Add the subscription from this link?');
  static String get replace => _t('Добавить', 'Add');
  static String get addAnother => _t('Добавить ещё подписку', 'Add another subscription');
  static String get subscriptions => _t('Подписки', 'Subscriptions');
  static String get ownKeys => _t('Свои ключи', 'My keys');
  static String get switchHint =>
      _t('VPN переключится на выбранную подписку', 'The VPN will switch to the subscription you choose');
  static String get chosenSubscription => _t('Выбранная', 'Current');

  static String replaceFrom(String address) =>
      _t('Новая ссылка: ${Uri.tryParse(address)?.host ?? ''}', 'New link: ${Uri.tryParse(address)?.host ?? ''}');

  static String briefTitle(Brief b) => b.title.trim().isNotEmpty ? b.title.trim() : (b.links ? ownKeys : subscription);

  static String serversCount(int n) =>
      _t('$n ${plural(n, 'сервер', 'сервера', 'серверов')}', _count(n, 'server', 'servers'));

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

  static String get support => _t('Поддержка', 'Support');
  static String get connection => _t('Подключение', 'Connection');
  static String get look => _t('Вид', 'Appearance');
  static String get network => _t('Сеть', 'Network');
  static String get advanced => _t('Дополнительно', 'Advanced');
  static String get mode => _t('Режим работы', 'Connection mode');
  static String get modeHint => _t(
    'Прокси не создаёт сетевой адаптер. Выберите его, если адаптер VPN мешает другой программе или рабочей сети',
    'The proxy adds no network adapter. Choose it if the VPN adapter gets in the way of another program or a work network',
  );
  static String get modeVpn => _t('VPN (весь трафик)', 'VPN (all traffic)');
  static String get modeVpnHint =>
      _t('Через сервер идут все программы и игры', 'Every program and game goes through the server');
  static String get modeProxy => _t('Системный прокси', 'System proxy');
  static String get modeProxyHint => _t(
    'Через сервер идут браузеры и программы, которые используют прокси Windows. Остальные — напрямую',
    'Browsers and programs that use the Windows proxy go through the server. The rest go direct',
  );
  static String proxyAt(int port) => '$modeProxy · 127.0.0.1:$port';
  static String get tunnelProxy => _t(
    'В режиме прокси действует только фрагментация. Домашняя сеть всегда идёт напрямую, DNS и IPv6 — настройки режима VPN',
    'In proxy mode only fragmentation applies. The local network always goes direct; DNS and IPv6 belong to the VPN mode',
  );

  static String _lower(String s) =>
      s.length > 1 && s[1] == s[1].toLowerCase() ? s[0].toLowerCase() + s.substring(1) : s;
  static String advancedAll(Iterable<String> parts) => [parts.first, ...parts.skip(1).map(_lower)].join(', ');
  static String advancedChanged(Iterable<String> parts) => _t('Изменено: ', 'Changed: ') + parts.map(_lower).join(', ');
  static String get notices => _t('Уведомления', 'Notifications');
  static String get remind => _t('Напоминание о подписке', 'Subscription reminder');
  static String get remindHint => _t(
    'Уведомление перед концом подписки и в день, когда она закончится',
    'A notice before the subscription ends and on the day it does',
  );
  static String remindBefore(int n) =>
      n == 1 ? _t('За день', 'A day before') : _t('За ${days(n)}', '${days(n)} before');
  static String get remindOff => _t('Не напоминать', 'Don’t remind');
  static String get theme => _t('Тема', 'Theme');
  static String get themeSystem => _t('Системная', 'System');
  static String get themeLight => _t('Светлая', 'Light');
  static String get themeDark => _t('Тёмная', 'Dark');

  static String themeNow(bool dark) =>
      _t('Как в системе: ${dark ? 'тёмная' : 'светлая'}', 'Follows the system: ${dark ? 'dark' : 'light'}');
  static String get language => _t('Язык', 'Language');
  static String get languageSystem => _t('Системный', 'System');
  static const russian = 'Русский';
  static const english = 'English';

  static String languageNow(Lang lang) =>
      _t('Как в системе: ', 'Follows the system: ') + (lang == Lang.en ? english : russian);
  static String get checking => _t('Проверка серверов', 'Server check');
  static String get probeTarget => _t('Адрес проверки', 'Check address');
  static String get probeAuto => _t('Google и Cloudflare', 'Google and Cloudflare');
  static String get probeHint => _t(
    'По нему приложение проверяет, отвечает ли сервер. Смените, если все серверы недоступны, а интернет есть',
    'The app opens it to check that a server answers. Change it if every server looks down but the internet works',
  );
  static String get probeAutoHint =>
      _t('Ответ первого из двух. Подходит почти всем', 'Whichever answers first. Works for almost everyone');
  static String get probeOwn => _t('Свой адрес', 'Custom address');
  static String get probeOwnHint =>
      _t('Адрес, который отвечает кодом 204 или 200', 'An address that answers with code 204 or 200');
  static String get probeWait => _t('Ждать ответа', 'Wait for an answer');
  static String get probeWaitHint => _t(
    'Сколько ждать ответа сервера. Больше — для медленной связи, но проверка дольше',
    'How long to wait for a server. More suits a slow connection, but checks take longer',
  );

  static String seconds(int n) => _t('$n с', '$n s');
  static String get tunnel => _t('Туннель', 'Tunnel');
  static String get fragment => _t('Фрагментация', 'Fragmentation');
  static String get fragmentHint => _t(
    'Включите, если VPN не подключается у вашего оператора. Делит начало соединения на две части, чтобы его не узнали. На скорость не влияет',
    'Turn on if the VPN won’t connect with your provider. Splits the start of the connection in two so it isn’t recognised. Doesn’t affect speed',
  );
  static const dns = 'DNS';
  static String get dnsDefault => _t('Как в подписке', 'From the subscription');
  static String get dnsHint => _t(
    'Находит сайты по имени. Смените, если с VPN сайты не открываются',
    'Finds sites by name. Change it if sites don’t open with the VPN on',
  );
  static String get dnsDefaultHint => _t('Подходит почти всем', 'Works for almost everyone');
  static String get dnsOwn => _t('Свой DNS', 'Custom DNS');
  static String get dnsOwnHint =>
      _t('Адрес IPv4 или IPv6, например 1.1.1.1', 'An IPv4 or IPv6 address, such as 1.1.1.1');
  static String get dnsSafe => _t('без вредоносных сайтов', 'blocks malicious sites');
  static String get dnsClean => _t('без рекламы', 'blocks ads');
  static const ipv6 = 'IPv6';
  static String get ipv6Hint => _t('Выключите, если сайты открываются долго', 'Turn it off if sites take long to open');
  static String get ipv6Auto => _t('Авто', 'Auto');
  static String get ipv6AutoHint => _t('Как в настройках подписки', 'As the subscription sets it');
  static String get ipv6Off => _t('Выключен', 'Off');
  static String get ipv6OffHint => _t('Только IPv4: сайты не ждут IPv6', 'IPv4 only: sites do not wait for IPv6');
  static String get lan => _t('Домашняя сеть', 'Local network');
  static String get lanHint => _t(
    'Включите, если с VPN не видно принтер, телевизор или роутер',
    'Turn on if your printer, TV or router can’t be reached with the VPN on',
  );
  static String get save => _t('Сохранить', 'Save');
  static String get badValue => _t('Такое значение не подходит', 'This value won’t work');
  static String get logs => _t('Журнал', 'Log');
  static String get logsHint => _t(
    'Что делало приложение. Без ключей, ссылок и адресов серверов',
    'What the app did. No keys, links or server addresses',
  );
  static String get logsDetailed => _t('Подробный журнал', 'Detailed log');
  static String get logsDetailedHint => _t(
    'Записывает и адреса сайтов. Включайте, только чтобы найти проблему',
    'Also records website addresses. Turn it on only to track down a problem',
  );
  static String get logsEmpty => _t('Записей пока нет', 'No entries yet');
  static String get copy => _t('Скопировать', 'Copy');
  static String get clear => _t('Очистить', 'Clear');
  static String get about => _t('О приложении', 'About');
  static String get engine => _t('Ядро', 'Core');
  static String get device => _t('Устройство', 'Device');
  static String get sourceCode => _t('Исходный код', 'Source code');
  static String get licenses => _t('Лицензии', 'Licenses');
  static String get licensesHint => _t('Открытые библиотеки внутри приложения', 'Open-source libraries inside the app');
  static String get coreSource => _t('Исходный код ядра в Spofy', 'Source code of the core in Spofy');
  static String get coreModules => _t('Модули ядра', 'Core modules');
  static String get coreModulesHint => _t('Список и версии', 'List and versions');
  static String get search => _t('Поиск страны или сервера', 'Search for a country or server');
  static String get nothingFound => _t('Ничего не найдено', 'Nothing found');

  static String best(int ms) => ms > 0 ? _t('лучший ${delay(ms)}', 'best ${delay(ms)}') : '';

  static const _countries = {
    'AE': ('ОАЭ', 'UAE'),
    'AL': ('Албания', 'Albania'),
    'AM': ('Армения', 'Armenia'),
    'AR': ('Аргентина', 'Argentina'),
    'AT': ('Австрия', 'Austria'),
    'AU': ('Австралия', 'Australia'),
    'AZ': ('Азербайджан', 'Azerbaijan'),
    'BA': ('Босния и Герцеговина', 'Bosnia and Herzegovina'),
    'BE': ('Бельгия', 'Belgium'),
    'BG': ('Болгария', 'Bulgaria'),
    'BR': ('Бразилия', 'Brazil'),
    'BY': ('Беларусь', 'Belarus'),
    'CA': ('Канада', 'Canada'),
    'CH': ('Швейцария', 'Switzerland'),
    'CL': ('Чили', 'Chile'),
    'CN': ('Китай', 'China'),
    'CO': ('Колумбия', 'Colombia'),
    'CY': ('Кипр', 'Cyprus'),
    'CZ': ('Чехия', 'Czechia'),
    'DE': ('Германия', 'Germany'),
    'DK': ('Дания', 'Denmark'),
    'EE': ('Эстония', 'Estonia'),
    'EG': ('Египет', 'Egypt'),
    'ES': ('Испания', 'Spain'),
    'FI': ('Финляндия', 'Finland'),
    'FR': ('Франция', 'France'),
    'GB': ('Великобритания', 'United Kingdom'),
    'GE': ('Грузия', 'Georgia'),
    'GR': ('Греция', 'Greece'),
    'HK': ('Гонконг', 'Hong Kong'),
    'HR': ('Хорватия', 'Croatia'),
    'HU': ('Венгрия', 'Hungary'),
    'ID': ('Индонезия', 'Indonesia'),
    'IE': ('Ирландия', 'Ireland'),
    'IL': ('Израиль', 'Israel'),
    'IN': ('Индия', 'India'),
    'IS': ('Исландия', 'Iceland'),
    'IT': ('Италия', 'Italy'),
    'JP': ('Япония', 'Japan'),
    'KG': ('Киргизия', 'Kyrgyzstan'),
    'KR': ('Южная Корея', 'South Korea'),
    'KZ': ('Казахстан', 'Kazakhstan'),
    'LT': ('Литва', 'Lithuania'),
    'LU': ('Люксембург', 'Luxembourg'),
    'LV': ('Латвия', 'Latvia'),
    'MD': ('Молдова', 'Moldova'),
    'ME': ('Черногория', 'Montenegro'),
    'MK': ('Северная Македония', 'North Macedonia'),
    'MN': ('Монголия', 'Mongolia'),
    'MT': ('Мальта', 'Malta'),
    'MX': ('Мексика', 'Mexico'),
    'MY': ('Малайзия', 'Malaysia'),
    'NL': ('Нидерланды', 'Netherlands'),
    'NO': ('Норвегия', 'Norway'),
    'NZ': ('Новая Зеландия', 'New Zealand'),
    'PE': ('Перу', 'Peru'),
    'PH': ('Филиппины', 'Philippines'),
    'PL': ('Польша', 'Poland'),
    'PT': ('Португалия', 'Portugal'),
    'QA': ('Катар', 'Qatar'),
    'RO': ('Румыния', 'Romania'),
    'RS': ('Сербия', 'Serbia'),
    'RU': ('Россия', 'Russia'),
    'SA': ('Саудовская Аравия', 'Saudi Arabia'),
    'SE': ('Швеция', 'Sweden'),
    'SG': ('Сингапур', 'Singapore'),
    'SI': ('Словения', 'Slovenia'),
    'SK': ('Словакия', 'Slovakia'),
    'TH': ('Таиланд', 'Thailand'),
    'TJ': ('Таджикистан', 'Tajikistan'),
    'TR': ('Турция', 'Türkiye'),
    'TW': ('Тайвань', 'Taiwan'),
    'UA': ('Украина', 'Ukraine'),
    'UK': ('Великобритания', 'United Kingdom'),
    'US': ('США', 'USA'),
    'UZ': ('Узбекистан', 'Uzbekistan'),
    'VN': ('Вьетнам', 'Vietnam'),
    'ZA': ('ЮАР', 'South Africa'),
  };

  static String country(String code) => switch (_countries[code.toUpperCase()]) {
    (final ru, final en) => _t(ru, en),
    null => code.toUpperCase(),
  };

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
    return days > 0 ? _t('$days д $clock', '${days}d $clock') : clock;
  }

  static String get uptimeLabel => _t('VPN работает', 'VPN on for');
  static String get update => _t('Обновление', 'Update');
  static String get updateNow => _t('Обновить', 'Update');
  static String get updateLater => _t('Позже', 'Later');
  static String get updateInstall => _t('Установить', 'Install');
  static String get updateCheck => _t('Проверить обновления', 'Check for updates');
  static String get updateChecking => _t('Проверяем', 'Checking');
  static String get updateLatest => _t('У вас последняя версия', 'You have the latest version');
  static String get updateSigned => _t(
    'Файл проверяется по подписи Spofy, прежде чем его установить',
    'The file is checked against the Spofy signature before it’s installed',
  );
  static String get updateRequired => _t(
    'Эта версия больше не поддерживается. Обновите приложение, чтобы VPN работал',
    'This version is no longer supported. Update the app to keep the VPN working',
  );

  static String get updateVpn => _t('На время установки VPN выключится', 'The VPN turns off while the update installs');

  static String release(String number, int build) =>
      build > 0 ? _t('Версия $number, сборка $build', 'Version $number, build $build') : version(number);

  static String licenseCount(int n) =>
      _t('$n ${plural(n, 'лицензия', 'лицензии', 'лицензий')}', _count(n, 'license', 'licenses'));

  static String updateTitle(Update u) => _t('Вышла версия ${u.version}', 'Version ${u.version} is out');

  static String updateBanner(Update u) => _t('Доступна версия ${u.version}', 'Version ${u.version} is available');

  static String updateProgress(Update u) => u.ready
      ? _t('Скачано и проверено', 'Downloaded and verified')
      : (u.done > 0
            ? _t('Скачиваем ${(u.share * 100).floor()} %', 'Downloading ${(u.share * 100).floor()}%')
            : bytes(u.size));
  static String get supportHint => _t(
    'Если что-то не работает, скопируйте отчёт и отправьте его в поддержку. Ссылки на подписку и адресов серверов в нём нет',
    'If something isn’t working, copy the report and send it to support. It has no subscription link or server addresses',
  );
  static String get copyReport => _t('Скопировать отчёт', 'Copy report');
  static String get copied => _t('Скопировано', 'Copied');
  static String get writeSupport => _t('Написать в поддержку', 'Contact support');
  static String get reportDevice => _t('Устройство', 'Device');
  static String get reportJournal => _t('Журнал, UTC', 'Log, UTC');

  static String yes(bool on) => on ? _t('да', 'yes') : _t('нет', 'no');
  static String get measure => _t('Проверить скорость', 'Test speed');
  static String get measuring => _t('Проверяем серверы', 'Testing servers');
  static String get forever => _t('Навсегда', 'Forever');
  static String get unlimited => _t('Без лимита', 'Unlimited');
  static String get traffic => _t('Трафик', 'Data');
  static String get validUntil => _t('Действует', 'Valid');
  static String get endedOn => _t('Закончилась', 'Ended');
  static String get updated => _t('Обновлено', 'Updated');
  static String get back => _t('Назад', 'Back');
  static const noDelay = '—';
  static String get silent => _t('Не отвечает', 'Not responding');
  static String get connect => _t('Включить VPN', 'Turn VPN on');
  static String get disconnect => _t('Выключить VPN', 'Turn VPN off');
  static String get stop => _t('Остановить подключение', 'Stop connecting');
  static String get coreClosed => _t(
    'Приложение потеряло связь со своим ядром. Перезапустите Spofy',
    'The app lost contact with its core. Restart Spofy',
  );
  static String get coreProtocol =>
      _t('Части приложения разных версий. Обновите Spofy', 'Parts of the app are different versions. Update Spofy');

  static const _months = [
    ('января', 'January'),
    ('февраля', 'February'),
    ('марта', 'March'),
    ('апреля', 'April'),
    ('мая', 'May'),
    ('июня', 'June'),
    ('июля', 'July'),
    ('августа', 'August'),
    ('сентября', 'September'),
    ('октября', 'October'),
    ('ноября', 'November'),
    ('декабря', 'December'),
  ];

  static const _short = [
    ('янв', 'Jan'),
    ('фев', 'Feb'),
    ('мар', 'Mar'),
    ('апр', 'Apr'),
    ('мая', 'May'),
    ('июн', 'Jun'),
    ('июл', 'Jul'),
    ('авг', 'Aug'),
    ('сен', 'Sep'),
    ('окт', 'Oct'),
    ('ноя', 'Nov'),
    ('дек', 'Dec'),
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

  static String _count(int n, String one, String other) => '$n ${n.abs() == 1 ? one : other}';

  static String days(int n) => _t('$n ${plural(n, 'день', 'дня', 'дней')}', _count(n, 'day', 'days'));

  static String fetched(int at, DateTime now) {
    if (at <= 0) return '';
    final ago = now.difference(DateTime.fromMillisecondsSinceEpoch(at * 1000));
    if (ago.inMinutes < 1) return _t('Обновлено только что', 'Updated just now');
    if (ago.inHours < 1) return _t('Обновлено ${ago.inMinutes} мин назад', 'Updated ${ago.inMinutes} min ago');
    if (ago.inDays < 1) return _t('Обновлено ${ago.inHours} ч назад', 'Updated ${ago.inHours} h ago');
    return _t('Обновлено ${days(ago.inDays)} назад', 'Updated ${days(ago.inDays)} ago');
  }

  static String delay(int ms) => ms > 0 ? _t('$ms мс', '$ms ms') : noDelay;
  static String answer(Server s) => s.silent ? silent : delay(s.delayMs);

  static List<String> get _units =>
      _lang == Lang.en ? const ['B', 'KB', 'MB', 'GB', 'TB'] : const ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ'];

  static String _decimal(double amount) {
    final text = amount >= 100 || amount == amount.roundToDouble()
        ? amount.round().toString()
        : amount.toStringAsFixed(1);
    return (text.endsWith('.0') ? text.substring(0, text.length - 2) : text).replaceAll('.', _t(',', '.'));
  }

  static String bytes(int value) {
    final units = _units;
    var amount = value.toDouble();
    var unit = 0;
    while (amount >= 1024 && unit < units.length - 1) {
      amount /= 1024;
      unit++;
    }
    return '${_decimal(unit == 0 ? amount.roundToDouble() : amount)} ${units[unit]}';
  }

  static String _day(DateTime day, DateTime now, List<(String, String)> months) {
    final (ru, en) = months[day.month - 1];
    final text = '${day.day} ${_t(ru, en)}';
    return day.year == now.year ? text : '$text ${day.year}';
  }

  static String date(DateTime day, DateTime now) => _day(day, now, _months);

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
      return _t('Закончилась $until', 'Ended on $until');
    }
    if (left == 0) {
      return _t('Заканчивается сегодня', 'Ends today');
    }
    if (left <= 7) {
      return _t('Ещё ${days(left)}', '${days(left)} left');
    }
    return _t('Ещё ${days(left)} · до $until', '${days(left)} left · until $until');
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
      return s.used > 0 ? _t('${bytes(s.used)}, без лимита', '${bytes(s.used)}, unlimited') : unlimited;
    }
    final total = bytes(s.total);
    final unit = _units.indexOf(total.substring(total.indexOf(' ') + 1));
    var scale = 1.0;
    for (var i = 0; i < unit; i++) {
      scale *= 1024;
    }
    return _t('${_decimal(s.used / scale)} из $total', '${_decimal(s.used / scale)} of $total');
  }

  static bool endless(Subscription s) => !s.limited && (s.expire <= 0 || lifetime(s));

  static String plan(Subscription s) {
    if (!endless(s)) {
      return s.limited ? usage(s) : unlimited;
    }
    final used = s.used > 0 ? ' · ${bytes(s.used)}' : '';
    return _t('Навсегда, без лимита$used', 'Forever, unlimited$used');
  }

  static String rate(int value) => _t('${bytes(value)}/с', '${bytes(value)}/s');

  static String speed(Traffic t) => _t(
    'Скорость: загрузка ${rate(t.down)}, отдача ${rate(t.up)}',
    'Speed: download ${rate(t.down)}, upload ${rate(t.up)}',
  );

  static String day(Subscription s, DateTime now) =>
      _day(DateTime.fromMillisecondsSinceEpoch(s.expire * 1000), now, _short);

  static String until(Subscription s, DateTime now) {
    if (s.expire <= 0) {
      return '';
    }
    if (lifetime(s)) {
      return _t('навсегда', 'forever');
    }
    final left = daysLeft(s, now);
    if (left < 0) {
      return _t('закончилась', 'ended');
    }
    if (left == 0) {
      return _t('до конца дня', 'ends today');
    }
    if (left <= 7) {
      return _t('ещё ${days(left)}', '${days(left)} left');
    }
    return _t('до ${day(s, now)}', 'until ${day(s, now)}');
  }

  static String serverName(Server s) {
    final label = s.label.trim();
    return label.isEmpty ? s.id : label;
  }

  static String title(VpnState state) => switch (state) {
    VpnState.idle => _t('VPN выключен', 'VPN is off'),
    VpnState.measuring => _t('Выбираем сервер', 'Choosing a server'),
    VpnState.connecting => _t('Подключаемся', 'Connecting'),
    VpnState.connected => _t('VPN работает', 'VPN is on'),
    VpnState.switching => _t('Меняем сервер', 'Switching server'),
    VpnState.stalled => _t('Сервер не отвечает', 'Server not responding'),
    VpnState.offline => _t('Нет интернета', 'No internet'),
    VpnState.failed => _t('Нет подключения', 'Not connected'),
  };

  static String hint(Connection c, {required Traffic traffic}) => switch (c.state) {
    VpnState.idle => _t('Нажмите, чтобы включить', 'Press to turn on'),
    VpnState.measuring => _t('Проверяем, какой быстрее', 'Finding the fastest one'),
    VpnState.connecting => _t('Ждём ответа сервера', 'Waiting for the server'),
    VpnState.connected =>
      traffic.down > 0 || traffic.up > 0
          ? '↓ ${bytes(traffic.down)}   ↑ ${bytes(traffic.up)}'
          : _t('Нажмите, чтобы выключить', 'Press to turn off'),
    VpnState.switching => _t('Прежний перестал отвечать', 'The previous one stopped responding'),
    VpnState.stalled => _t('Трафик не идёт мимо VPN. Пробуем снова', 'Nothing leaks past the VPN. Trying again'),
    VpnState.offline => _t('VPN включится сам, когда сеть вернётся', 'The VPN will reconnect when the network is back'),
    VpnState.failed => reason(c.code),
  };

  static String reason(String code) => switch (code) {
    'no_server' => _t('Ни один сервер не ответил. Проверьте интернет', 'No server responded. Check your internet'),
    'server_unreachable' => _t(
      'Этот сервер не отвечает. Выберите другой или «Авто»',
      'This server isn’t responding. Choose another one or “Auto”',
    ),
    'server_not_found' => _t('Такого сервера больше нет в подписке', 'This server is no longer in the subscription'),
    'no_subscription' => _t('Сначала добавьте подписку', 'Add a subscription first'),
    'sub_bad_url' => _t(
      'Это не ссылка подписки и не ключ. Ссылка начинается с https://, ключ — с vless://, trojan:// или ss://',
      'This isn’t a subscription link or a key. A link starts with https://, a key with vless://, trojan:// or ss://',
    ),
    'sub_unreachable' => _t(
      'Сервер подписки не отвечает. Проверьте\u00A0интернет',
      'The subscription server isn’t responding. Check your internet',
    ),
    'sub_status' => _t('Ссылка не принята. Скопируйте её заново', 'The link wasn’t accepted. Copy it again'),
    'sub_format' => _t('По ссылке нет подписки Spofy', 'There’s no Spofy subscription at this link'),
    'sub_ended' => _t('Подписка закончилась', 'Subscription has ended'),
    'sub_spent' => _t('Трафик закончился', 'Out of data'),
    'sub_devices' => _t('Все места заняты', 'All device slots are taken'),
    'sub_other' => _t('Подписка не работает', 'Subscription isn’t working'),
    'store_failed' => _t('Не удалось сохранить данные на устройстве', 'Couldn’t save data on this device'),
    'connected' => _t('Проверка скорости доступна, когда VPN выключен', 'Speed test works when the VPN is off'),
    'update_none' => updateLatest,
    'update_unreachable' => _t(
      'Сервер обновлений не отвечает. Попробуйте позже',
      'The update server isn’t responding. Try again later',
    ),
    'update_broken' => _t(
      'Файл обновления не прошёл проверку подписи и не будет установлен',
      'The update file failed the signature check and won’t be installed',
    ),
    'update_file' => _t(
      'Файл обновления не скачался целиком. Попробуйте ещё раз',
      'The update file didn’t download completely. Try again',
    ),
    'update_busy' => _t('Обновление уже скачивается', 'The update is already downloading'),
    'update_off' => _t('В этой сборке обновления выключены', 'Updates are turned off in this build'),
    'update_install' => _t(
      'Не удалось открыть установку. Разрешите Spofy устанавливать приложения',
      'Couldn’t open the installer. Allow Spofy to install apps',
    ),
    'sub_unknown' => _t('Такой подписки больше нет', 'This subscription no longer exists'),
    'vpn_denied' ||
    'guard_failed' => _t('Нужно разрешение на VPN. Нажмите ещё раз', 'VPN permission is needed. Press again'),
    'tun_permission' => _t('Нет прав, чтобы создать туннель', 'No permission to create the tunnel'),
    '' => '',
    _ => _t('Ошибка: $code', 'Error: $code'),
  };

  static String failure(Failure f) => reason(f.code);
}
