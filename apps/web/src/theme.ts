export type ThemePreference = 'system' | 'light' | 'dark';
export const THEME_KEY = 'aihub-appearance-v1';
export const parseTheme = (value: unknown): ThemePreference => value === 'light' || value === 'dark' ? value : 'system';
export const resolveTheme = (value: ThemePreference, dark: boolean) => value === 'system' ? (dark ? 'dark' : 'light') : value;
let preference: ThemePreference | undefined;
export function getTheme(): ThemePreference {
  if (preference !== undefined) return preference;
  try { return parseTheme(localStorage.getItem(THEME_KEY)); } catch { return 'system'; }
}
function applyTheme() {
  const theme = resolveTheme(getTheme(), window.matchMedia('(prefers-color-scheme: dark)').matches);
  document.documentElement.dataset.theme = theme;
  document.documentElement.style.colorScheme = theme;
}
export function setTheme(value: ThemePreference): boolean {
  preference = value;
  let saved = true;
  try { localStorage.setItem(THEME_KEY, value); } catch { saved = false; }
  applyTheme();
  window.dispatchEvent(new Event('aihub-theme'));
  return saved;
}
export function initializeTheme() {
  applyTheme();
  const media = window.matchMedia('(prefers-color-scheme: dark)');
  const onSystemChange = () => { if (getTheme() === 'system') applyTheme(); };
  const onStorage = (event: StorageEvent) => {
    if (event.key === THEME_KEY || event.key === null) {
      preference = undefined;
      applyTheme();
      window.dispatchEvent(new Event('aihub-theme'));
    }
  };
  media.addEventListener('change', onSystemChange);
  window.addEventListener('storage', onStorage);
  return () => { media.removeEventListener('change', onSystemChange); window.removeEventListener('storage', onStorage); };
}
