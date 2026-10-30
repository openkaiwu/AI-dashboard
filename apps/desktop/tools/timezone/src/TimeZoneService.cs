using System;
using System.ComponentModel;
using System.Diagnostics;
using System.IO;
using System.Security;
using System.Security.Principal;
using System.ServiceProcess;
using System.Threading;
using System.Threading.Tasks;
using Microsoft.Win32;

namespace TimeZoneSwitch
{
    public sealed class ZoneTarget
    {
        public readonly string Key;
        public readonly string Id;
        public readonly string Name;
        public readonly TimeZoneInfo Zone;

        private ZoneTarget(string key, string id, string name)
        {
            Key = key; Id = id; Name = name;
            Zone = TimeZoneInfo.FindSystemTimeZoneById(id);
        }

        public static readonly ZoneTarget LosAngeles = new ZoneTarget("la", "Pacific Standard Time", "洛杉矶时间");
        public static readonly ZoneTarget China = new ZoneTarget("cn", "China Standard Time", "中国时间");

        public static ZoneTarget Find(string key)
        {
            if (key == "la") return LosAngeles;
            if (key == "cn") return China;
            throw new ArgumentException("只支持洛杉矶和中国时区。");
        }

        public string OffsetAt(DateTimeOffset utc)
        {
            TimeSpan offset = Zone.GetUtcOffset(utc);
            return "UTC" + (offset < TimeSpan.Zero ? "−" : "+") + offset.Duration().ToString(@"hh\:mm");
        }
    }

    public sealed class ZoneState
    {
        public string Id;
        public bool DaylightDisabled;
        public int? AutoStart;
        public bool AutoRunning;
        public bool Manual { get { return (!AutoStart.HasValue || AutoStart.Value == 4) && !AutoRunning; } }

        public bool Matches(ZoneTarget target)
        {
            return String.Equals(Id, target.Id, StringComparison.OrdinalIgnoreCase) &&
                (target != ZoneTarget.LosAngeles || !DaylightDisabled);
        }

        public bool Ready(ZoneTarget target) { return Matches(target) && Manual; }
    }

    public sealed class ApplyResult
    {
        public bool Success;
        public bool NeedsElevation;
        public bool RollbackFailed;
        public string Message;
    }

    public static class TimeZoneService
    {
        private const string AutoKey = @"SYSTEM\CurrentControlSet\Services\tzautoupdate";
        private const string ZoneKey = @"SYSTEM\CurrentControlSet\Control\TimeZoneInformation";

        public static bool IsAdmin
        {
            get
            {
                using (WindowsIdentity identity = WindowsIdentity.GetCurrent())
                    return new WindowsPrincipal(identity).IsInRole(WindowsBuiltInRole.Administrator);
            }
        }

        public static ZoneState ReadState()
        {
            ZoneState state = new ZoneState();
            using (RegistryKey key = Registry.LocalMachine.OpenSubKey(ZoneKey))
            {
                if (key == null) throw new InvalidOperationException("无法读取 Windows 时区设置。");
                state.Id = Convert.ToString(key.GetValue("TimeZoneKeyName", "")).TrimEnd('\0');
                state.DaylightDisabled = Convert.ToInt32(key.GetValue("DynamicDaylightTimeDisabled", 0)) != 0;
            }
            using (RegistryKey key = Registry.LocalMachine.OpenSubKey(AutoKey))
            {
                if (key != null) state.AutoStart = Convert.ToInt32(key.GetValue("Start", 3));
            }
            if (state.AutoStart.HasValue)
            {
                using (ServiceController service = new ServiceController("tzautoupdate"))
                    state.AutoRunning = service.Status != ServiceControllerStatus.Stopped;
            }
            return state;
        }

        private static void SetAutoStart(int start)
        {
            using (RegistryKey key = Registry.LocalMachine.OpenSubKey(AutoKey, true))
            {
                if (key == null) throw new InvalidOperationException("无法读取自动时区服务。");
                key.SetValue("Start", start, RegistryValueKind.DWord);
            }
        }

        private static void StopAutoService()
        {
            using (ServiceController service = new ServiceController("tzautoupdate"))
            {
                if (service.Status == ServiceControllerStatus.Stopped) return;
                if (service.Status != ServiceControllerStatus.StopPending) service.Stop();
                service.WaitForStatus(ServiceControllerStatus.Stopped, TimeSpan.FromSeconds(8));
            }
        }

        private static void RunTzutil(string id, bool daylightDisabled)
        {
            // Only predefined targets or a previous Windows registry value reach this call.
            // UseShellExecute=false ensures the ID is never interpreted by a command shell.
            if (String.IsNullOrWhiteSpace(id) || id.IndexOf('"') >= 0 || id.IndexOf('\r') >= 0 || id.IndexOf('\n') >= 0)
                throw new ArgumentException("无效的 Windows 时区 ID。");
            ProcessStartInfo start = new ProcessStartInfo();
            start.FileName = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.System), "tzutil.exe");
            start.Arguments = "/s \"" + id + (daylightDisabled ? "_dstoff" : "") + "\"";
            start.UseShellExecute = false;
            start.CreateNoWindow = true;
            start.RedirectStandardOutput = true;
            start.RedirectStandardError = true;
            using (Process process = Process.Start(start))
            {
                Task<string> output = process.StandardOutput.ReadToEndAsync();
                Task<string> error = process.StandardError.ReadToEndAsync();
                if (!process.WaitForExit(12000))
                {
                    try { process.Kill(); process.WaitForExit(2000); } catch (InvalidOperationException) { }
                    throw new System.TimeoutException("Windows 切换时区超时。");
                }
                string detail = (error.Result + " " + output.Result).Trim();
                if (process.ExitCode != 0)
                    throw new InvalidOperationException("Windows 拒绝切换时区。" + (detail.Length == 0 ? "" : " " + detail));
            }
            TimeZoneInfo.ClearCachedData();
        }

        public static ApplyResult Apply(ZoneTarget target)
        {
            // Validate identity as well as ID, so helper callers cannot supply arbitrary zones.
            if (target != ZoneTarget.LosAngeles && target != ZoneTarget.China)
                return new ApplyResult { Message = "不支持的时区。" };
            using (Mutex mutex = new Mutex(false, @"Local\TimeZoneSwitch.Apply"))
            {
                bool acquired;
                try { acquired = mutex.WaitOne(0); }
                catch (AbandonedMutexException) { acquired = true; }
                if (!acquired) return new ApplyResult { Message = "另一个窗口正在切换时区，请稍后重试。" };
                try { return ApplyLocked(target); }
                finally { mutex.ReleaseMutex(); }
            }
        }

        private static ApplyResult ApplyLocked(ZoneTarget target)
        {
            ZoneState original;
            try { original = ReadState(); }
            catch (Exception ex) { return new ApplyResult { Message = ex.Message }; }
            if (original.Ready(target)) return new ApplyResult { Success = true, Message = "已在使用" + target.Name + "。" };
            if (!original.Manual && !IsAdmin)
                return new ApplyResult { NeedsElevation = true, Message = "关闭自动时区需要管理员权限。" };

            bool autoTouched = false;
            bool zoneTouched = false;
            try
            {
                if (!original.Manual && original.AutoStart.HasValue)
                {
                    autoTouched = true;
                    if (original.AutoStart.Value != 4) SetAutoStart(4);
                    StopAutoService();
                }
                zoneTouched = true;
                RunTzutil(target.Id, false);
                if (!ReadState().Ready(target))
                    throw new InvalidOperationException("切换后系统设置未保持目标时区，请检查系统策略。");
                TimeZoneInfo.ClearCachedData();
                if (!String.Equals(TimeZoneInfo.Local.Id, target.Id, StringComparison.OrdinalIgnoreCase))
                    throw new InvalidOperationException("系统时区核验失败。");
                return new ApplyResult { Success = true, Message = "已切换到" + target.Name + "，系统时区已核验。" };
            }
            catch (Exception ex)
            {
                string rollbackError = "";
                // A failed attempt restores both settings, including the previous DST flag.
                if (zoneTouched)
                {
                    try
                    {
                        ZoneState current = ReadState();
                        if (current.Id != original.Id || current.DaylightDisabled != original.DaylightDisabled)
                            RunTzutil(original.Id, original.DaylightDisabled);
                    }
                    catch (Exception restore) { rollbackError = restore.Message; }
                }
                if (autoTouched)
                {
                    try
                    {
                        SetAutoStart(original.AutoStart.Value);
                        if (original.AutoRunning)
                        {
                            using (ServiceController service = new ServiceController("tzautoupdate"))
                            {
                                if (service.Status == ServiceControllerStatus.Stopped) service.Start();
                                service.WaitForStatus(ServiceControllerStatus.Running, TimeSpan.FromSeconds(8));
                            }
                        }
                    }
                    catch (Exception restore) { rollbackError += " " + restore.Message; }
                }
                return new ApplyResult
                {
                    Message = ex.Message + (rollbackError.Length == 0 ? "" : " 部分原设置未能恢复，请在系统设置中检查。"),
                    NeedsElevation = !IsAdmin && rollbackError.Length == 0,
                    RollbackFailed = rollbackError.Length != 0
                };
            }
        }

        public static ApplyResult ApplyElevated(ZoneTarget target)
        {
            try
            {
                ProcessStartInfo start = new ProcessStartInfo();
                start.FileName = System.Windows.Forms.Application.ExecutablePath;
                start.Arguments = "--apply " + target.Key;
                start.UseShellExecute = true;
                start.Verb = "runas";
                start.WindowStyle = ProcessWindowStyle.Hidden;
                using (Process process = Process.Start(start))
                {
                    // Do not abandon a helper while it can still change system settings.
                    process.WaitForExit();
                    if (process.ExitCode == 0 && ReadState().Ready(target))
                        return new ApplyResult { Success = true, Message = "已切换到" + target.Name + "，系统时区已核验。" };
                    return new ApplyResult
                    {
                        RollbackFailed = process.ExitCode == 2,
                        Message = process.ExitCode == 2 ? "切换失败，部分原设置未能恢复。请打开 Windows 日期和时间设置检查。" :
                            "系统未允许切换。请检查管理员权限或单位的系统策略，再重试。"
                    };
                }
            }
            catch (Win32Exception ex)
            {
                return new ApplyResult { Message = ex.NativeErrorCode == 1223 ? "已取消系统授权，本次未执行切换。" : "无法获取管理员权限：" + ex.Message };
            }
            catch (Exception ex) { return new ApplyResult { Message = "切换失败：" + ex.Message }; }
        }
    }
}
