import '../core/protocol.dart';
import '../state/model.dart';

abstract final class S {
  static const app = 'Spofy';
  static const auto = 'Авто';
  static const autoHint = 'Самый быстрый сервер';
  static const server = 'Сервер';
  static const servers = 'Серверы';
  static const subscription = 'Подписка';
  static const addSubscription = 'Добавить подписку';
  static const noSubscriptionTitle = 'Подписки пока нет';
  static const noSubscriptionHint = 'Скопируйте ссылку в боте или в кабинете Spofy и вставьте её здесь';
  static const pasteLink = 'Вставить ссылку';
  static const manual = 'Ввести вручную';
  static const pinned = 'Выбран вручную';
  static const removeShort = 'Удалить';
  static const noLink = 'В буфере нет ссылки. Скопируйте её в боте или в кабинете Spofy';
  static const linkHint = 'Ссылка на подписку';
  static const paste = 'Вставить из буфера';
  static const add = 'Добавить';
  static const refresh = 'Обновить';
  static const remove = 'Удалить подписку';
  static const removeConfirm = 'Удалить подписку с этого устройства?';
  static const cancel = 'Отмена';
  static const support = 'Поддержка';
  static const measure = 'Проверить скорость';
  static const measuring = 'Проверяем серверы';
  static const forever = 'Навсегда';
  static const unlimited = 'Без ограничений';
  static const traffic = 'Трафик';
  static const validUntil = 'Действует';
  static const updated = 'Обновлено';
  static const back = 'Назад';
  static const noDelay = '—';
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
    if (left < 0) {
      return 'Закончилась';
    }
    if (left == 0) {
      return 'Заканчивается сегодня';
    }
    final until = date(DateTime.fromMillisecondsSinceEpoch(s.expire * 1000), now);
    return left <= 7 ? 'Ещё ${days(left)}' : 'До $until';
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
    final day = DateTime.fromMillisecondsSinceEpoch(s.expire * 1000);
    final text = 'до ${day.day} ${_short[day.month - 1]}';
    return day.year == now.year ? text : '$text ${day.year}';
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

  static String hint(Connection c, {required Traffic traffic, required bool guarded}) => switch (c.state) {
    VpnState.idle => 'Нажмите, чтобы включить',
    VpnState.measuring => 'Проверяем, какой быстрее',
    VpnState.connecting => 'Ждём ответа сервера',
    VpnState.connected =>
      traffic.down > 0 || traffic.up > 0
          ? '↓ ${bytes(traffic.down)}   ↑ ${bytes(traffic.up)}'
          : 'Нажмите, чтобы выключить',
    VpnState.switching => 'Прежний перестал отвечать',
    VpnState.stalled => guarded ? 'Трафик не идёт мимо VPN. Пробуем снова' : 'Пробуем снова',
    VpnState.offline => 'VPN включится сам, когда сеть вернётся',
    VpnState.failed => reason(c.code),
  };

  static String reason(String code) => switch (code) {
    'no_server' => 'Ни один сервер не ответил. Проверьте интернет',
    'server_unreachable' => 'Этот сервер не отвечает. Выберите другой или «Авто»',
    'server_not_found' => 'Такого сервера больше нет в подписке',
    'no_subscription' => 'Сначала добавьте подписку',
    'sub_bad_url' => 'Это не похоже на ссылку подписки. Она начинается с https://',
    'sub_unreachable' => 'Сервер подписки не отвечает. Проверьте интернет',
    'sub_status' => 'Ссылка не принята. Скопируйте её заново',
    'sub_format' => 'По ссылке нет подписки Spofy',
    'store_failed' => 'Не удалось сохранить данные на устройстве',
    'connected' => 'Проверка скорости доступна, когда VPN выключен',
    '' => '',
    _ => 'Ошибка: $code',
  };

  static String failure(Failure f) => reason(f.code);
}
