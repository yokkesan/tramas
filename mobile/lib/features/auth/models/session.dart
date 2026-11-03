class AuthUser {
  const AuthUser({
    required this.id,
    required this.email,
    required this.profileCompleted,
    this.nickname,
  });
  final String id;
  final String email;
  final bool profileCompleted;
  final String? nickname;
  factory AuthUser.fromJson(Map<String, dynamic> j) => AuthUser(
    id: j['id'] as String,
    email: j['email'] as String,
    profileCompleted: j['profile_completed'] as bool,
    nickname: j['nickname'] as String?,
  );
  AuthUser withoutProfile() => AuthUser(
    id: id,
    email: email,
    profileCompleted: false,
    nickname: nickname,
  );
}

class TokenPair {
  const TokenPair(
    this.access,
    this.refresh,
    this.accessExpiry,
    this.refreshExpiry,
  );
  final String access, refresh;
  final DateTime accessExpiry, refreshExpiry;
  factory TokenPair.fromJson(Map<String, dynamic> j) => TokenPair(
    j['access_token'] as String,
    j['refresh_token'] as String,
    DateTime.parse(j['access_expires_at'] as String).toUtc(),
    DateTime.parse(j['refresh_expires_at'] as String).toUtc(),
  );
  Map<String, dynamic> toJson() => {
    'access_token': access,
    'refresh_token': refresh,
    'access_expires_at': accessExpiry.toIso8601String(),
    'refresh_expires_at': refreshExpiry.toIso8601String(),
  };
}

class PendingRefresh {
  const PendingRefresh(this.requestId, this.startedAt);
  final String requestId;
  final DateTime startedAt;
  Map<String, dynamic> toJson() => {
    'request_id': requestId,
    'started_at': startedAt.toIso8601String(),
  };
  factory PendingRefresh.fromJson(Map<String, dynamic> j) => PendingRefresh(
    j['request_id'] as String,
    DateTime.parse(j['started_at'] as String).toUtc(),
  );
}

class SessionRecord {
  const SessionRecord(this.tokens, {this.pending, this.logoutPending = false});
  final TokenPair tokens;
  final PendingRefresh? pending;
  final bool logoutPending;
  Map<String, dynamic> toJson() => {
    'tokens': tokens.toJson(),
    if (pending != null) 'pending': pending!.toJson(),
    'logout_pending': logoutPending,
  };
  factory SessionRecord.fromJson(Map<String, dynamic> j) => SessionRecord(
    TokenPair.fromJson((j['tokens'] as Map).cast<String, dynamic>()),
    pending: j['pending'] == null
        ? null
        : PendingRefresh.fromJson(
            (j['pending'] as Map).cast<String, dynamic>(),
          ),
    logoutPending: j['logout_pending'] == true,
  );
}
