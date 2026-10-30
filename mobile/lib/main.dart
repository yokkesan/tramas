import 'package:flutter/material.dart';

import 'config/app_config.dart';
import 'features/auth/screens/login_screen.dart';

void main() {
  runApp(TramasApp(config: AppConfig.fromEnvironment()));
}

class TramasApp extends StatelessWidget {
  const TramasApp({super.key, required this.config});

  final AppConfig config;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'tramas',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        useMaterial3: true,
        colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xFFFFDA2D)),
      ),
      home: LoginScreen(apiBaseUrl: config.apiBaseUrl.toString()),
    );
  }
}
