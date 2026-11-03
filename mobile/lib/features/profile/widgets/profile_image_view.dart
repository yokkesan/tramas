import 'dart:typed_data';
import 'package:flutter/material.dart';
import '../../../app/app_scope.dart';
import '../../../core/widgets/feature_widgets.dart';
import '../models/profile.dart';

Uri? standardImageUri(Uri base, String value) {
  final parsed = Uri.tryParse(value);
  if (parsed == null) return null;
  final uri = base.resolveUri(parsed);
  if (!['http', 'https'].contains(uri.scheme) ||
      uri.userInfo.isNotEmpty ||
      uri.scheme != 'https' && uri.origin != base.origin) {
    return null;
  }
  return uri;
}

class ProfileImageView extends StatelessWidget {
  const ProfileImageView({
    super.key,
    this.image,
    this.standardId,
    required this.kind,
  });
  final ProfileImage? image;
  final String? standardId;
  final String kind;
  @override
  Widget build(BuildContext context) {
    final scope = AppScope.of(context);
    Widget placeholder() => Container(
      color: const Color(0xffedeadf),
      alignment: Alignment.center,
      child: Icon(
        kind == 'avatar' ? Icons.person : Icons.panorama,
        color: Colors.black54,
        size: 36,
      ),
    );
    Widget picture(Uint8List bytes) => Image.memory(
      bytes,
      fit: BoxFit.cover,
      gaplessPlayback: false,
      errorBuilder: (_, _, _) => const Center(child: Text('画像を表示できません')),
    );
    Widget child;
    if (image != null) {
      child = AsyncContent<Uint8List>(
        compact: true,
        key: ValueKey(image!.id),
        load: () => scope.profiles.image(image!.id),
        builder: (bytes, _) => picture(bytes),
      );
    } else if (standardId != null) {
      child = AsyncContent<List<StandardImage>>(
        compact: true,
        key: ValueKey(standardId),
        load: scope.profiles.standards,
        builder: (catalog, _) {
          final matches = catalog.where(
            (v) => v.kind == kind && v.id == standardId,
          );
          if (matches.isEmpty) return placeholder();
          final uri = standardImageUri(
            scope.session.api.config.apiBaseUrl,
            matches.first.url,
          );
          if (uri == null) return placeholder();
          return Image.network(
            uri.toString(),
            fit: BoxFit.cover,
            errorBuilder: (_, _, _) => placeholder(),
          );
        },
      );
    } else {
      child = placeholder();
    }
    return SizedBox(
      height: kind == 'avatar' ? 72 : 140,
      width: kind == 'avatar' ? 72 : double.infinity,
      child: ClipRRect(
        borderRadius: BorderRadius.circular(kind == 'avatar' ? 36 : 14),
        child: child,
      ),
    );
  }
}
