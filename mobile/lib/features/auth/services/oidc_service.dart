import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';
import 'package:flutter_appauth/flutter_appauth.dart';
import 'package:sign_in_with_apple/sign_in_with_apple.dart';
import '../../../config/app_config.dart';
import '../../../core/network/api_client.dart';
import '../models/session.dart';
import '../state/session_controller.dart';

abstract interface class IdentitySdk {
  bool available(String provider);
  Future<String> identityToken(String provider, String nonce);
}

class NativeIdentitySdk implements IdentitySdk {
  NativeIdentitySdk(this.config);
  final AppConfig config;
  @override
  bool available(String provider) => switch (provider) {
    'apple' =>
      config.appleEnabled &&
          !kIsWeb &&
          (defaultTargetPlatform == TargetPlatform.iOS ||
              defaultTargetPlatform == TargetPlatform.macOS ||
              defaultTargetPlatform == TargetPlatform.android &&
                  config.appleWebClientId.isNotEmpty &&
                  config.appleRedirectUri.isNotEmpty),
    'google' =>
      defaultTargetPlatform == TargetPlatform.android
          ? config.googleAndroidServerClientId.isNotEmpty
          : config.googleEnabled,
    _ => false,
  };
  @override
  Future<String> identityToken(String provider, String nonce) async {
    if (!available(provider)) {
      throw const ApiException('provider_unavailable', status: 503);
    }
    try {
      String? token;
      if (provider == 'apple') {
        final credential = await SignInWithApple.getAppleIDCredential(
          scopes: [AppleIDAuthorizationScopes.email],
          nonce: nonce,
          state: nonce,
          webAuthenticationOptions:
              defaultTargetPlatform == TargetPlatform.android
              ? WebAuthenticationOptions(
                  clientId: config.appleWebClientId,
                  redirectUri: Uri.parse(config.appleRedirectUri),
                )
              : null,
        );
        if (credential.state != nonce) {
          throw const ApiException('external_auth_failed');
        }
        token = credential.identityToken;
      } else if (defaultTargetPlatform == TargetPlatform.android) {
        token = await const MethodChannel('tramas/oidc').invokeMethod<String>(
          'googleSignIn',
          {'client_id': config.googleAndroidServerClientId, 'nonce': nonce},
        );
      } else {
        final result = await const FlutterAppAuth().authorizeAndExchangeCode(
          AuthorizationTokenRequest(
            config.googleClientId,
            config.googleRedirectUri,
            discoveryUrl:
                'https://accounts.google.com/.well-known/openid-configuration',
            scopes: ['openid', 'email'],
            nonce: nonce,
            promptValues: ['select_account'],
          ),
        );
        token = result.idToken;
      }
      if (token == null || token.isEmpty) {
        throw const ApiException('external_auth_failed', status: 401);
      }
      return token;
    } on SignInWithAppleAuthorizationException catch (e) {
      throw ApiException(
        e.code == AuthorizationErrorCode.canceled
            ? 'provider_cancelled'
            : 'external_auth_failed',
      );
    } on FlutterAppAuthUserCancelledException {
      throw const ApiException('provider_cancelled');
    } on PlatformException catch (e) {
      throw ApiException(
        e.code == 'provider_cancelled'
            ? 'provider_cancelled'
            : 'external_auth_failed',
      );
    } on ApiException {
      rethrow;
    } catch (_) {
      throw const ApiException('external_auth_failed');
    }
  }
}

class OidcService {
  OidcService(this.session, this.sdk);
  final SessionController session;
  final IdentitySdk sdk;
  bool get enabled => !session.authBusy;
  Future<void> login(String provider) =>
      session.authenticate(() => _login(provider));
  Future<TokenPair> _login(String provider) async {
    final response = await _exchange(provider, 'login');
    return TokenPair.fromJson(
      (response.json['tokens'] as Map).cast<String, dynamic>(),
    );
  }

  Future<ApiResponse> _exchange(
    String provider,
    String purpose, {
    String? proof,
  }) async {
    final generation = session.epoch;
    if (!sdk.available(provider)) {
      throw const ApiException('provider_unavailable', status: 503);
    }
    final body = {'purpose': purpose, 'reauth_token': ?proof};
    final ApiResponse challenge;
    if (purpose == 'login') {
      challenge = await session.api.send(
        'POST',
        '/api/v1/auth/oidc/$provider/challenge',
        body: body,
      );
    } else {
      challenge = await session.request(
        'POST',
        '/api/v1/auth/oidc/$provider/challenge',
        body: body,
      );
    }
    final nonce = challenge.json['nonce'] as String;
    final token = await sdk.identityToken(provider, nonce);
    if (purpose != 'login' && session.epoch != generation) {
      throw const ApiException('session_expired', status: 401);
    }
    final completion = {
      'challenge_token': challenge.json['challenge_token'] as String,
      'id_token': token,
    };
    if (purpose == 'login') {
      return session.api.send(
        'POST',
        '/api/v1/auth/oidc/$provider/login',
        body: completion,
      );
    }
    return session.request(
      'POST',
      '/api/v1/auth/oidc/$provider/$purpose',
      body: completion,
    );
  }

  Future<String> passwordProof(String password) async {
    final r = await session.request(
      'POST',
      '/api/v1/auth/reauthenticate',
      body: {'password': password},
    );
    return r.json['reauth_token'] as String;
  }

  Future<String> providerProof(String provider) async =>
      (await _exchange(provider, 'reauth')).json['reauth_token'] as String;
  Future<void> link(String provider, String proof) async {
    await _exchange(provider, 'link', proof: proof);
  }
}
