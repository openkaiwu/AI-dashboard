using System;
using System.Drawing;
using System.Globalization;
using System.Windows.Forms;

namespace TimeZoneSwitch
{
    internal static class Tests
    {
        private static int checks;
        private static void Check(bool value, string name)
        {
            if (!value) throw new Exception("FAIL: " + name);
            checks++;
            Console.WriteLine("PASS: " + name);
        }

        [STAThread]
        private static int Main(string[] args)
        {
            try
            {
                Check(ZoneTarget.Find("la").Id == "Pacific Standard Time", "Los Angeles Windows ID");
                Check(ZoneTarget.Find("cn").Id == "China Standard Time", "China Windows ID");
                bool rejected = false;
                try { ZoneTarget.Find("arbitrary"); } catch (ArgumentException) { rejected = true; }
                Check(rejected, "Reject unknown target");
                DateTimeOffset winter = DateTimeOffset.Parse("2026-01-15T12:00:00Z", CultureInfo.InvariantCulture);
                DateTimeOffset summer = DateTimeOffset.Parse("2026-07-15T12:00:00Z", CultureInfo.InvariantCulture);
                Check(ZoneTarget.LosAngeles.Zone.GetUtcOffset(winter) == TimeSpan.FromHours(-8), "Los Angeles winter UTC-8");
                Check(ZoneTarget.LosAngeles.Zone.GetUtcOffset(summer) == TimeSpan.FromHours(-7), "Los Angeles summer UTC-7");
                Check(ZoneTarget.China.Zone.GetUtcOffset(winter) == TimeSpan.FromHours(8) && ZoneTarget.China.Zone.GetUtcOffset(summer) == TimeSpan.FromHours(8), "China UTC+8 in both seasons");
                DateTimeOffset spring = DateTimeOffset.Parse("2026-03-08T10:00:00Z", CultureInfo.InvariantCulture);
                DateTimeOffset fall = DateTimeOffset.Parse("2026-11-01T09:00:00Z", CultureInfo.InvariantCulture);
                Check(ZoneTarget.LosAngeles.Zone.GetUtcOffset(spring.AddSeconds(-1)) == TimeSpan.FromHours(-8) && ZoneTarget.LosAngeles.Zone.GetUtcOffset(spring) == TimeSpan.FromHours(-7), "DST spring transition boundary");
                Check(ZoneTarget.LosAngeles.Zone.GetUtcOffset(fall.AddSeconds(-1)) == TimeSpan.FromHours(-7) && ZoneTarget.LosAngeles.Zone.GetUtcOffset(fall) == TimeSpan.FromHours(-8), "DST fall transition boundary");
                ZoneState state = new ZoneState { Id = ZoneTarget.LosAngeles.Id, DaylightDisabled = true, AutoStart = 4 };
                Check(!state.Ready(ZoneTarget.LosAngeles), "Pacific with DST disabled is not ready");
                state.DaylightDisabled = false; state.AutoStart = 3;
                Check(!state.Ready(ZoneTarget.LosAngeles), "Automatic time zone is not a verified manual selection");
                state.AutoStart = 4; state.AutoRunning = true;
                Check(!state.Ready(ZoneTarget.LosAngeles), "Running automatic service is not ready");
                state.AutoRunning = false;
                Check(state.Ready(ZoneTarget.LosAngeles), "Manual Pacific with DST enabled is ready");
                Check(!TimeZoneService.Apply(null).Success, "Apply rejects null before mutating settings");
                Application.EnableVisualStyles();
                Application.SetCompatibleTextRenderingDefault(false);
                using (MainForm form = new MainForm())
                {
                    form.CreateControl();
                    Console.WriteLine("LAYOUT: ClientSize=" + form.ClientSize + " AutoScaleDimensions=" + form.AutoScaleDimensions + " Current=" + form.CurrentAutoScaleDimensions + " Font=" + form.Font);
                    float dpi;
                    using (Graphics graphics = form.CreateGraphics()) dpi = graphics.DpiX;
                    Console.WriteLine("LAYOUT: Graphics DPI=" + dpi);
                    foreach (Control child in form.Controls)
                    {
                        Console.WriteLine("LAYOUT: " + child.GetType().Name + " " + child.Text + " Bounds=" + child.Bounds + " Font=" + child.Font.Size);
                    }
                    Check(form.ClientSize.Width >= (int)(760 * dpi / 96) && form.ClientSize.Height >= (int)(650 * dpi / 96), "Window scales layout bounds at actual display DPI");
                }
                Console.WriteLine("PASS: " + checks + " checks. No system settings changed by these tests.");
                return 0;
            }
            catch (Exception ex) { Console.Error.WriteLine(ex); return 1; }
        }
    }
}
