import 'package:flutter/material.dart';

import 'api/health_client.dart';
import 'config/app_config.dart';

void main() => runApp(TramasApp(config: AppConfig.fromEnvironment()));

class TramasApp extends StatelessWidget {
  const TramasApp({super.key, required this.config});
  final AppConfig config;

  @override
  Widget build(BuildContext context) => MaterialApp(
    title: 'tramas',
    theme: ThemeData(
      colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xffd4aa00)),
    ),
    home: HomePage(config: config),
  );
}

class HomePage extends StatefulWidget {
  const HomePage({super.key, required this.config});
  final AppConfig config;

  @override
  State<HomePage> createState() => _HomePageState();
}

class _HomePageState extends State<HomePage> {
  bool _loading = false;
  String _status = '未確認';

  Future<void> _check() async {
    setState(() {
      _loading = true;
    });
    var status = 'APIに接続できませんでした';
    try {
      if (await HealthClient().check(widget.config.apiBaseUrl)) {
        status = 'API・データベースに接続できました';
      }
    } catch (_) {
      // A generic message avoids displaying network or server details.
    }
    if (!mounted) return;
    setState(() {
      _loading = false;
      _status = status;
    });
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('tramas')),
    body: SafeArea(
      child: Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Text('阪神タイガースファンの共有アプリ'),
              const SizedBox(height: 24),
              FilledButton(
                onPressed: _loading ? null : _check,
                child: Text(_loading ? '確認中…' : '接続を確認'),
              ),
              const SizedBox(height: 16),
              Semantics(liveRegion: true, child: Text(_status)),
            ],
          ),
        ),
      ),
    ),
  );
}
