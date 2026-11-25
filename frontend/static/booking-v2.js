// Progressive enhancement: every date and time remains a native server form.
document.querySelectorAll('[data-slot-form]').forEach((form) => {
  const labels = [...form.querySelectorAll('.booking-slot')];
  const periods = form.querySelector('[data-periods]');
  const more = form.querySelector('[data-more-slots]');
  if (!periods || !more || !labels.length) return;
  const group = (label) => {
    const text = label.querySelector('span').textContent.trim();
    const hour = Number(text.split(':')[0]);
    return hour < 12 ? 0 : hour < 17 ? 1 : 2;
  };
  const groups = [0, 1, 2].map((index) => labels.filter((label) => group(label) === index));
  let active = labels.some((label) => label.querySelector('input').checked)
    ? group(labels.find((label) => label.querySelector('input').checked))
    : groups.findIndex((items) => items.length);
  let limit = 6;
  const names = [periods.dataset.morning, periods.dataset.day, periods.dataset.evening];
  const buttons = names.map((name, index) => {
    if (!groups[index].length) {
      const empty = document.createElement('span');
      empty.className = 'v2-period-unavailable';
      const title = document.createElement('span'); title.textContent = name;
      const detail = document.createElement('small'); detail.textContent = `— ${periods.dataset.empty}`;
      empty.append(title, detail); periods.append(empty);
      return null;
    }
    const button = document.createElement('button');
    button.type = 'button';
    const title = document.createElement('span');
    title.textContent = name;
    const count = document.createElement('small');
    count.textContent = groups[index].length ? String(groups[index].length) : periods.dataset.empty;
    button.append(title, count);
    button.disabled = !groups[index].length;
    button.addEventListener('click', () => { active = index; limit = 6; update(); });
    periods.append(button);
    return button;
  });
  periods.setAttribute('role', 'group');
  periods.hidden = false;
  function update() {
    buttons.forEach((button, index) => button?.setAttribute('aria-pressed', String(index === active)));
    labels.forEach((label) => {
      label.hidden = group(label) !== active || groups[active].indexOf(label) >= limit;
      const input = label.querySelector('input');
      input.required = !label.hidden;
      input.disabled = label.hidden && !input.checked;
    });
    const remaining = Math.max(0, groups[active].length - limit);
    more.hidden = groups[active].length <= 6;
    more.textContent = remaining ? `${more.dataset.more} · ${remaining}` : more.dataset.less;
    more.setAttribute('aria-expanded', String(limit > 6));
    const selected = labels.find((label) => label.querySelector('input').checked);
    const status = form.querySelector('[data-selected-time]');
    status.textContent = selected ? `${status.dataset.label}: ${selected.querySelector('span').textContent}` : '';
    const edit = document.querySelector('[data-edit-service]');
    if (edit && selected) {
      const url = new URL(edit.href);
      url.searchParams.set('previous_time', selected.querySelector('input').value);
      edit.href = url.href;
    }
  }
  more.addEventListener('click', () => {
    const previous = limit;
    limit = limit < groups[active].length ? limit + 6 : 6;
    update();
    if (limit > previous) groups[active][previous]?.querySelector('input').focus();
    else buttons[active].focus();
  });
  form.addEventListener('change', update);
  update();
});

document.querySelectorAll('[data-date-tools]').forEach((tools) => {
  const calendar = document.querySelector('.booking-calendar');
  if (!calendar) return;
  const days = [...calendar.querySelectorAll('button[name="date"]')]
    .filter((button) => button.value >= tools.dataset.min && button.value <= tools.dataset.max);
  if (!days.length || typeof HTMLDialogElement === 'undefined') return;
  const language = document.documentElement.lang || 'ru';
  const asDate = (value) => new Date(`${value}T12:00:00Z`);
  const format = (value, options) => new Intl.DateTimeFormat(language, { ...options, timeZone: 'UTC' }).format(asDate(value));
  let offset = Math.max(0, days.findIndex((day) => day.value === tools.dataset.date));
  const dialog = document.createElement('dialog');
  dialog.className = 'v2-calendar-dialog';
  dialog.setAttribute('aria-labelledby', 'calendar-title');
  const close = document.createElement('button');
  close.type = 'button';
  close.className = 'v2-calendar-close';
  close.textContent = '×';
  close.setAttribute('aria-label', tools.dataset.closeLabel);
  // Restore the original heading and form on browsers without dialog support.
  calendar.before(dialog);
  dialog.append(close, calendar);
  close.addEventListener('click', () => dialog.close());
  let opener;
  dialog.addEventListener('close', () => opener?.focus());
  const row = document.createElement('div'); row.className = 'v2-date-row';
  const month = document.createElement('button'); month.type = 'button'; month.className = 'v2-month-button';
  month.setAttribute('aria-haspopup', 'dialog');
  month.addEventListener('click', () => { opener = month; dialog.showModal(); close.focus(); });
  const previous = document.createElement('button'); previous.type = 'button'; previous.textContent = '‹'; previous.setAttribute('aria-label', tools.dataset.prevLabel);
  const next = document.createElement('button'); next.type = 'button'; next.textContent = '›'; next.setAttribute('aria-label', tools.dataset.nextLabel);
  row.append(month, previous, next);
  const strip = document.createElement('div'); strip.className = 'v2-date-strip';
  const nearest = document.createElement('a'); nearest.className = 'v2-nearest'; nearest.textContent = tools.dataset.nearestLabel;
  const href = (date) => `${tools.dataset.path}?${new URLSearchParams({service: tools.dataset.service, date})}#booking-slots`;
  const firstAvailable = days.find((day) => !day.disabled);
  if (firstAvailable) nearest.href = href(firstAvailable.value); else nearest.hidden = true;
  tools.append(row, strip, nearest); tools.hidden = false;
  function draw() {
    month.textContent = `${format(days[offset].value, {month: 'long', year: 'numeric'})} ▾`;
    previous.disabled = offset === 0; next.disabled = offset + 5 >= days.length;
    strip.replaceChildren();
    days.slice(offset, offset + 5).forEach((day) => {
      const button = document.createElement('button'); button.type = 'button'; button.disabled = day.disabled;
      button.setAttribute('aria-label', day.getAttribute('aria-label'));
      button.setAttribute('aria-pressed', String(day.value === tools.dataset.date));
      const weekday = document.createElement('small'); weekday.textContent = format(day.value, {weekday: 'short'});
      const number = document.createElement('strong'); number.textContent = format(day.value, {day: 'numeric'});
      const count = document.createElement('small'); count.textContent = day.disabled ? tools.dataset.emptyLabel : `${tools.dataset.countLabel}: ${day.dataset.count}`;
      button.append(weekday, number, count);
      button.addEventListener('click', () => { location.href = href(day.value); });
      strip.append(button);
    });
  }
  previous.addEventListener('click', () => { offset = Math.max(0, offset - 5); draw(); });
  next.addEventListener('click', () => { offset = Math.min(days.length - 1, offset + 5); draw(); });
  draw();
});

const receiptLink = document.querySelector('[data-receipt-link]');
if (receiptLink) {
  receiptLink.value = location.href;
  const fallback = document.querySelector('[data-link-fallback]');
  fallback.hidden = true;
  const copy = document.querySelector('[data-copy-receipt]');
  copy.hidden = false;
  copy.addEventListener('click', async () => {
    const status = document.querySelector('[data-copy-status]');
    try { await navigator.clipboard.writeText(receiptLink.value); fallback.hidden = true; status.textContent = copy.dataset.success; }
    catch { fallback.hidden = false; receiptLink.focus(); receiptLink.select(); status.textContent = copy.dataset.failure; }
  });
}
const calendarDownload = document.querySelector('[data-save-calendar]');
if (calendarDownload) {
  calendarDownload.hidden = false;
  calendarDownload.addEventListener('click', () => {
    const escape = (value) => value.replace(/\\/g, '\\\\').replace(/\r?\n/g, '\\n').replace(/,/g, '\\,').replace(/;/g, '\\;');
    const lines = ['BEGIN:VCALENDAR', 'VERSION:2.0', 'PRODID:-//Okno//Booking//EN', 'BEGIN:VEVENT',
      `UID:${calendarDownload.dataset.start}-${calendarDownload.dataset.end}@okno`,
      `DTSTAMP:${new Date().toISOString().replace(/[-:]/g, '').replace(/\.\d{3}/, '')}`,
      `DTSTART:${calendarDownload.dataset.start}`, `DTEND:${calendarDownload.dataset.end}`,
      `SUMMARY:${escape(calendarDownload.dataset.title)}`, `DESCRIPTION:${escape(calendarDownload.dataset.provider)}`,
      'END:VEVENT', 'END:VCALENDAR'];
    // Fold at UTF-8 byte boundaries as required by iCalendar (75 octets per line).
    const fold = (line) => {
      let result = '', width = 0;
      for (const char of line) {
        const bytes = new TextEncoder().encode(char).length;
        if (width + bytes > 75) { result += '\r\n '; width = 1; }
        result += char; width += bytes;
      }
      return result;
    };
    const url = URL.createObjectURL(new Blob([lines.map(fold).join('\r\n') + '\r\n'], {type: 'text/calendar;charset=utf-8'}));
    const link = document.createElement('a'); link.href = url; link.download = 'okno.ics'; link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  });
}
