// scheduled/preset — human text for a task's schedule.
import { t } from '../../core/i18n.js';

export function presetText(tk) {
  const hm = `${String(tk.hour).padStart(2, '0')}:${String(tk.minute).padStart(2, '0')}`;
  const map = {
    manual: t('tasks.preset.manual'),
    hourly: t('tasks.preset.hourlyAt', { minute: tk.minute }),
    daily: t('tasks.preset.dailyAt', { time: hm }),
    weekdays: t('tasks.preset.weekdaysAt', { time: hm }),
    weekly: t('tasks.preset.weeklyAt', { weekday: t('tasks.weekday.' + (tk.weekday || 0)), time: hm }),
  };
  return map[tk.preset] || tk.preset;
}
