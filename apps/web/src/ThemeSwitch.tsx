import { useEffect, useState } from 'react';
import { Monitor, Moon, Sun } from 'lucide-react';
import { getTheme, setTheme, type ThemePreference } from './theme';

export default function ThemeSwitch() {
  const [value, update] = useState(getTheme);
  const [unsaved, setUnsaved] = useState(false);
  useEffect(() => {
    const refresh = () => update(getTheme());
    window.addEventListener('aihub-theme', refresh);
    return () => window.removeEventListener('aihub-theme', refresh);
  }, []);
  const Icon = value === 'system' ? Monitor : value === 'dark' ? Moon : Sun;
  return <div className="appearance-control"><label className="theme-switch" title="界面主题，仅保存在这台设备">
    <Icon size={16} aria-hidden="true"/>
    <select aria-label="界面主题" value={value} onChange={e => setUnsaved(!setTheme(e.target.value as ThemePreference))}>
      <option value="system">跟随系统</option><option value="light">浅色主题</option><option value="dark">深色主题</option>
    </select>
  </label>{unsaved && <small role="status">已切换，本次未能保存偏好</small>}</div>;
}
