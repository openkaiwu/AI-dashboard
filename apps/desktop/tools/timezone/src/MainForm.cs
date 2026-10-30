using System;
using System.Diagnostics;
using System.Drawing;
using System.Drawing.Drawing2D;
using System.Globalization;
using System.Threading.Tasks;
using System.Windows.Forms;

namespace TimeZoneSwitch
{
    internal static class Theme
    {
        public static readonly Color Background = Color.FromArgb(246, 247, 251);
        public static readonly Color Ink = Color.FromArgb(28, 35, 53);
        public static readonly Color Muted = Color.FromArgb(106, 115, 136);
        public static readonly Color Blue = Color.FromArgb(77, 84, 219);
        public static readonly Color Teal = Color.FromArgb(13, 129, 111);

        public static GraphicsPath Rounded(RectangleF r, float radius)
        {
            float d = radius * 2;
            GraphicsPath path = new GraphicsPath();
            path.AddArc(r.Left, r.Top, d, d, 180, 90);
            path.AddArc(r.Right - d, r.Top, d, d, 270, 90);
            path.AddArc(r.Right - d, r.Bottom - d, d, d, 0, 90);
            path.AddArc(r.Left, r.Bottom - d, d, d, 90, 90);
            path.CloseFigure();
            return path;
        }

        public static Label Label(Control parent, string text, int x, int y, int w, int h, float size, Color color, bool bold)
        {
            Label label = new Label();
            label.Text = text;
            label.SetBounds(x, y, w, h);
            label.Font = new Font("Microsoft YaHei UI", size, bold ? FontStyle.Bold : FontStyle.Regular);
            label.ForeColor = color;
            label.BackColor = parent is RoundPanel ? ((RoundPanel)parent).Surface : parent.BackColor;
            label.TextAlign = ContentAlignment.MiddleLeft;
            parent.Controls.Add(label);
            return label;
        }
    }

    internal sealed class RoundPanel : Panel
    {
        public Color Surface = Color.White;
        public Color Outline = Color.FromArgb(228, 231, 239);

        public RoundPanel()
        {
            SetStyle(ControlStyles.UserPaint | ControlStyles.AllPaintingInWmPaint | ControlStyles.OptimizedDoubleBuffer | ControlStyles.ResizeRedraw, true);
        }

        protected override void OnPaintBackground(PaintEventArgs e)
        {
            e.Graphics.Clear(Parent == null ? Theme.Background : Parent.BackColor);
            e.Graphics.SmoothingMode = SmoothingMode.AntiAlias;
            float scale = e.Graphics.DpiX / 96f;
            using (GraphicsPath path = Theme.Rounded(new RectangleF(1, 1, Width - 2, Height - 2), 17 * scale))
            using (SolidBrush fill = new SolidBrush(Surface))
            using (Pen pen = new Pen(Outline, scale))
            {
                e.Graphics.FillPath(fill, path);
                e.Graphics.DrawPath(pen, path);
            }
        }
    }

    internal sealed class SwitchButton : Button
    {
        public Color Accent = Theme.Blue;
        private bool hovered;

        public SwitchButton()
        {
            SetStyle(ControlStyles.UserPaint | ControlStyles.AllPaintingInWmPaint | ControlStyles.OptimizedDoubleBuffer | ControlStyles.ResizeRedraw, true);
            FlatStyle = FlatStyle.Flat;
            FlatAppearance.BorderSize = 0;
            Font = new Font("Microsoft YaHei UI", 10.5f, FontStyle.Bold);
            Cursor = Cursors.Hand;
        }

        protected override void OnMouseEnter(EventArgs e) { hovered = true; Invalidate(); base.OnMouseEnter(e); }
        protected override void OnMouseLeave(EventArgs e) { hovered = false; Invalidate(); base.OnMouseLeave(e); }
        protected override void OnEnabledChanged(EventArgs e) { Invalidate(); base.OnEnabledChanged(e); }
        protected override void OnPaint(PaintEventArgs e)
        {
            Graphics g = e.Graphics;
            g.Clear(Parent is RoundPanel ? ((RoundPanel)Parent).Surface : Parent.BackColor);
            g.SmoothingMode = SmoothingMode.AntiAlias;
            float scale = g.DpiX / 96f;
            Color fill = !Enabled ? Color.FromArgb(231, 234, 240) : hovered ? ControlPaint.Dark(Accent, 0.08f) : Accent;
            using (GraphicsPath path = Theme.Rounded(new RectangleF(0, 0, Width - 1, Height - 1), 10 * scale))
            using (SolidBrush brush = new SolidBrush(fill)) g.FillPath(brush, path);
            TextRenderer.DrawText(g, Text, Font, ClientRectangle, Enabled ? Color.White : Color.FromArgb(115, 125, 143),
                TextFormatFlags.HorizontalCenter | TextFormatFlags.VerticalCenter | TextFormatFlags.SingleLine);
            if (Focused && ShowFocusCues)
                ControlPaint.DrawFocusRectangle(g, new Rectangle(5, 5, Width - 10, Height - 10), Color.White, fill);
        }
    }

    internal sealed class ZoneCard
    {
        public readonly ZoneTarget Target;
        public readonly RoundPanel Panel;
        public readonly SwitchButton Button;
        private readonly Label time;
        private readonly Label date;
        private readonly Label rule;
        private readonly Label badge;

        public ZoneCard(Control parent, ZoneTarget target, int x, Color accent, Color surface)
        {
            Target = target;
            Panel = new RoundPanel();
            Panel.Surface = surface;
            Panel.BackColor = surface;
            Panel.Outline = Color.FromArgb(225, 229, 239);
            Panel.SetBounds(x, 238, 340, 270);
            Panel.AccessibleName = target.Name;
            parent.Controls.Add(Panel);
            bool la = target == ZoneTarget.LosAngeles;
            Theme.Label(Panel, la ? "洛杉矶" : "中国", 24, 18, 185, 34, 18, Theme.Ink, true);
            Theme.Label(Panel, la ? "LOS ANGELES" : "BEIJING / SHANGHAI", 26, 54, 265, 22, 8.5f, Theme.Muted, false);
            badge = Theme.Label(Panel, "● 当前时区", 222, 25, 100, 24, 9, accent, true);
            time = Theme.Label(Panel, "00:00:00", 20, 82, 304, 64, 36, Theme.Ink, false);
            time.Font.Dispose();
            time.Font = new Font("Segoe UI", 36, FontStyle.Regular);
            date = Theme.Label(Panel, "", 26, 148, 292, 27, 10, Theme.Muted, false);
            rule = Theme.Label(Panel, "", 26, 179, 292, 24, 9, accent, false);
            Button = new SwitchButton();
            Button.Accent = accent;
            Button.SetBounds(24, 218, 292, 40);
            Button.AccessibleName = la ? "切换到洛杉矶" : "切换到中国";
            Button.AccessibleDescription = "切换 Windows 系统时区为" + target.Name;
            Panel.Controls.Add(Button);
        }

        public void Refresh(DateTimeOffset utc, ZoneState state, bool busy)
        {
            DateTimeOffset local = TimeZoneInfo.ConvertTime(utc, Target.Zone);
            time.Text = local.ToString("HH:mm:ss", CultureInfo.InvariantCulture);
            date.Text = local.ToString("yyyy年M月d日  dddd", CultureInfo.GetCultureInfo("zh-CN"));
            bool daylight = Target.Zone.IsDaylightSavingTime(utc);
            rule.Text = Target.OffsetAt(utc) + "  ·  " + (Target == ZoneTarget.China ? "北京时间" : daylight ? "夏令时 PDT" : "标准时间 PST");
            bool matches = state != null && String.Equals(state.Id, Target.Id, StringComparison.OrdinalIgnoreCase);
            badge.Visible = matches;
            bool ready = state != null && state.Ready(Target);
            Button.Text = busy ? "正在处理…" : ready ? "✓  当前正在使用" : (matches ? "重新应用此时区" : "切换到" + (Target == ZoneTarget.China ? "中国时间" : "洛杉矶时间") + "  →");
            Button.Enabled = !busy && !ready;
        }
    }

    internal sealed class MainForm : Form
    {
        private readonly Label currentName;
        private readonly Label currentClock;
        private readonly Label status;
        private readonly Label difference;
        private readonly ZoneCard la;
        private readonly ZoneCard china;
        private readonly Timer timer;
        private readonly ToolTip tip;
        private ZoneState state;
        private bool busy;
        private bool statusError;
        private string observedId;

        public MainForm()
        {
            SuspendLayout();
            Text = "时区切换 · 洛杉矶 / 中国";
            ClientSize = new Size(760, 650);
            Font = new Font("Microsoft YaHei UI", 10);
            BackColor = Theme.Background;
            FormBorderStyle = FormBorderStyle.FixedSingle;
            MaximizeBox = false;
            StartPosition = FormStartPosition.CenterScreen;
            KeyPreview = true;
            DoubleBuffered = true;
            Icon = Icon.ExtractAssociatedIcon(Application.ExecutablePath);

            Theme.Label(this, "时区切换", 30, 23, 430, 49, 25, Theme.Ink, true);
            Label platform = Theme.Label(this, "WINDOWS", 590, 37, 138, 24, 9, Theme.Muted, true);
            platform.TextAlign = ContentAlignment.MiddleRight;
            Theme.Label(this, "洛杉矶 ↔ 中国，让电脑时间随你切换。", 32, 77, 650, 26, 10.5f, Theme.Muted, false);

            RoundPanel current = new RoundPanel();
            current.SetBounds(30, 121, 700, 94);
            current.AccessibleName = "电脑系统时区";
            Controls.Add(current);
            Theme.Label(current, "●  当前电脑时区", 23, 16, 300, 24, 9.5f, Theme.Muted, false);
            currentName = Theme.Label(current, "正在读取…", 22, 43, 440, 33, 17, Theme.Ink, true);
            currentName.AccessibleName = "当前电脑时区";
            currentClock = Theme.Label(current, "", 469, 25, 207, 43, 26, Theme.Ink, false);
            currentClock.Font.Dispose();
            currentClock.Font = new Font("Segoe UI", 26);
            currentClock.TextAlign = ContentAlignment.MiddleRight;

            la = new ZoneCard(this, ZoneTarget.LosAngeles, 30, Theme.Blue, Color.FromArgb(241, 242, 255));
            china = new ZoneCard(this, ZoneTarget.China, 390, Theme.Teal, Color.FromArgb(237, 248, 245));
            la.Button.Click += async delegate { await Switch(ZoneTarget.LosAngeles); };
            china.Button.Click += async delegate { await Switch(ZoneTarget.China); };

            status = Theme.Label(this, "选择城市，即可切换整台电脑的系统时区。", 32, 524, 696, 46, 10, Theme.Muted, false);
            status.AccessibleName = status.Text;
            tip = new ToolTip();
            difference = Theme.Label(this, "", 32, 579, 696, 23, 9, Theme.Muted, false);
            Theme.Label(this, "切换时关闭自动时区，保留自动校时设置。", 32, 609, 430, 22, 8.5f, Theme.Muted, false);

            LinkLabel settings = new LinkLabel();
            settings.Text = "Windows 日期和时间设置";
            settings.SetBounds(480, 607, 248, 25);
            settings.Font = new Font("Microsoft YaHei UI", 8.5f);
            settings.LinkColor = Theme.Blue;
            settings.ActiveLinkColor = Theme.Blue;
            settings.TextAlign = ContentAlignment.MiddleRight;
            settings.LinkBehavior = LinkBehavior.HoverUnderline;
            settings.LinkClicked += delegate
            {
                try { Process.Start(new ProcessStartInfo("ms-settings:dateandtime") { UseShellExecute = true }); }
                catch (Exception ex) { SetStatus("无法打开系统设置：" + ex.Message, true); }
            };
            Controls.Add(settings);
            tip.SetToolTip(la.Button, "Alt + L：切换到洛杉矶；自动处理夏令时。");
            tip.SetToolTip(china.Button, "Alt + C：切换到中国北京时间。");

            timer = new Timer();
            timer.Interval = 1000;
            timer.Tick += delegate { RefreshState(); };
            timer.Start();
            RefreshState();
            FormClosing += delegate(object sender, FormClosingEventArgs e)
            {
                if (busy) { e.Cancel = true; SetStatus("正在完成切换，请先完成系统授权或等待结果。", false); }
            };
            // Establish the design DPI after assigning fonts and adding controls. Doing
            // this earlier consumes the DPI change before the controls have been built.
            AutoScaleDimensions = new SizeF(96, 96);
            AutoScaleMode = AutoScaleMode.Dpi;
            ResumeLayout(true);
        }

        private void SetStatus(string message, bool error)
        {
            statusError = error;
            status.Text = message;
            status.AccessibleName = message;
            status.ForeColor = error ? Color.FromArgb(177, 66, 47) : Theme.Teal;
            tip.SetToolTip(status, message);
        }

        private void RefreshState()
        {
            try
            {
                state = TimeZoneService.ReadState();
                TimeZoneInfo.ClearCachedData();
                DateTimeOffset now = DateTimeOffset.UtcNow;
                string name = state.Id == ZoneTarget.China.Id ? "中国时间 · 北京 / 上海" : state.Id == ZoneTarget.LosAngeles.Id ? "洛杉矶时间 · 美国太平洋" : TimeZoneInfo.Local.DisplayName;
                currentName.Text = name;
                currentName.AccessibleName = "当前电脑时区：" + name;
                tip.SetToolTip(currentName, name + "\n" + state.Id + "\n" + (state.Manual ? "手动时区" : "自动时区已开启；切换时将关闭"));
                currentClock.Text = DateTime.Now.ToString("HH:mm:ss", CultureInfo.InvariantCulture);
                if (!busy && observedId != null && observedId != state.Id)
                    SetStatus("检测到系统时区已更新，当前使用" + (state.Id == ZoneTarget.China.Id ? "中国时间。" : state.Id == ZoneTarget.LosAngeles.Id ? "洛杉矶时间。" : "其他时区。"), false);
                observedId = state.Id;
                la.Refresh(now, state, busy);
                china.Refresh(now, state, busy);
                double hours = (ZoneTarget.China.Zone.GetUtcOffset(now) - ZoneTarget.LosAngeles.Zone.GetUtcOffset(now)).TotalHours;
                difference.Text = "中国时间比洛杉矶快 " + hours.ToString("0", CultureInfo.InvariantCulture) + " 小时  ·  洛杉矶夏令时自动调整";
            }
            catch (Exception ex)
            {
                state = null;
                currentName.Text = "暂时无法读取时区";
                la.Refresh(DateTimeOffset.UtcNow, null, busy);
                china.Refresh(DateTimeOffset.UtcNow, null, busy);
                if (!busy && !statusError) SetStatus("无法读取系统设置：" + ex.Message, true);
            }
        }

        private async Task Switch(ZoneTarget target)
        {
            if (busy) return;
            busy = true;
            SetStatus("正在切换到" + target.Name + "…如弹出系统授权窗口，请选择“是”。", false);
            RefreshState();
            try
            {
                ApplyResult result = await Task.Run(delegate { return TimeZoneService.Apply(target); });
                if (result.NeedsElevation)
                {
                    SetStatus("需要系统授权。请在 Windows 授权窗口中选择“是”以完成切换。", false);
                    result = await Task.Run(delegate { return TimeZoneService.ApplyElevated(target); });
                }
                RefreshState();
                SetStatus(result.Message, !result.Success);
            }
            catch (Exception ex) { SetStatus("切换失败：" + ex.Message, true); }
            finally { busy = false; RefreshState(); }
        }

        protected override bool ProcessCmdKey(ref Message msg, Keys keyData)
        {
            if (keyData == (Keys.Alt | Keys.L)) { la.Button.PerformClick(); return true; }
            if (keyData == (Keys.Alt | Keys.C)) { china.Button.PerformClick(); return true; }
            return base.ProcessCmdKey(ref msg, keyData);
        }

        protected override void Dispose(bool disposing)
        {
            if (disposing) { if (timer != null) timer.Dispose(); if (tip != null) tip.Dispose(); }
            base.Dispose(disposing);
        }
    }
}
