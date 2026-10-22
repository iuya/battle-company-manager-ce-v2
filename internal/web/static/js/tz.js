// Tells the server the browser's time zone (cookie "tz", not sensitive) so times can be shown in it when the account has no zone saved.
(function () {
  try {
    const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    if (!zone || document.cookie.split('; ').includes('tz=' + zone)) return;
    document.cookie = 'tz=' + zone + '; path=/; max-age=31536000; samesite=lax';
  } catch (e) { /* no Intl: the server falls back to APP_TIMEZONE or UTC */ }
})();
