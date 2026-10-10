document.querySelectorAll('.password-toggle').forEach((button) => {
  const input = button.closest('.password-field').querySelector('input');
  button.hidden = false;
  button.addEventListener('click', () => {
    const visible = input.type === 'password';
    input.type = visible ? 'text' : 'password';
    button.setAttribute('aria-pressed', String(visible));
    button.setAttribute('aria-label', visible ? button.dataset.hideLabel : button.dataset.showLabel);
  });
});

const publicLink = document.querySelector('[data-public-link]');
const copyButton = document.querySelector('.copy-link');
if (publicLink && copyButton && navigator.clipboard && window.isSecureContext) {
  copyButton.hidden = false;
  copyButton.addEventListener('click', async () => {
    const status = document.querySelector('.copy-status');
    try {
      await navigator.clipboard.writeText(publicLink.href);
      status.textContent = status.dataset.success;
    } catch {
      status.textContent = status.dataset.failure;
    }
  });
}

document.querySelectorAll('.language-picker').forEach((form) => {
  form.querySelector('.language-apply').hidden = true;
  form.querySelector('.language-select').addEventListener('change', () => form.requestSubmit());
});

const timezoneSelect = document.querySelector('[data-auto-timezone="true"]');
if (timezoneSelect) {
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  if (timezone && timezone !== 'Local') {
    if (![...timezoneSelect.options].some((option) => option.value === timezone)) {
      const offset = new Intl.DateTimeFormat('en-US', {
        timeZone: timezone, timeZoneName: 'shortOffset',
      }).formatToParts(new Date()).find((part) => part.type === 'timeZoneName').value;
      const utcOffset = offset === 'GMT' ? 'UTC+0' : offset.replace('GMT', 'UTC');
      timezoneSelect.add(new Option(`${timezone} — ${utcOffset}`, timezone));
    }
    timezoneSelect.value = timezone;
  }
}

document.querySelectorAll('.schedule-row').forEach((row) => {
  const toggle = row.querySelector('input[type=checkbox]');
  const status = row.querySelector('.schedule-day-status');
  const update = () => {
    row.classList.toggle('schedule-closed', !toggle.checked);
    row.querySelectorAll('input[type=time]').forEach((input) => input.readOnly = !toggle.checked);
    status.textContent = toggle.checked ? status.dataset.open : status.dataset.closed;
  };
  toggle.addEventListener('change', update);
  update();
});

document.querySelectorAll('.booking-calendar').forEach((calendar) => {
  const months = [...calendar.querySelectorAll('.calendar-month')];
  const navigation = calendar.querySelector('.calendar-navigation');
  const previous = calendar.querySelector('[data-calendar-prev]');
  const next = calendar.querySelector('[data-calendar-next]');
  let index = Math.max(0, months.findIndex((month) => month.dataset.selected === 'true'));
  const show = () => {
    months.forEach((month, i) => month.hidden = i !== index);
    previous.disabled = index === 0;
    next.disabled = index === months.length - 1;
  };
  previous.addEventListener('click', () => { if (index > 0) { index--; show(); } });
  next.addEventListener('click', () => { if (index < months.length - 1) { index++; show(); } });
  navigation.hidden = false;
  show();
});
document.querySelectorAll('.calendar-service-form').forEach((form) => {
  form.querySelector('.calendar-refresh').hidden = Boolean(document.querySelector('.booking-calendar'));
  form.querySelector('select').addEventListener('change', () => form.requestSubmit());
});


document.querySelectorAll('[data-owner-calendar]').forEach((calendar) => {
  const panels = [...calendar.querySelectorAll('[data-owner-panel]')];
  const links = [...calendar.querySelectorAll('[data-owner-visit]')];
  if (!panels.length) return;
  const show = (id, scroll = false) => {
    const panel = panels.find((item) => item.dataset.ownerPanel === id);
    if (!panel) return;
    panels.forEach((item) => item.hidden = item !== panel);
    links.forEach((link) => {
      const selected = link.dataset.ownerVisit === id;
      link.classList.toggle('event-selected', selected);
      if (selected) link.setAttribute('aria-current', 'true');
      else link.removeAttribute('aria-current');
    });
    if (scroll && window.matchMedia('(max-width: 760px)').matches) panel.scrollIntoView({block: 'start'});
  };
  links.forEach((link) => link.addEventListener('click', (event) => {event.preventDefault();show(link.dataset.ownerVisit, true);}));
  show(panels.find((item) => item.dataset.selected === 'true')?.dataset.ownerPanel || panels[0].dataset.ownerPanel);
});

document.querySelectorAll('.days-off-widget').forEach((widget) => {
  const input = widget.querySelector('[name=days_off]');
  const parse = (value) => {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return null;
    const date = new Date(value + 'T12:00:00Z');
    return Number.isFinite(date.getTime()) && date.toISOString().slice(0, 10) === value ? date : null;
  };
  const initial = input.value.split(/[\n,\r]/).map((v) => v.trim()).filter(Boolean);
  const today = widget.dataset.today;
  if (!parse(today) || initial.some((date) => !parse(date))) return;
  const selected = new Set(initial);
  const picker = widget.querySelector('.days-off-picker');
  const grid = widget.querySelector('.days-off-grid');
  const heading = widget.querySelector('.days-off-month');
  const list = widget.querySelector('.days-off-selected');
  const status = widget.querySelector('.days-off-status');
  const previous = widget.querySelector('[data-off-prev]');
  const next = widget.querySelector('[data-off-next]');
  const locale = document.documentElement.lang || 'ru';
  const monthFormat = new Intl.DateTimeFormat(locale, {month:'long',year:'numeric',timeZone:'UTC'});
  const dateFormat = new Intl.DateTimeFormat(locale, {day:'numeric',month:'long',year:'numeric',timeZone:'UTC'});
  let month = new Date(today.slice(0, 7) + '-01T12:00:00Z');
  const toggle = (value) => {
    if (selected.has(value)) selected.delete(value);
    else {
      if (selected.size >= 100) { status.textContent = widget.dataset.limit; return; }
      selected.add(value);
    }
    input.value = [...selected].sort().join('\n');
    status.textContent = widget.dataset.unsaved;
    render();
  };
  const render = () => {
    heading.textContent = monthFormat.format(month);
    grid.replaceChildren();
    const offset = (month.getUTCDay() + 6) % 7;
    for (let i = 0; i < offset; i++) {
      const blank = document.createElement('span');
      blank.setAttribute('aria-hidden', 'true');
      grid.append(blank);
    }
    const end = new Date(Date.UTC(month.getUTCFullYear(), month.getUTCMonth()+1, 0,12)).getUTCDate();
    for (let day = 1; day <= end; day++) {
      const value = month.toISOString().slice(0,7) + '-' + String(day).padStart(2,'0');
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'calendar-day off-day' + (selected.has(value) ? ' off-selected' : '') + (value === today ? ' calendar-today' : '');
      button.textContent = String(day);
      button.dataset.date = value;
      button.setAttribute('aria-label', dateFormat.format(parse(value)));
      button.setAttribute('aria-pressed', String(selected.has(value)));
      button.disabled = value < today && !selected.has(value);
      button.addEventListener('click', () => {
        toggle(value);
        grid.querySelector(`[data-date="${value}"]`)?.focus();
      });
      grid.append(button);
    }
    previous.disabled = month.getUTCFullYear() === 1 && month.getUTCMonth() === 0;
    next.disabled = month.getUTCFullYear() === 9999 && month.getUTCMonth() === 11;
    list.replaceChildren();
    if (!selected.size) {
      const empty = document.createElement('p'); empty.className = 'field-hint'; empty.textContent = widget.dataset.empty; list.append(empty);
    }
    [...selected].sort().forEach((value) => {
      const chip = document.createElement('button'); chip.type = 'button';chip.className = 'days-off-chip';
      chip.textContent = dateFormat.format(parse(value)) + ' ×';
      chip.setAttribute('aria-label', widget.dataset.remove + ': ' + dateFormat.format(parse(value)));
      chip.addEventListener('click', () => toggle(value));list.append(chip);
    });
  };
  previous.addEventListener('click', () => {month.setUTCMonth(month.getUTCMonth()-1);render();});
  next.addEventListener('click', () => {month.setUTCMonth(month.getUTCMonth()+1);render();});
  widget.querySelector('.days-off-fallback').hidden = true;
  picker.hidden = false;
  render();
});
