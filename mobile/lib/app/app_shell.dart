import 'package:flutter/material.dart';
import '../core/theme/app_colors.dart';
import '../core/widgets/feature_widgets.dart';
import '../features/favorites/screens/favorites_screen.dart';
import '../features/profile/screens/profile_screen.dart';

class AppShell extends StatefulWidget {
  const AppShell({super.key});
  @override
  State<AppShell> createState() => _AppShellState();
}

class _AppShellState extends State<AppShell> {
  int _index = 0;
  @override
  Widget build(BuildContext context) => Scaffold(
    body: switch (_index) {
      0 => FeatureScaffold(
        title: 'tramas',
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Container(
              width: double.infinity,
              padding: const EdgeInsets.all(24),
              decoration: BoxDecoration(
                gradient: const LinearGradient(
                  colors: [Color(0xff111111), Color(0xff413a23)],
                ),
                borderRadius: BorderRadius.circular(14),
              ),
              child: const Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'TIGERS FAN COMMUNITY',
                    style: TextStyle(color: AppColors.primary, fontSize: 11),
                  ),
                  SizedBox(height: 12),
                  Text(
                    '今日も、虎とともに。',
                    style: TextStyle(
                      color: Colors.white,
                      fontSize: 23,
                      fontWeight: FontWeight.w900,
                    ),
                  ),
                  Text(
                    '試合も、記録も、ファンの声も。',
                    style: TextStyle(color: Colors.white70, fontSize: 11),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 16),
            FeatureCard(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text(
                    'あなたのtramas',
                    style: TextStyle(fontWeight: FontWeight.w800),
                  ),
                  const SizedBox(height: 12),
                  Wrap(
                    spacing: 8,
                    runSpacing: 8,
                    children: [
                      ActionButton(
                        'マイページ',
                        onPressed: () => setState(() => _index = 4),
                      ),
                      ActionButton(
                        '推し選手',
                        onPressed: () => setState(() => _index = 3),
                      ),
                    ],
                  ),
                  TextButton(
                    onPressed: () => Navigator.of(context).push(
                      MaterialPageRoute<void>(
                        builder: (_) => const PublicProfileLookupScreen(),
                      ),
                    ),
                    child: const Text('公開プロフィールを見る'),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
      1 => const FeatureScaffold(
        title: '試合',
        child: FeatureCard(child: Text('試合情報の機能は準備中です。')),
      ),
      2 => const FeatureScaffold(
        title: 'SNS',
        child: FeatureCard(child: Text('SNS機能は準備中です。')),
      ),
      3 => const FavoritesScreen(),
      _ => const ProfileScreen(),
    },
    bottomNavigationBar: BottomNavigationBar(
      currentIndex: _index,
      onTap: (value) => setState(() => _index = value),
      type: BottomNavigationBarType.fixed,
      backgroundColor: AppColors.header,
      selectedItemColor: AppColors.primary,
      unselectedItemColor: Colors.grey,
      selectedFontSize: 10,
      unselectedFontSize: 10,
      items: const [
        BottomNavigationBarItem(icon: Icon(Icons.home_outlined), label: 'ホーム'),
        BottomNavigationBarItem(
          icon: Icon(Icons.scoreboard_outlined),
          label: '試合',
        ),
        BottomNavigationBarItem(icon: Icon(Icons.forum_outlined), label: 'SNS'),
        BottomNavigationBarItem(
          icon: Icon(Icons.sports_baseball_outlined),
          label: '選手',
        ),
        BottomNavigationBarItem(
          icon: Icon(Icons.person_outline),
          label: 'マイページ',
        ),
      ],
    ),
  );
}
