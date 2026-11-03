import 'package:flutter/material.dart';
import '../../../app/app_scope.dart';
import '../../../core/widgets/feature_widgets.dart';
import '../state/favorites_controller.dart';
import 'history_screen.dart';

class FavoritesScreen extends StatefulWidget {
  const FavoritesScreen({super.key});
  @override
  State<FavoritesScreen> createState() => _FavoritesScreenState();
}

class _FavoritesScreenState extends State<FavoritesScreen> {
  FavoritesController? _controller;
  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _controller ??= FavoritesController(AppScope.of(context).favorites)..load();
  }

  @override
  void dispose() {
    _controller?.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: _controller!,
    builder: (context, _) {
      final c = _controller!;
      return FeatureScaffold(
        title: '推し選手',
        actions: [
          IconButton(
            onPressed: c.loading || c.busy ? null : c.load,
            tooltip: '最新の情報に更新',
            icon: const Icon(Icons.refresh),
          ),
        ],
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('最大3人、メインは1人。現在 ${c.favorites.length}/3人'),
            const SizedBox(height: 12),
            if (c.loading) const Center(child: CircularProgressIndicator()),
            ErrorText(c.error),
            if (!c.loading && c.error != null)
              ActionButton('再読み込み', onPressed: c.load),
            FeatureCard(
              child: Column(
                children: [
                  if (c.favorites.isEmpty) const Text('推し選手はまだ登録されていません。'),
                  ...c.favorites.map(
                    (f) => ListTile(
                      contentPadding: EdgeInsets.zero,
                      title: Text(f.player.name),
                      subtitle: Text(f.main ? 'メイン' : '登録中'),
                      leading: IconButton(
                        tooltip: f.main ? 'メインを解除' : 'メインに設定',
                        onPressed: c.busy || c.loading || !c.loaded
                            ? null
                            : () => c.setMain(f.main ? null : f.player.id),
                        icon: Icon(f.main ? Icons.star : Icons.star_border),
                      ),
                      trailing: IconButton(
                        tooltip: '推しを解除',
                        onPressed: c.busy || c.loading || !c.loaded
                            ? null
                            : () => c.toggle(f.player),
                        icon: const Icon(Icons.remove_circle_outline),
                      ),
                    ),
                  ),
                ],
              ),
            ),
            const Text(
              '登録できる現役阪神選手',
              style: TextStyle(fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 12),
            if (!c.loading && c.error == null && c.players.isEmpty)
              const FeatureCard(
                child: Text('現在、登録できる選手データはありません。データの公開をお待ちください。'),
              ),
            ...c.players.map((p) {
              final selected = c.favorites.any((f) => f.player.id == p.id);
              return FeatureCard(
                child: ListTile(
                  contentPadding: EdgeInsets.zero,
                  title: Text(p.name),
                  trailing: ActionButton(
                    selected ? '解除' : '追加',
                    busy: c.busy || c.loading || !c.loaded,
                    onPressed: !selected && c.favorites.length >= 3
                        ? null
                        : () => c.toggle(p),
                  ),
                ),
              );
            }),
            Wrap(
              spacing: 8,
              children: [
                OutlinedButton(
                  onPressed: () => Navigator.of(context).push(
                    MaterialPageRoute<void>(
                      builder: (_) => const HistoryScreen(),
                    ),
                  ),
                  child: const Text('過去の推し履歴'),
                ),
                OutlinedButton(
                  onPressed: () => Navigator.of(context).push(
                    MaterialPageRoute<void>(
                      builder: (_) => const HistoryScreen(main: true),
                    ),
                  ),
                  child: const Text('過去のメイン履歴'),
                ),
              ],
            ),
          ],
        ),
      );
    },
  );
}
