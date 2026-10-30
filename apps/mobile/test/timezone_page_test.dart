import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:aihub_mobile/timezone_page.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  const channel = MethodChannel('dev.aihub/timezone');
  testWidgets('page displays native state and passes a validated city selection', (tester) async {
    final calls = <MethodCall>[];
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(channel, (call) async {
      calls.add(call);
      if (call.method == 'status') return {'zone':'Asia/Shanghai','auto':false,'ready':true,'running':false,'message':''};
      if (call.method == 'switch') throw PlatformException(code:'cancel',message:'切换失败，请重试');
      return true;
    });
    await tester.pumpWidget(const MaterialApp(home:TimeZonePage()));
    await tester.pump();
    expect(find.text('Asia/Shanghai'),findsOneWidget);
    expect(find.text('时区切换助手已就绪'),findsOneWidget);
    await tester.tap(find.text('切换').first);
    await tester.pump();
    expect(calls.where((c)=>c.method=='switch').single.arguments,{'target':'la'});
    expect(find.text('切换失败，请重试'),findsOneWidget);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
    final countBefore = calls.where((c) => c.method == 'status').length;
    await tester.pump(const Duration(seconds: 4));
    expect(calls.where((c) => c.method == 'status').length, countBefore);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await tester.pumpWidget(const SizedBox());
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger.setMockMethodCallHandler(channel,null);
  });
}
