import 'package:flutter/material.dart';
import '../../../app/app_scope.dart';
import '../../../core/widgets/feature_widgets.dart';

class HistoryScreen extends StatefulWidget {
  const HistoryScreen({super.key, this.main = false});
  final bool main;
  @override
  State<HistoryScreen> createState() => _HistoryScreenState();
}

class _HistoryScreenState extends State<HistoryScreen> {
  final List<Map<String, dynamic>> _entries = [];
  String _cursor = '';
  bool _loading = false, _started = false, _more = true;
  Object? _error;
  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (!_started) {
      _started = true;
      _load();
    }
  }

  Future<void> _load() async {
    if (_loading) return;
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final page = await AppScope.of(
        context,
      ).favorites.history(main: widget.main, cursor: _cursor);
      if (!mounted) return;
      setState(() {
        final ids = _entries.map((e) => e['id']).toSet();
        _entries.addAll(page.entries.where((e) => !ids.contains(e['id'])));
        _cursor = page.nextCursor;
        _more = _cursor.isNotEmpty;
      });
    } catch (e) {
      if (mounted) setState(() => _error = e);
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) => FeatureScaffold(
    title: widget.main ? 'メイン履歴' : '推し履歴',
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Text('この履歴は本人にだけ表示されます。'),
        const SizedBox(height: 12),
        if (_entries.isEmpty && !_loading && _error == null)
          const FeatureCard(child: Text('履歴はまだありません。')),
        ..._entries.map((e) {
          final player = e['player'] as Map?;
          final name = player?['name'] as String? ?? '選手ID: ${e['player_id']}';
          final end = e['ended_at'];
          return FeatureCard(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(name, style: const TextStyle(fontWeight: FontWeight.w800)),
                Text('開始: ${e[widget.main ? 'started_at' : 'registered_at']}'),
                Text(end == null ? '継続中' : '終了: $end'),
                if (e['end_reason'] != null)
                  Text(
                    e['end_reason'] == 'ineligible'
                        ? '所属・現役状況の変更により終了'
                        : '登録解除',
                  ),
              ],
            ),
          );
        }),
        ErrorText(_error),
        if (_loading) const Center(child: CircularProgressIndicator()),
        if (!_loading && (_more || _error != null))
          ActionButton(_error == null ? '続きを読み込む' : '再試行', onPressed: _load),
      ],
    ),
  );
}
