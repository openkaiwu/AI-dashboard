import 'display_time.dart';
import 'dart:math' as math;
import 'package:flutter/material.dart';
import 'api.dart';
import 'store.dart' show Json;
import 'theme.dart';

String consumptionFreshness(Json data) {
  final observed = DateTime.tryParse(data['data_as_of'] as String? ?? '');
  final time = observed == null
      ? '采集时间未知'
      : '数据截至 ${formatLocalTimestamp(data['data_as_of'])}';
  const sources = {
    'server_rollup': '服务器采样',
    'local_backup': '本机备份聚合',
    'merged': '服务器与本机备份合并',
    'none': '暂无采集来源'
  };
  final source = sources[data['source']];
  final stale =
      data['stale_reason'] == 'stale_source' ? ' · 数据已滞后，请检查采集或同步状态' : '';
  return '$time${source == null ? '' : ' · 来源：$source'}$stale';
}

class ConsumptionPoint {
  final String label;
  final double? value;
  const ConsumptionPoint(this.label, this.value);
}

// Calendar gaps stay unknown; a successful sample with zero consumption is zero.
List<ConsumptionPoint> consumptionPoints(Json? data, int days,
    {bool hourly = false, DateTime? now}) {
  final current = (now ?? DateTime.now()).toUtc();
  final offset =
      Duration(minutes: (data?['tz_offset_minutes'] as num?)?.toInt() ?? 0);
  if (hourly) {
    final end =
        DateTime.utc(current.year, current.month, current.day, current.hour);
    final values = <int, double>{};
    for (final row in (data?['hourly'] as List?) ?? []) {
      final at = DateTime.tryParse(row['hour'] as String? ?? '')?.toUtc();
      final value = row['consumed_pp'] as num?;
      if (at == null ||
          value == null ||
          !value.isFinite ||
          value < 0 ||
          ((row['samples'] as num?) ?? 0) <= 0) {
        continue;
      }
      final hour = DateTime.utc(at.year, at.month, at.day, at.hour)
          .millisecondsSinceEpoch;
      values[hour] = (values[hour] ?? 0) + value.toDouble();
    }
    return List.generate(24, (i) {
      final at = end.subtract(Duration(hours: 23 - i));
      return ConsumptionPoint(
          '${at.add(offset).hour.toString().padLeft(2, '0')}:00',
          values[at.millisecondsSinceEpoch]);
    });
  }
  final local = current.add(offset);
  final end = DateTime.utc(local.year, local.month, local.day);
  final rows = {
    for (final row in (data?['series'] as List?) ?? []) row['bucket_start']: row
  };
  return List.generate(days, (i) {
    final key = end
        .subtract(Duration(days: days - i - 1))
        .toIso8601String()
        .substring(0, 10);
    final row = rows[key];
    final value = row?['consumed_pp'] as num?;
    return ConsumptionPoint(
        key.substring(5).replaceAll('-', '/'),
        value != null &&
                value.isFinite &&
                value >= 0 &&
                ((row?['coverage_hours'] as num?) ?? 0) > 0
            ? value.toDouble()
            : null);
  });
}

double? consumptionTotal(List<ConsumptionPoint> points) {
  final known = points.where((p) => p.value != null);
  return known.isEmpty
      ? null
      : known.fold<double>(0, (sum, p) => sum + p.value!);
}

class ConsumptionPanel extends StatefulWidget {
  final HubApi api;
  final String plan;
  final VoidCallback onConnections;
  const ConsumptionPanel(
      {super.key,
      required this.api,
      required this.plan,
      required this.onConnections});
  @override
  State<ConsumptionPanel> createState() => _ConsumptionPanelState();
}

class _ConsumptionPanelState extends State<ConsumptionPanel> {
  final data = <String, Json>{};
  final errors = <String>{};
  final pending = <String>{};
  int generation = 0;
  int range = 7;
  int? selectedPoint;
  String basis = '10080';
  bool get five => widget.plan == 'plus';
  List<String> get keys => ['hourly', 'daily', if (five) 'five'];

  @override
  void initState() {
    super.initState();
    reload();
  }

  @override
  void didUpdateWidget(ConsumptionPanel oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.plan != widget.plan) {
      basis = '10080';
      reload(clear: true);
    }
  }

  @override
  void dispose() {
    generation++;
    super.dispose();
  }

  Future<void> reload({bool clear = false}) async {
    final requestGeneration = ++generation;
    if (clear) {
      data.clear();
      errors.clear();
    }
    setState(() {
      pending.clear();
      pending.addAll(keys);
    });
    await Future.wait(keys.map((key) async {
      final query = key == 'five'
          ? 'granularity=window&window=300'
          : key == 'hourly'
              ? 'granularity=hourly&hours=24&window=$basis'
              : 'granularity=daily&days=30&window=$basis';
      try {
        final result = await widget.api
            .call('GET', '/api/v1/codex/consumption?$query')
            .timeout(const Duration(seconds: 15));
        final field = key == 'five'
            ? 'generations'
            : key == 'hourly'
                ? 'hourly'
                : 'series';
        if (result[field] is! List) {
          throw const FormatException('Missing consumption series');
        }
        if (!mounted || requestGeneration != generation) return;
        setState(() {
          data[key] = result;
          if (result['_offline'] == true) {
            errors.add(key);
          } else {
            errors.remove(key);
          }
          pending.remove(key);
        });
      } catch (_) {
        if (!mounted || requestGeneration != generation) return;
        setState(() {
          errors.add(key);
          pending.remove(key);
        });
      }
    }));
  }

  String label(int value) => value == 1 ? '近 24 小时' : '近 $value 天';
  String keyFor(int value) => value == 1 ? 'hourly' : 'daily';
  List<ConsumptionPoint> pointsFor(int value) =>
      consumptionPoints(data[keyFor(value)], value, hourly: value == 1);
  String stamp(String value) =>
      DateTime.tryParse(value)?.toLocal().toString().substring(0, 16) ?? value;

  Widget placeholder(bool failed) => Padding(
        padding: const EdgeInsets.symmetric(vertical: 26, horizontal: 8),
        child: Column(children: [
          Icon(failed ? Icons.cloud_off_outlined : Icons.bar_chart_rounded,
              size: 34, color: HubTheme.muted),
          const SizedBox(height: 14),
          Text(failed ? '暂时无法获取消耗数据' : '这段时间还没有消耗样本',
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 10),
          Text(failed ? '请检查服务器连接后重试，当前无法判断消耗量。' : '电脑在线并完成采集后，这里会显示趋势。',
              textAlign: TextAlign.center,
              style: const TextStyle(color: HubTheme.muted, fontSize: 12)),
          const SizedBox(height: 16),
          Wrap(
              spacing: 8,
              runSpacing: 8,
              alignment: WrapAlignment.center,
              children: [
                FilledButton.icon(
                    onPressed: pending.isEmpty ? reload : null,
                    icon: const Icon(Icons.refresh, size: 16),
                    label: Text(failed ? '重新加载' : '刷新数据')),
                TextButton(
                    onPressed: widget.onConnections,
                    child: const Text('检查电脑连接')),
              ]),
        ]),
      );

  Widget fiveCard() {
    Json? point;
    int? reset;
    for (final g in (data['five']?['generations'] as List?) ?? []) {
      for (final p in (g['points'] as List?) ?? []) {
        final used = p['used'] as num?;
        if (used != null &&
            used.isFinite &&
            used >= 0 &&
            used <= 100 &&
            DateTime.tryParse(p['t'] as String? ?? '') != null) {
          point = p as Json;
          reset = (g['resets_at'] as num?)?.toInt();
        }
      }
    }
    final expired =
        reset != null && reset * 1000 <= DateTime.now().millisecondsSinceEpoch;
    return Card(
        child: Padding(
            padding: const EdgeInsets.all(16),
            child:
                Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              const Row(children: [
                Icon(Icons.schedule, size: 16, color: HubTheme.accent),
                SizedBox(width: 7),
                Text('5 小时窗口 · Plus')
              ]),
              const SizedBox(height: 10),
              Text(
                  point == null || expired
                      ? '—'
                      : '${(100 - (point['used'] as num)).toStringAsFixed(0)}% 剩余',
                  style: Theme.of(context).textTheme.headlineSmall),
              const SizedBox(height: 7),
              Text(
                  errors.contains('five')
                      ? (point == null ? '获取失败' : '刷新失败 · 上次数据')
                      : expired
                          ? '已到重置时间，等待新采集'
                          : point != null
                              ? '最近采集 ${stamp(point['t'] as String)}'
                              : pending.contains('five')
                                  ? '正在读取额度…'
                                  : '暂无窗口样本',
                  style: const TextStyle(color: HubTheme.muted, fontSize: 11)),
            ])));
  }

  @override
  Widget build(BuildContext context) {
    final selectedKey = keyFor(range);
    final points = pointsFor(range);
    final total = consumptionTotal(points);
    final known = points.where((p) => p.value != null).length;
    final allFailed =
        keys.every((key) => errors.contains(key) && !data.containsKey(key)) &&
            pending.isEmpty;
    final loading =
        pending.contains(selectedKey) && !data.containsKey(selectedKey);
    final selected = selectedPoint == null
        ? points.last
        : points[selectedPoint!.clamp(0, points.length - 1)];
    final maxValue = math.max(
            1.0, points.fold<double>(0, (v, p) => math.max(v, p.value ?? 0))) *
        1.15;
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Row(children: [
        if (five)
          Expanded(
              child: DropdownButton<String>(
                  value: basis,
                  isExpanded: true,
                  items: const [
                    DropdownMenuItem(value: '10080', child: Text('统计周额度')),
                    DropdownMenuItem(value: '300', child: Text('统计 5 小时额度'))
                  ],
                  onChanged: (value) {
                    if (value == null) return;
                    basis = value;
                    reload(clear: true);
                  }))
        else
          const Expanded(
              child:
                  Text('周额度消耗', style: TextStyle(fontWeight: FontWeight.w600))),
        TextButton.icon(
            onPressed: pending.isEmpty ? reload : null,
            icon: const Icon(Icons.refresh, size: 16),
            label: Text(pending.isEmpty ? '刷新数据' : '更新中…')),
      ]),
      const SizedBox(height: 10),
      if (allFailed)
        Card(child: placeholder(true))
      else ...[
        if (five) fiveCard(),
        Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            for (final value in [1, 7, 30])
              Expanded(
                  child: Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 3),
                      child: Material(
                          color: range == value
                              ? const Color(0xffedf2ff)
                              : Colors.white,
                          shape: RoundedRectangleBorder(
                              borderRadius: BorderRadius.circular(12),
                              side: BorderSide(
                                  color: range == value
                                      ? const Color(0xff9eb6f5)
                                      : HubTheme.line)),
                          child: InkWell(
                              borderRadius: BorderRadius.circular(12),
                              onTap: () => setState(() {
                                    range = value;
                                    selectedPoint = null;
                                  }),
                              child: Padding(
                                  padding: const EdgeInsets.symmetric(
                                      vertical: 14, horizontal: 9),
                                  child: Column(
                                      crossAxisAlignment:
                                          CrossAxisAlignment.start,
                                      children: [
                                        Text(label(value),
                                            style: const TextStyle(
                                                fontSize: 11,
                                                color: HubTheme.muted)),
                                        const SizedBox(height: 10),
                                        Text(
                                            pending.contains(keyFor(value)) &&
                                                    !data.containsKey(
                                                        keyFor(value))
                                                ? '…'
                                                : consumptionTotal(
                                                            pointsFor(value))
                                                        ?.toStringAsFixed(1) ??
                                                    '—',
                                            style: const TextStyle(
                                                fontSize: 24,
                                                fontWeight: FontWeight.w600,
                                                color: HubTheme.ink)),
                                        const SizedBox(height: 5),
                                        Text(
                                            errors.contains(keyFor(value))
                                                ? (data.containsKey(
                                                        keyFor(value))
                                                    ? '上次数据'
                                                    : '获取失败')
                                                : consumptionTotal(
                                                            pointsFor(value)) ==
                                                        null
                                                    ? '等待样本'
                                                    : '个百分点',
                                            style: const TextStyle(
                                                fontSize: 10,
                                                color: HubTheme.muted)),
                                      ]))))))
          ],
        ),
        if (errors.isNotEmpty)
          const Padding(
              padding: EdgeInsets.only(top: 12),
              child: Text('部分数据更新失败；保留已有数据，缺失数据以“—”显示。',
                  style: TextStyle(fontSize: 12, color: HubTheme.warn))),
        const SizedBox(height: 14),
        Card(
            child: Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      Text('消耗趋势',
                          style: Theme.of(context).textTheme.titleMedium),
                      const SizedBox(height: 6),
                      Text(
                          '${basis == '300' ? '5 小时额度' : '周额度'} · ${label(range)}${range == 1 ? '' : '（含今天）'}',
                          style: const TextStyle(
                              fontSize: 11, color: HubTheme.muted)),
                      const SizedBox(height: 18),
                      if (loading)
                        const Padding(
                            padding: EdgeInsets.all(30),
                            child: Column(children: [
                              CircularProgressIndicator(),
                              SizedBox(height: 12),
                              Text('正在读取消耗记录…')
                            ]))
                      else if (!data.containsKey(selectedKey) &&
                          errors.contains(selectedKey))
                        placeholder(true)
                      else if (total == null)
                        placeholder(false)
                      else ...[
                        const Text('单位：百分点（pp） · 灰线表示无样本',
                            style:
                                TextStyle(fontSize: 10, color: HubTheme.muted)),
                        const SizedBox(height: 12),
                        SizedBox(
                            height: 160,
                            child: Row(
                                crossAxisAlignment: CrossAxisAlignment.end,
                                children: [
                                  for (var i = 0; i < points.length; i++)
                                    Expanded(
                                        child: Semantics(
                                            label:
                                                '${points[i].label} ${points[i].value == null ? '无样本' : '消耗 ${points[i].value!.toStringAsFixed(1)} 个百分点'}',
                                            button: true,
                                            child: InkWell(
                                                onTap: () => setState(() =>
                                                    selectedPoint = i),
                                                child: Align(
                                                    alignment: Alignment
                                                        .bottomCenter,
                                                    child: Container(
                                                        margin:
                                                            const EdgeInsets.symmetric(
                                                                horizontal: 2),
                                                        height: points[i].value ==
                                                                null
                                                            ? 2
                                                            : math.max(
                                                                2.0,
                                                                points[i].value! /
                                                                    maxValue *
                                                                    160),
                                                        decoration:
                                                            BoxDecoration(
                                                                color: points[i].value ==
                                                                        null
                                                                    ? HubTheme
                                                                        .line
                                                                    : selectedPoint ==
                                                                            i
                                                                        ? HubTheme
                                                                            .accent
                                                                        : const Color(
                                                                            0xff8ba7f3),
                                                                borderRadius:
                                                                    const BorderRadius.vertical(
                                                                        top: Radius.circular(4))))))))
                                ])),
                        const SizedBox(height: 9),
                        Row(
                            mainAxisAlignment: MainAxisAlignment.spaceBetween,
                            children: [
                              Text(points.first.label,
                                  style: const TextStyle(
                                      fontSize: 10, color: HubTheme.muted)),
                              Text(points.last.label,
                                  style: const TextStyle(
                                      fontSize: 10, color: HubTheme.muted))
                            ]),
                        const SizedBox(height: 14),
                        Text(
                            '${selected.label} · ${selected.value == null ? '暂无采集样本' : '${selected.value!.toStringAsFixed(1)} pp'}',
                            style: const TextStyle(fontSize: 12)),
                        const Divider(height: 28),
                        Text(
                            '已记录 ${total.toStringAsFixed(1)} pp · $known/${points.length} ${range == 1 ? '小时' : '天'}有样本',
                            style: const TextStyle(fontSize: 12)),
                        if (known < points.length)
                          const Padding(
                              padding: EdgeInsets.only(top: 8),
                              child: Text('存在采集空档，合计仅包含已记录部分。',
                                  style: TextStyle(
                                      fontSize: 11, color: HubTheme.muted))),
                      ],
                      if (data[selectedKey] != null)
                        Padding(
                            padding: const EdgeInsets.only(top: 15),
                            child: Text(
                                '${errors.contains(selectedKey) ? '刷新失败，保留上次数据 · ' : ''}${consumptionFreshness(data[selectedKey]!)}',
                                style: const TextStyle(
                                    fontSize: 10, color: HubTheme.muted))),
                    ]))),
      ],
      const SizedBox(height: 12),
      const Text('1 个百分点 = 本窗口完整额度的 1%。重置不算消耗，空档不代表零消耗。',
          style: TextStyle(fontSize: 11, color: HubTheme.muted)),
      const SizedBox(height: 20),
    ]);
  }
}
