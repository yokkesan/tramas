import 'dart:convert';
import 'dart:typed_data';
import 'package:file_picker/file_picker.dart';
import 'package:flutter/services.dart';
import '../../../core/network/api_client.dart';

class SelectedImage {
  const SelectedImage(this.bytes, this.contentType);
  final Uint8List bytes;
  final String contentType;
  static SelectedImage validate(Uint8List bytes) {
    if (bytes.length > 10 * 1024 * 1024) {
      throw const ApiException('image_too_large', status: 413);
    }
    String? type;
    if (bytes.length >= 3 &&
        bytes[0] == 255 &&
        bytes[1] == 216 &&
        bytes[2] == 255) {
      type = 'image/jpeg';
    }
    if (bytes.length >= 8 &&
        const ListEquality().same(bytes.sublist(0, 8), [
          137,
          80,
          78,
          71,
          13,
          10,
          26,
          10,
        ])) {
      type = 'image/png';
    }
    if (bytes.length >= 24 &&
        ascii.decode(bytes.sublist(4, 8), allowInvalid: true) == 'ftyp') {
      final length = ByteData.sublistView(bytes).getUint32(0);
      if (length >= 24 && length <= bytes.length && length % 4 == 0) {
        for (var i = 8; i < length; i += 4) {
          if (i == 12) continue;
          final brand = ascii.decode(
            bytes.sublist(i, i + 4),
            allowInvalid: true,
          );
          if (brand == 'heic' || brand == 'heix') type = 'image/heic';
        }
      }
    }
    if (type == null) throw const ApiException('invalid_image', status: 422);
    return SelectedImage(bytes, type);
  }
}

class ListEquality {
  const ListEquality();
  bool same(List<int> a, List<int> b) {
    if (a.length != b.length) return false;
    for (var i = 0; i < a.length; i++) {
      if (a[i] != b[i]) return false;
    }
    return true;
  }
}

abstract interface class ImageSelection {
  Future<SelectedImage?> pick();
}

class NativeImageSelection implements ImageSelection {
  @override
  Future<SelectedImage?> pick() async {
    try {
      final file = await FilePicker.pickFile(
        type: FileType.image,
        compressionQuality: 0,
        darwinOptions: const DarwinOptions(
          assetRepresentationMode: DarwinAssetRepresentationMode.current,
        ),
      );
      if (file == null) return null;
      if ((await file.length() ?? 0) > 10 * 1024 * 1024) {
        throw const ApiException('image_too_large', status: 413);
      }
      final builder = BytesBuilder(copy: false);
      await for (final chunk in file.readAsByteStream()) {
        if (builder.length + chunk.length > 10 * 1024 * 1024) {
          throw const ApiException('image_too_large', status: 413);
        }
        builder.add(chunk);
      }
      return SelectedImage.validate(builder.takeBytes());
    } on ApiException {
      rethrow;
    } on PlatformException catch (e) {
      final code = e.code.toLowerCase();
      throw ApiException(
        code.contains('permission') || code.contains('denied')
            ? 'image_permission_denied'
            : 'image_selection_failed',
      );
    } catch (_) {
      throw const ApiException('image_selection_failed');
    }
  }
}
