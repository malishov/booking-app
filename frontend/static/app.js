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
