#!/usr/bin/env bash
# Download everything `marvindocs --embedder onnx` needs into data/onnx/:
#   lib/libtokenizers.a      static tokenizer library (linked at build time)
#   lib/libonnxruntime.so    ONNX Runtime (loaded at run time)
#   model/model_int8.onnx    bge-m3, int8
#   model/tokenizer.json
# Idempotent: files that already exist are kept. Linux x86_64 only.
set -euo pipefail

TOKENIZERS_VERSION=v1.27.0
ONNXRUNTIME_VERSION=1.30.0
MODEL_REPO=https://huggingface.co/Xenova/bge-m3/resolve/main

cd "$(dirname "$0")/.."
DEST=${ONNX_DIR:-data/onnx}
mkdir -p "$DEST/lib" "$DEST/model"

case "$(uname -s)-$(uname -m)" in
Linux-x86_64)
	tk_asset=linux-amd64
	ort_asset=linux-x64
	;;
*)
	echo "unsupported platform $(uname -s)-$(uname -m): fetch the files by hand (see docs/en/vectors.md)" >&2
	exit 1
	;;
esac

fetch() { # url dest
	if [ -e "$2" ]; then
		echo "exists: $2"
		return
	fi
	echo "fetch:  $2"
	curl -fL --progress-bar -o "$2.part" "$1" && mv "$2.part" "$2"
}

if [ ! -e "$DEST/lib/libtokenizers.a" ]; then
	fetch "https://github.com/daulet/tokenizers/releases/download/$TOKENIZERS_VERSION/libtokenizers.$tk_asset.tar.gz" "$DEST/libtokenizers.tar.gz"
	tar xzf "$DEST/libtokenizers.tar.gz" -C "$DEST/lib" libtokenizers.a
	rm "$DEST/libtokenizers.tar.gz"
else
	echo "exists: $DEST/lib/libtokenizers.a"
fi

if [ ! -e "$DEST/lib/libonnxruntime.so" ]; then
	ort="onnxruntime-$ort_asset-$ONNXRUNTIME_VERSION"
	fetch "https://github.com/microsoft/onnxruntime/releases/download/v$ONNXRUNTIME_VERSION/$ort.tgz" "$DEST/$ort.tgz"
	tar xzf "$DEST/$ort.tgz" -C "$DEST/lib" --strip-components=2 "$ort/lib/libonnxruntime.so.$ONNXRUNTIME_VERSION"
	ln -sf "libonnxruntime.so.$ONNXRUNTIME_VERSION" "$DEST/lib/libonnxruntime.so"
	rm "$DEST/$ort.tgz"
else
	echo "exists: $DEST/lib/libonnxruntime.so"
fi

fetch "$MODEL_REPO/onnx/model_int8.onnx" "$DEST/model/model_int8.onnx"
fetch "$MODEL_REPO/tokenizer.json" "$DEST/model/tokenizer.json"

echo "done: $DEST"
