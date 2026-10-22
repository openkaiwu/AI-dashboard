import 'package:flutter_test/flutter_test.dart';
import 'package:aihub_mobile/consumption_panel.dart';

void main() {
  test('response generation time never becomes collection time', () {
    expect(consumptionFreshness({'generated_at': '2099-12-31T12:00:00Z'}),
        '采集时间未知');
    expect(consumptionFreshness({'data_as_of': 'invalid'}), '采集时间未知');
  });
  test('observation time and source render without generation time', () {
    final label = consumptionFreshness({
      'data_as_of': '2026-10-22T00:00:00Z',
      'source': 'server_rollup',
      'stale_reason': 'stale_source'
    });
    expect(label, contains('数据截至'));
    expect(label, contains('服务器采样'));
    expect(label, contains('数据已滞后'));
  });
}
