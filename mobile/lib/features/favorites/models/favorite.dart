class Player {
  const Player(this.id, this.name, this.team, this.status);
  final String id, name, status;
  final String? team;
  bool get eligible => team == 'hanshin_tigers' && status == 'active';
  factory Player.fromJson(Map<String, dynamic> j) => Player(
    j['id'] as String,
    j['name'] as String,
    j['team'] as String?,
    j['status'] as String,
  );
}

class Favorite {
  const Favorite(
    this.id,
    this.player,
    this.main,
    this.registeredAt,
    this.endedAt,
    this.endReason,
  );
  final String id;
  final Player player;
  final bool main;
  final String registeredAt;
  final String? endedAt, endReason;
  factory Favorite.fromJson(Map<String, dynamic> j) => Favorite(
    j['id'] as String,
    Player.fromJson((j['player'] as Map).cast<String, dynamic>()),
    j['main'] as bool,
    j['registered_at'] as String,
    j['ended_at'] as String?,
    j['end_reason'] as String?,
  );
}

class HistoryPage {
  const HistoryPage(this.entries, this.nextCursor);
  final List<Map<String, dynamic>> entries;
  final String nextCursor;
}
