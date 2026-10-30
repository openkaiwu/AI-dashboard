using System;
using System.Reflection;
using System.Runtime.InteropServices;
using System.Windows.Forms;
using System.Web.Script.Serialization;
using System.Text;

[assembly: AssemblyTitle("时区切换")]
[assembly: AssemblyDescription("洛杉矶与中国 Windows 系统时区一键切换")]
[assembly: AssemblyCompany("TimeZoneSwitch")]
[assembly: AssemblyProduct("时区切换 Windows")]
[assembly: AssemblyVersion("1.0.0.0")]
[assembly: AssemblyFileVersion("1.0.0.0")]

namespace TimeZoneSwitch
{
    internal static class Program
    {
        [DllImport("user32.dll")]
        private static extern bool SetProcessDPIAware();

        [STAThread]
        private static int Main(string[] args)
        {
            if (args.Length == 1 && args[0] == "--status")
            {
                try { WriteJson(State()); return 0; }
                catch (Exception ex) { WriteJson(new { success = false, message = ex.Message }); return 1; }
            }
            if (args.Length == 2 && args[0] == "--switch")
            {
                try
                {
                    ZoneTarget target = ZoneTarget.Find(args[1]);
                    ApplyResult result = TimeZoneService.Apply(target);
                    if (result.NeedsElevation) result = TimeZoneService.ApplyElevated(target);
                    WriteJson(new { success = result.Success, message = result.Message, state = State() });
                    return result.Success ? 0 : 1;
                }
                catch (Exception ex) { WriteJson(new { success = false, message = ex.Message }); return 1; }
            }
            if (args.Length != 0)
            {
                if (args.Length != 2 || args[0] != "--apply") return 64;
                try
                {
                    ApplyResult result = TimeZoneService.Apply(ZoneTarget.Find(args[1]));
                    return result.Success ? 0 : (result.RollbackFailed ? 2 : 1);
                }
                catch { return 1; }
            }
            SetProcessDPIAware();
            Application.EnableVisualStyles();
            Application.SetCompatibleTextRenderingDefault(false);
            try { Application.Run(new MainForm()); }
            catch (Exception ex)
            {
                MessageBox.Show("软件启动失败：" + ex.Message, "时区切换", MessageBoxButtons.OK, MessageBoxIcon.Error);
                return 1;
            }
            return 0;
        }
        private static object State()
        {
            ZoneState state = TimeZoneService.ReadState();
            TimeZoneInfo.ClearCachedData();
            return new { id = state.Id, manual = state.Manual, daylightDisabled = state.DaylightDisabled, offsetMinutes = TimeZoneInfo.Local.GetUtcOffset(DateTime.UtcNow).TotalMinutes,
                chinaReady = state.Ready(ZoneTarget.China), laReady = state.Ready(ZoneTarget.LosAngeles) };
        }
        private static void WriteJson(object value)
        {
            using (System.IO.StreamWriter output = new System.IO.StreamWriter(Console.OpenStandardOutput(), new UTF8Encoding(false)))
                output.WriteLine(new JavaScriptSerializer().Serialize(value));
        }
    }
}
