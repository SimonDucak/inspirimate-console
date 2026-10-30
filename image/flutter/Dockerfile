# syntax=docker/dockerfile:1.6
# sandboxd-flutter:0.3.0 — Flutter web toolchain on the sandboxd base image.
#
# Build from the repo root:
#   docker build -t sandboxd-flutter:0.3.0 -f image/flutter/Dockerfile image/flutter
# Then set SANDBOXD_IMAGE=sandboxd-flutter:0.3.0 and recreate the control plane.
# Every sandbox on the instance uses this image. It still contains the base
# toolchains (Node, Python, agents), plus Flutter.
#
# The sandbox root filesystem is read-only, and the Flutter tool writes into
# its own tree (bin/cache lockfiles). Leave the SDK at /opt/flutter and copy
# it into $HOME/flutter on first boot from sandbox.yaml before running it.
FROM sandboxd-base:0.3.0

USER 0

# Matches the client_app created with Flutter 3.32.8 / Dart 3.8.1.
ARG FLUTTER_VERSION=3.32.8

RUN apt-get update && apt-get install -y --no-install-recommends \
      xz-utils zip libglu1-mesa \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL -o /tmp/flutter.tar.xz \
      "https://storage.googleapis.com/flutter_infra_release/releases/stable/linux/flutter_linux_${FLUTTER_VERSION}-stable.tar.xz" \
    && tar -C /opt -xf /tmp/flutter.tar.xz \
    && rm /tmp/flutter.tar.xz \
    && git config --system --add safe.directory '*'

# Precache the web SDK in the image so the first preview does not download it.
# CI and the analytics flag keep the tool from prompting during the build.
ENV FLUTTER_SUPPRESS_ANALYTICS=true \
    CI=true

RUN /opt/flutter/bin/flutter config --no-analytics \
    && /opt/flutter/bin/flutter precache --web \
    && /opt/flutter/bin/flutter --version

# Login shells (runtimed runs commands with bash -lc) must see flutter.
# Prefer the writable copy under $HOME once sandbox.yaml has made it.
RUN printf '%s\n' \
      'export FLUTTER_SUPPRESS_ANALYTICS=true' \
      'export CI=true' \
      'export PUB_CACHE="${PUB_CACHE:-$HOME/.pub-cache}"' \
      'if [ -x "$HOME/flutter/bin/flutter" ]; then' \
      '  export PATH="$HOME/flutter/bin:$PATH"' \
      'else' \
      '  export PATH="/opt/flutter/bin:$PATH"' \
      'fi' \
      > /etc/profile.d/flutter.sh \
    && chmod 644 /etc/profile.d/flutter.sh \
    && ln -sfn /opt/flutter/bin/flutter /usr/local/bin/flutter \
    && ln -sfn /opt/flutter/bin/dart /usr/local/bin/dart \
    && chown -R 1000:1000 /opt/flutter

USER sandbox
