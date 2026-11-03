import 'dart:typed_data';
import '../../../core/network/api_client.dart';
import '../models/profile.dart';

class ProfileRepository {
  ProfileRepository(this.api);
  final AuthenticatedApi api;
  Future<UserProfile> get({String? userId}) async {
    if (userId != null &&
        !RegExp(
          r'^[a-fA-F0-9]{8}-(?:[a-fA-F0-9]{4}-){3}[a-fA-F0-9]{12}$',
        ).hasMatch(userId)) {
      throw const ApiException('not_found', status: 404);
    }
    final r = await api.request(
      'GET',
      '/api/v1/users/${userId ?? 'me'}/profile',
    );
    return UserProfile.fromJson(
      (r.json['profile'] as Map).cast<String, dynamic>(),
    );
  }

  Future<UserProfile> update({
    required String nickname,
    required String biography,
  }) async {
    if (!validNickname(nickname)) {
      throw const ApiException('invalid_nickname', status: 422);
    }
    final r = await api.request(
      'PUT',
      '/api/v1/users/me/profile',
      body: {'nickname': normalizeNickname(nickname), 'biography': biography},
    );
    return UserProfile.fromJson(
      (r.json['profile'] as Map).cast<String, dynamic>(),
    );
  }

  Future<List<StandardImage>> standards() async {
    final r = await api.request('GET', '/api/v1/profile/standard-images');
    return (r.json['images'] as List)
        .map((v) => StandardImage.fromJson((v as Map).cast<String, dynamic>()))
        .toList();
  }

  Future<Uint8List> image(String id) async => (await api.request(
    'GET',
    '/api/v1/profile/images/${Uri.encodeComponent(id)}',
  )).bytes;
  Future<void> upload(String kind, Uint8List bytes, String contentType) async {
    _kind(kind);
    if (bytes.length > 10 * 1024 * 1024) {
      throw const ApiException('image_too_large', status: 413);
    }
    await api.request(
      'PUT',
      '/api/v1/users/me/profile/images/$kind',
      body: bytes,
      contentType: contentType,
    );
  }

  Future<void> remove(String kind) async {
    _kind(kind);
    await api.request('DELETE', '/api/v1/users/me/profile/images/$kind');
  }

  Future<void> selectStandard(String kind, String id) async {
    _kind(kind);
    await api.request(
      'PUT',
      '/api/v1/users/me/profile/images/$kind/standard',
      body: {'image_id': id},
    );
  }

  void _kind(String kind) {
    if (kind != 'avatar' && kind != 'header') {
      throw const ApiException('invalid_request');
    }
  }
}
