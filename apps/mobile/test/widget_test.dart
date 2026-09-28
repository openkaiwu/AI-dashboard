import 'package:flutter_test/flutter_test.dart';
import 'package:aihub_mobile/api.dart';

void main() {
  test('Server Profile requires HTTPS outside loopback', () {
    expect(
        validateServer('https://hub.example.com/'), 'https://hub.example.com');
    expect(validateServer('http://127.0.0.1:8080'), 'http://127.0.0.1:8080');
    for (final url in [
      'http://192.168.1.2:8080',
      'https://user:pass@hub.example.com',
      'https://hub.example.com/api',
      'https://hub.example.com?token=x'
    ]) {
      expect(() => validateServer(url), throwsArgumentError);
    }
  });
}
