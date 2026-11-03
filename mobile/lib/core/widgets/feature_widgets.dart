import 'package:flutter/material.dart';
import '../network/error_messages.dart';
import '../theme/app_colors.dart';

class FeatureScaffold extends StatelessWidget {
  const FeatureScaffold({
    super.key,
    required this.title,
    required this.child,
    this.actions,
  });
  final String title;
  final Widget child;
  final List<Widget>? actions;
  @override
  Widget build(BuildContext context) => Scaffold(
    backgroundColor: AppColors.background,
    appBar: AppBar(
      backgroundColor: AppColors.header,
      foregroundColor: AppColors.white,
      title: Text(
        title,
        style: const TextStyle(fontSize: 27, fontWeight: FontWeight.w900),
      ),
      actions: actions,
    ),
    body: SafeArea(
      child: SingleChildScrollView(
        padding: const EdgeInsets.all(16),
        child: child,
      ),
    ),
  );
}

class FeatureCard extends StatelessWidget {
  const FeatureCard({super.key, required this.child});
  final Widget child;
  @override
  Widget build(BuildContext context) => Container(
    width: double.infinity,
    margin: const EdgeInsets.only(bottom: 12),
    padding: const EdgeInsets.all(15),
    decoration: BoxDecoration(
      color: Colors.white,
      border: Border.all(color: const Color(0xffe9e9e9)),
      borderRadius: BorderRadius.circular(14),
    ),
    child: child,
  );
}

class ActionButton extends StatelessWidget {
  const ActionButton(
    this.label, {
    super.key,
    this.onPressed,
    this.busy = false,
  });
  final String label;
  final VoidCallback? onPressed;
  final bool busy;
  @override
  Widget build(BuildContext context) => ElevatedButton(
    onPressed: busy ? null : onPressed,
    style: ElevatedButton.styleFrom(
      backgroundColor: AppColors.primary,
      foregroundColor: AppColors.textPrimary,
      elevation: 0,
      padding: const EdgeInsets.symmetric(horizontal: 13, vertical: 10),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(9)),
    ),
    child: Text(
      busy ? '処理中...' : label,
      style: const TextStyle(fontSize: 12, fontWeight: FontWeight.w800),
    ),
  );
}

class ErrorText extends StatelessWidget {
  const ErrorText(this.error, {super.key});
  final Object? error;
  @override
  Widget build(BuildContext context) => error == null
      ? const SizedBox.shrink()
      : Padding(
          padding: const EdgeInsets.symmetric(vertical: 12),
          child: Semantics(
            liveRegion: true,
            child: Text(
              errorMessage(error!),
              style: const TextStyle(color: Colors.redAccent, fontSize: 13),
            ),
          ),
        );
}

InputDecoration featureInput(String label, {String? helper, String? counter}) =>
    InputDecoration(
      labelText: label,
      helperText: helper,
      counterText: counter,
      filled: true,
      fillColor: Colors.white,
      contentPadding: const EdgeInsets.all(12),
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(10),
        borderSide: const BorderSide(color: Color(0xffdddddd)),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(10),
        borderSide: const BorderSide(color: AppColors.primary),
      ),
    );

class AsyncContent<T> extends StatefulWidget {
  const AsyncContent({
    super.key,
    required this.load,
    required this.builder,
    this.compact = false,
  });
  final bool compact;
  final Future<T> Function() load;
  final Widget Function(T data, VoidCallback reload) builder;
  @override
  State<AsyncContent<T>> createState() => _AsyncContentState<T>();
}

class _AsyncContentState<T> extends State<AsyncContent<T>> {
  late Future<T> _future;
  @override
  void initState() {
    super.initState();
    _future = widget.load();
  }

  void reload() => setState(() => _future = widget.load());
  @override
  Widget build(BuildContext context) => FutureBuilder<T>(
    future: _future,
    builder: (context, snapshot) {
      if (snapshot.connectionState != ConnectionState.done) {
        return const Center(
          child: Padding(
            padding: EdgeInsets.all(24),
            child: CircularProgressIndicator(),
          ),
        );
      }
      if (snapshot.hasError && widget.compact) {
        return Center(
          child: IconButton(
            tooltip: '${errorMessage(snapshot.error!)} 再試行',
            onPressed: reload,
            icon: const Icon(Icons.refresh),
          ),
        );
      }
      if (snapshot.hasError) {
        return Column(
          children: [
            ErrorText(snapshot.error),
            ActionButton('再読み込み', onPressed: reload),
          ],
        );
      }
      return widget.builder(snapshot.data as T, reload);
    },
  );
}
