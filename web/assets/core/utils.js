(function(ns) {
  const $ = (id) => document.getElementById(id);
  const escapeHtml = (value) => String(value ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('\"', '&quot;')
    .replaceAll("'", '&#39;');

  function fmt(sec) {
    sec = Math.max(0, sec | 0);
    const m = Math.floor(sec / 60);
    const s = sec % 60;
    return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  }

  function normText(s) {
    return String(s || '').trim().toLowerCase().replace(/\s+/g, ' ');
  }

  Object.assign(ns, { $, escapeHtml, fmt, normText });
})(window.HexSonic = window.HexSonic || {});
