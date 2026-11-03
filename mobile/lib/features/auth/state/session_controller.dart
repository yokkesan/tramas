import 'dart:async';
import 'dart:convert';
import 'dart:math';

import 'package:flutter/foundation.dart';
import '../../../core/network/api_client.dart';
import '../data/auth_repository.dart';
import '../data/session_vault.dart';
import '../models/session.dart';

enum SessionPhase { restoring, signedOut, ready, offline, logoutPending }

class SessionController extends ChangeNotifier implements AuthenticatedApi {
  SessionController({
    required this.api,
    required this.vault,
    DateTime Function()? now,
    Future<void> Function(Duration)? sleep,
  }) : now = now ?? (() => DateTime.now().toUtc()),
       sleep = sleep ?? Future<void>.delayed,
       repository = AuthRepository(api);
  final ApiClient api;
  final SessionVault vault;
  final AuthRepository repository;
  final DateTime Function() now;
  final Future<void> Function(Duration) sleep;
  SessionPhase phase = SessionPhase.restoring;
  AuthUser? user;
  ApiException? problem;
  SessionRecord? _record;
  Future<void>? _refreshing;
  bool _logoutRequested = false;
  bool _authBusy = false;
  bool _disposed = false;
  int _epoch = 0;
  int get epoch => _epoch;
  bool get authBusy => _authBusy;
  void _emit() {
    if (!_disposed) notifyListeners();
  }

  Future<void> restore() async {
    if (_authBusy) return;
    phase = SessionPhase.restoring;
    problem = null;
    _emit();
    try {
      _record = await vault.read();
      _logoutRequested = _record?.logoutPending ?? false;
      if (_record == null) {
        user = null;
        phase = SessionPhase.signedOut;
        _emit();
        return;
      }
      if (_logoutRequested) {
        phase = SessionPhase.logoutPending;
        _emit();
        return;
      }
      await reloadUser();
    } on ApiException catch (e) {
      problem = e;
      if (phase != SessionPhase.signedOut) phase = SessionPhase.offline;
      _emit();
    } catch (_) {
      problem = const ApiException('invalid_response');
      phase = SessionPhase.offline;
      _emit();
    }
  }

  Future<void> login(String email, String password) =>
      authenticate(() => repository.login(email, password));

  Future<void> authenticate(Future<TokenPair> Function() action) async {
    if (_authBusy || _logoutRequested) throw const ApiException('action_busy');
    _authBusy = true;
    _emit();
    try {
      final tokens = await action();
      final record = SessionRecord(tokens);
      await vault.write(record);
      _epoch++;
      _record = record;
      problem = null;
      phase = SessionPhase.restoring;
      _emit();
      try {
        await reloadUser();
      } on ApiException catch (e) {
        if (_record != null) {
          phase = SessionPhase.offline;
          problem = e;
          _emit();
        }
        rethrow;
      }
    } finally {
      _authBusy = false;
      _emit();
    }
  }

  Future<void> reloadUser() async {
    final r = await request('GET', '/api/v1/auth/me');
    try {
      user = AuthUser.fromJson((r.json['user'] as Map).cast<String, dynamic>());
    } catch (_) {
      throw const ApiException('invalid_response');
    }
    if (!_logoutRequested) {
      phase = SessionPhase.ready;
      problem = null;
      _emit();
    }
  }

  @override
  Future<ApiResponse> request(
    String method,
    String path, {
    Object? body,
    String? contentType,
  }) async {
    final current = _record;
    if (current == null || _logoutRequested) {
      throw const ApiException('session_expired', status: 401);
    }
    final generation = _epoch;
    if (current.pending != null ||
        !current.tokens.accessExpiry.isAfter(
          now().add(const Duration(seconds: 20)),
        )) {
      await refresh();
    }
    final access = _record?.tokens.access;
    if (access == null || generation != _epoch || _logoutRequested) {
      throw const ApiException('session_expired', status: 401);
    }
    try {
      return await _sendProtected(
        method,
        path,
        generation,
        access,
        body,
        contentType,
      );
    } on ApiException catch (e) {
      if (e.status != 401) {
        if (e.code == 'profile_required' && user != null) {
          user = user!.withoutProfile();
          _emit();
        }
        rethrow;
      }
      if (generation != _epoch || _logoutRequested) rethrow;
      if (_record?.tokens.access == access) await refresh();
      final token = _record?.tokens.access;
      if (token == null) {
        throw const ApiException('session_expired', status: 401);
      }
      try {
        return await _sendProtected(
          method,
          path,
          generation,
          token,
          body,
          contentType,
        );
      } on ApiException catch (retry) {
        if (retry.status == 401) {
          await _expire(const ApiException('session_expired', status: 401));
        }
        rethrow;
      }
    }
  }

  Future<ApiResponse> _sendProtected(
    String method,
    String path,
    int generation,
    String token,
    Object? body,
    String? contentType,
  ) async {
    final response = await api.send(
      method,
      path,
      body: body,
      token: token,
      contentType: contentType,
    );
    if (_epoch != generation || _logoutRequested) {
      throw const ApiException('session_expired', status: 401);
    }
    return response;
  }

  Future<void> refresh() {
    if (_refreshing != null) return _refreshing!;
    final future = _doRefresh();
    _refreshing = future;
    return future.whenComplete(() {
      if (identical(_refreshing, future)) _refreshing = null;
    });
  }

  Future<void> _doRefresh() async {
    var record = _record;
    if (record == null) {
      throw const ApiException('session_expired', status: 401);
    }
    if (!record.tokens.refreshExpiry.isAfter(now())) {
      await _expire(const ApiException('session_expired', status: 401));
      throw problem!;
    }
    final pending = record.pending ?? PendingRefresh(_randomId(), now());
    if (now().difference(pending.startedAt) >= const Duration(seconds: 55)) {
      await _expire(
        const ApiException('refresh_recovery_expired', status: 401),
      );
      throw problem!;
    }
    if (record.pending == null) {
      record = SessionRecord(
        record.tokens,
        pending: pending,
        logoutPending: record.logoutPending || _logoutRequested,
      );
      await vault.write(
        record,
      ); // Persist BEFORE sending; crash recovery reuses the operation ID.
      _record = record;
    }
    for (var attempt = 0; attempt < 2; attempt++) {
      try {
        final tokens = await repository.refresh(
          record.tokens.refresh,
          pending.requestId,
        );
        final updated = SessionRecord(
          tokens,
          logoutPending: record.logoutPending || _logoutRequested,
        );
        await vault.write(
          updated,
        ); // Both replacement tokens committed in one secure value.
        _record = updated;
        return;
      } on ApiException catch (e) {
        if (e.code == 'refresh_conflict') {
          final saved = await vault.read();
          if (saved != null &&
              !saved.logoutPending &&
              saved.pending == null &&
              saved.tokens.refresh != record.tokens.refresh) {
            _record = saved;
            return; // Never clear a pair successfully persisted by another request.
          }
        }
        if (e.status == 401) {
          await _expire(const ApiException('session_expired', status: 401));
        }
        if (attempt == 0 &&
            e.transient &&
            e.status != 429 &&
            e.code != 'secure_storage_error' &&
            now().difference(pending.startedAt) < const Duration(seconds: 50)) {
          await sleep(const Duration(milliseconds: 500));
          continue;
        }
        rethrow;
      }
    }
  }

  Future<void> logout() async {
    if (_authBusy) throw const ApiException('action_busy');
    _authBusy = true;
    _logoutRequested = true;
    phase = SessionPhase.logoutPending;
    _emit();
    try {
      if (_refreshing != null) await _refreshing;
      if (_record?.pending != null) await refresh();
      final record = _record;
      if (record == null) {
        await _expire(const ApiException('session_expired'));
        return;
      }
      final pending = SessionRecord(record.tokens, logoutPending: true);
      await vault.write(pending);
      _record = pending;
      await repository.logout(pending.tokens.refresh);
      await vault.clear();
      _epoch++;
      _record = null;
      user = null;
      _logoutRequested = false;
      problem = null;
      phase = SessionPhase.signedOut;
    } on ApiException catch (e) {
      problem = e;
      if (_record != null) {
        final pending = SessionRecord(
          _record!.tokens,
          pending: _record!.pending,
          logoutPending: true,
        );
        try {
          await vault.write(pending);
        } on ApiException catch (storageError) {
          problem = storageError;
          rethrow;
        }
        _record = pending;
        phase = SessionPhase.logoutPending;
      }
      rethrow;
    } finally {
      _authBusy = false;
      _emit();
    }
  }

  Future<void> _expire(ApiException reason) async {
    _epoch++;
    _record = null;
    user = null;
    _logoutRequested = false;
    phase = SessionPhase.restoring;
    problem = reason;
    _emit();
    try {
      await vault.clear();
      phase = SessionPhase.signedOut;
    } on ApiException catch (e) {
      phase = SessionPhase.offline;
      problem = e;
      rethrow;
    } finally {
      _emit();
    }
  }

  Future<void> forgetLocalSession() async =>
      _expire(const ApiException('session_expired'));
  String _randomId() {
    final r = Random.secure();
    return base64UrlEncode(
      List<int>.generate(32, (_) => r.nextInt(256)),
    ).replaceAll('=', '');
  }

  @override
  void dispose() {
    _disposed = true;
    super.dispose();
  }
}
