class ProfileImage {
  const ProfileImage(this.id, this.contentType);
  final String id, contentType;
  factory ProfileImage.fromJson(Map<String, dynamic> j) =>
      ProfileImage(j['id'] as String, j['content_type'] as String);
}

class UserProfile {
  const UserProfile({
    required this.id,
    required this.biography,
    this.nickname,
    this.avatar,
    this.header,
    this.avatarStandard,
    this.headerStandard,
  });
  final String id, biography;
  final String? nickname, avatarStandard, headerStandard;
  final ProfileImage? avatar, header;
  factory UserProfile.fromJson(Map<String, dynamic> j) => UserProfile(
    id: j['id'] as String,
    nickname: j['nickname'] as String?,
    biography: j['biography'] as String,
    avatar: j['avatar'] == null
        ? null
        : ProfileImage.fromJson((j['avatar'] as Map).cast<String, dynamic>()),
    header: j['header'] == null
        ? null
        : ProfileImage.fromJson((j['header'] as Map).cast<String, dynamic>()),
    avatarStandard: j['avatar_standard_id'] as String?,
    headerStandard: j['header_standard_id'] as String?,
  );
}

class StandardImage {
  const StandardImage(this.id, this.kind, this.url);
  final String id, kind, url;
  factory StandardImage.fromJson(Map<String, dynamic> j) =>
      StandardImage(j['id'] as String, j['kind'] as String, j['url'] as String);
}

String normalizeNickname(String value) =>
    value.replaceAll(RegExp(r'^ +| +$'), '');
bool validNickname(String value) {
  final v = normalizeNickname(value);
  final runes = v.runes.toList();
  if (runes.length < 2 ||
      runes.length > 20 ||
      runes.any(
        (r) =>
            r <= 31 ||
            r >= 127 && r <= 159 ||
            r == 0xfffd ||
            r >= 0xd800 && r <= 0xdfff,
      )) {
    return false;
  }
  // Explicit Unicode White_Space set, independent of Dart String.trim semantics.
  return !runes.every(
    (r) =>
        r == 0x20 ||
        r == 0x85 ||
        r == 0xa0 ||
        r == 0x1680 ||
        r >= 0x2000 && r <= 0x200a ||
        const [0x2028, 0x2029, 0x202f, 0x205f, 0x3000].contains(r),
  );
}
