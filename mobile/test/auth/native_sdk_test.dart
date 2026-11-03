import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tramas/config/app_config.dart';
import 'package:tramas/core/network/api_client.dart';
import 'package:tramas/features/auth/services/oidc_service.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test(
    'Android Google bridge receives server client ID and a per-attempt nonce',
    () async {
      debugDefaultTargetPlatformOverride = TargetPlatform.android;
      addTearDown(() => debugDefaultTargetPlatformOverride = null);
      const channel = MethodChannel('tramas/oidc');
      final nonces = <String>[];
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(channel, (call) async {
            expect(call.method, 'googleSignIn');
            expect(
              (call.arguments as Map)['client_id'],
              'test-android-server-id',
            );
            nonces.add((call.arguments as Map)['nonce'] as String);
            return 'android-id-token';
          });
      addTearDown(
        () => TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
            .setMockMethodCallHandler(channel, null),
      );
      final sdk = NativeIdentitySdk(
        AppConfig.fromValues(
          environment: 'development',
          baseUrl: 'http://127.0.0.1:8080',
          googleAndroidServerClientId: 'test-android-server-id',
        ),
      );
      expect(sdk.available('google'), true);
      await sdk.identityToken('google', 'attempt-1');
      await sdk.identityToken('google', 'attempt-2');
      expect(nonces, ['attempt-1', 'attempt-2']);
    },
  );
  test(
    'Google native AppAuth bridge receives a fresh exact nonce per request and PKCE authorization flow',
    () async {
      debugDefaultTargetPlatformOverride = TargetPlatform.iOS;
      addTearDown(() => debugDefaultTargetPlatformOverride = null);
      const channel = MethodChannel('crossingthestreams.io/flutter_appauth');
      final nonces = <String>[];
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(channel, (call) async {
            expect(call.method, 'authorizeAndExchangeCode');
            final args = call.arguments as Map;
            nonces.add(args['nonce'] as String);
            expect(args['clientId'], 'test-client-id');
            expect(args['redirectUrl'], 'com.example.oauth:/callback');
            expect(args['scopes'], ['openid', 'email']);
            expect(args['allowInsecureConnections'], false);
            return {'idToken': 'native-test-id-token'};
          });
      addTearDown(
        () => TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
            .setMockMethodCallHandler(channel, null),
      );
      final config = AppConfig.fromValues(
        environment: 'development',
        baseUrl: 'http://127.0.0.1:8080',
        googleClientId: 'test-client-id',
        googleRedirectUri: 'com.example.oauth:/callback',
      );
      final sdk = NativeIdentitySdk(config);
      expect(
        await sdk.identityToken('google', 'first-nonce'),
        'native-test-id-token',
      );
      expect(
        await sdk.identityToken('google', 'second-nonce'),
        'native-test-id-token',
      );
      expect(nonces, ['first-nonce', 'second-nonce']);
    },
  );
  test(
    'Apple native bridge preserves nonce/state and rejects a mismatched callback',
    () async {
      debugDefaultTargetPlatformOverride = TargetPlatform.iOS;
      addTearDown(() => debugDefaultTargetPlatformOverride = null);
      const channel = MethodChannel(
        'com.aboutyou.dart_packages.sign_in_with_apple',
      );
      var stateMatches = true;
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(channel, (call) async {
            expect(call.method, 'performAuthorizationRequest');
            final args = (call.arguments as List).first as Map;
            expect(args['nonce'], 'apple-nonce');
            expect(args['state'], 'apple-nonce');
            return {
              'type': 'appleid',
              'authorizationCode': 'native-code',
              'identityToken': 'native-apple-token',
              'state': stateMatches ? 'apple-nonce' : 'wrong-state',
            };
          });
      addTearDown(
        () => TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
            .setMockMethodCallHandler(channel, null),
      );
      final sdk = NativeIdentitySdk(
        AppConfig.fromValues(
          environment: 'development',
          baseUrl: 'http://127.0.0.1:8080',
          appleEnabled: true,
        ),
      );
      expect(
        await sdk.identityToken('apple', 'apple-nonce'),
        'native-apple-token',
      );
      stateMatches = false;
      await expectLater(
        sdk.identityToken('apple', 'apple-nonce'),
        throwsA(
          isA<ApiException>().having(
            (e) => e.code,
            'code',
            'external_auth_failed',
          ),
        ),
      );
    },
  );
  test('provider configuration and secure redirects fail closed', () {
    final config = AppConfig.fromValues(
      environment: 'development',
      baseUrl: 'http://127.0.0.1:8080',
    );
    final sdk = NativeIdentitySdk(config);
    expect(sdk.available('apple'), false);
    expect(sdk.available('google'), false);
    expect(
      () => AppConfig.fromValues(
        environment: 'production',
        baseUrl: 'https://api.example.com',
        googleRedirectUri: 'javascript:alert(1)',
      ),
      throwsArgumentError,
    );
    expect(
      () => AppConfig.fromValues(
        environment: 'production',
        baseUrl: 'https://api.example.com',
        appleRedirectUri: 'http://example.com/callback',
      ),
      throwsArgumentError,
    );
    debugDefaultTargetPlatformOverride = TargetPlatform.android;
    addTearDown(() => debugDefaultTargetPlatformOverride = null);
    final android = NativeIdentitySdk(
      AppConfig.fromValues(
        environment: 'development',
        baseUrl: 'http://127.0.0.1:8080',
        appleEnabled: true,
      ),
    );
    expect(android.available('apple'), false);
    final configured = NativeIdentitySdk(
      AppConfig.fromValues(
        environment: 'development',
        baseUrl: 'http://127.0.0.1:8080',
        appleEnabled: true,
        appleWebClientId: 'test-services-id',
        appleRedirectUri: 'https://example.com/apple/callback',
      ),
    );
    expect(configured.available('apple'), true);
  });
}
