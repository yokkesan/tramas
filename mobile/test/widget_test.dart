import 'package:flutter_test/flutter_test.dart';
import 'package:tramas/config/app_config.dart';
import 'package:tramas/main.dart';

void main() {
  testWidgets('初期画面を表示する', (tester) async {
    await tester.pumpWidget(
      TramasApp(
        config: AppConfig.fromValues(
          environment: 'development',
          baseUrl: 'http://127.0.0.1:8080',
        ),
      ),
    );
    expect(find.text('tramas'), findsOneWidget);
    expect(find.text('接続を確認'), findsOneWidget);
    expect(find.text('未確認'), findsOneWidget);
  });

  test('release requires a production HTTPS URL', () {
    expect(
      () => AppConfig.fromValues(
        environment: 'production',
        baseUrl: 'http://localhost',
        release: true,
      ),
      throwsArgumentError,
    );
    expect(
      () => AppConfig.fromValues(
        environment: 'development',
        baseUrl: 'https://localhost',
        release: true,
      ),
      throwsArgumentError,
    );
    expect(
      AppConfig.fromValues(
        environment: 'production',
        baseUrl: 'https://localhost',
        release: true,
      ).apiBaseUrl.scheme,
      'https',
    );
  });
}
