import 'dart:convert';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import '../../../core/network/api_client.dart';
import '../models/session.dart';

abstract interface class SessionVault {
  Future<SessionRecord?> read();
  Future<void> write(SessionRecord record);
  Future<void> clear();
}

class SecureSessionVault implements SessionVault {
  SecureSessionVault(Uri origin, {FlutterSecureStorage? storage})
    : _storage =
          storage ??
          const FlutterSecureStorage(
            iOptions: IOSOptions(
              accessibility: KeychainAccessibility.unlocked_this_device,
            ),
          ),
      _key = 'tramas.session.v1.${base64UrlEncode(utf8.encode(origin.origin))}';
  final FlutterSecureStorage _storage;
  final String _key;
  @override
  Future<SessionRecord?> read() async {
    try {
      final value = await _storage.read(key: _key);
      return value == null
          ? null
          : SessionRecord.fromJson(
              (jsonDecode(value) as Map).cast<String, dynamic>(),
            );
    } catch (_) {
      throw const ApiException('secure_storage_error');
    }
  }

  @override
  Future<void> write(SessionRecord record) async {
    try {
      await _storage.write(key: _key, value: jsonEncode(record.toJson()));
    } catch (_) {
      throw const ApiException('secure_storage_error');
    }
  }

  @override
  Future<void> clear() async {
    try {
      await _storage.delete(key: _key);
    } catch (_) {
      throw const ApiException('secure_storage_error');
    }
  }
}
