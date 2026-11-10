# syntax=docker/dockerfile:1.6
# sandboxd-flutter:0.3.0 — Flutter web + Android toolchain on the sandboxd base image.
#
# Build from the repo root:
#   docker build -t sandboxd-flutter:0.3.0 -f image/flutter/Dockerfile image/flutter
# Then set SANDBOXD_IMAGE=sandboxd-flutter:0.3.0 and recreate the control plane.
# Every sandbox on the instance uses this image. It still contains the base
# toolchains (Node, Python, agents), plus Flutter and the Android SDK.
#
# The sandbox root filesystem is read-only. The Android SDK and the Flutter
# tool cache are completed here so `flutter build apk` does not need to write
# into /opt. Gradle's own cache and the project build directory live under
# the writable home directory (/home/sandbox).
FROM sandboxd-base:0.3.0

USER 0

# Matches the client_app created with Flutter 3.32.8 / Dart 3.8.1.
ARG FLUTTER_VERSION=3.32.8
# Pinned Android command-line tools (sdkmanager). The NDK version is read
# from this Flutter SDK so it matches the version the app template requests.
ARG ANDROID_CMDLINE_TOOLS=13114758

RUN apt-get update && apt-get install -y --no-install-recommends \
      xz-utils zip libglu1-mesa openjdk-21-jdk-headless \
    && rm -rf /var/lib/apt/lists/* \
    && ln -sfn "$(dirname "$(dirname "$(readlink -f "$(command -v java)")")")" /usr/lib/jvm/java-21

# Debian stable no longer ships JDK 17. Flutter 3.32's Android Gradle
# plugin accepts JDK 21.
ENV JAVA_HOME=/usr/lib/jvm/java-21 \
    ANDROID_HOME=/opt/android-sdk \
    ANDROID_SDK_ROOT=/opt/android-sdk \
    FLUTTER_SUPPRESS_ANALYTICS=true \
    CI=true \
    PATH="/opt/android-sdk/cmdline-tools/latest/bin:/opt/android-sdk/platform-tools:/opt/flutter/bin:${PATH}"

RUN curl -fsSL -o /tmp/flutter.tar.xz \
      "https://storage.googleapis.com/flutter_infra_release/releases/stable/linux/flutter_linux_${FLUTTER_VERSION}-stable.tar.xz" \
    && tar -C /opt -xf /tmp/flutter.tar.xz \
    && rm /tmp/flutter.tar.xz \
    && git config --system --add safe.directory '*'

RUN mkdir -p /tmp/android-cmdline \
    && curl -fsSL -o /tmp/android-cmdline.zip \
      "https://dl.google.com/android/repository/commandlinetools-linux-${ANDROID_CMDLINE_TOOLS}_latest.zip" \
    && unzip -q /tmp/android-cmdline.zip -d /tmp/android-cmdline \
    && mkdir -p /opt/android-sdk/cmdline-tools \
    && rm -rf /opt/android-sdk/cmdline-tools/latest \
    && mv /tmp/android-cmdline/cmdline-tools /opt/android-sdk/cmdline-tools/latest \
    && rm -rf /tmp/android-cmdline /tmp/android-cmdline.zip

# Platform 35 matches Flutter 3.32. gradle_utils.dart names one NDK;
# app builds use flutter.ndkVersion from FlutterExtension, installed below.
# The root filesystem is read-only, so a missing NDK cannot be downloaded later.
RUN set -eu; \
    NDK_VERSION="$(grep -RhoE "ndkVersion *= *['\"][0-9][0-9.]+['\"]" \
      /opt/flutter/packages/flutter_tools/lib/src/android | head -1 | tr -cd '0-9.')"; \
    if [ -z "$NDK_VERSION" ]; then echo "could not read Flutter NDK version" >&2; exit 1; fi; \
    echo "installing Android NDK ${NDK_VERSION}"; \
    yes | sdkmanager --sdk_root="${ANDROID_HOME}" --licenses >/dev/null || true; \
    sdkmanager --sdk_root="${ANDROID_HOME}" \
      "platform-tools" "platforms;android-35" "build-tools;35.0.0" "ndk;${NDK_VERSION}"; \
    yes | sdkmanager --sdk_root="${ANDROID_HOME}" --licenses >/dev/null || true

# Precache web and Android so the first preview and the first APK build
# do not download engine artifacts into the read-only SDK tree.
RUN /opt/flutter/bin/flutter config --no-analytics \
    && /opt/flutter/bin/flutter config --android-sdk /opt/android-sdk \
    && /opt/flutter/bin/flutter precache --web \
    && /opt/flutter/bin/flutter precache --android \
    && /opt/flutter/bin/flutter --version

# `ndkVersion = flutter.ndkVersion` reads FlutterExtension, which is a
# different NDK than gradle_utils.dart. Install it before the tree is
# chowned and the image becomes read-only at runtime.
RUN set -eu; \
    NDK_VERSION="$(sed -n 's/.*ndkVersion: String = .\([0-9.]*\).*/\1/p' \
      /opt/flutter/packages/flutter_tools/gradle/src/main/kotlin/FlutterExtension.kt)"; \
    if [ -z "$NDK_VERSION" ]; then echo "could not read flutter.ndkVersion" >&2; exit 1; fi; \
    if [ -d "${ANDROID_HOME}/ndk/${NDK_VERSION}" ]; then echo "NDK ${NDK_VERSION} already installed"; exit 0; fi; \
    echo "installing Android NDK ${NDK_VERSION}"; \
    yes | sdkmanager --sdk_root="${ANDROID_HOME}" --licenses >/dev/null || true; \
    sdkmanager --sdk_root="${ANDROID_HOME}" "ndk;${NDK_VERSION}"

# Plugins in a real app often require an older build-tools package and a
# newer NDK than Flutter's own defaults. Flutter's Gradle plugin also points
# the app at an empty CMakeLists.txt so the Android Gradle Plugin will fetch
# the NDK; AGP 8 then installs cmake;3.22.1 next to it. Those components must
# already be present: the SDK directory is not writable when the APK is built.
RUN set -eu; \
    yes | sdkmanager --sdk_root="${ANDROID_HOME}" --licenses >/dev/null || true; \
    sdkmanager --sdk_root="${ANDROID_HOME}" \
      "platforms;android-34" \
      "build-tools;34.0.0" \
      "ndk;27.0.12077973" \
      "cmake;3.22.1"

# Login shells (runtimed runs commands with bash -lc) must see flutter
# and the Android SDK. docker exec also inherits the image ENV above.
# Prefer a writable Flutter copy under $HOME when one has been made.
RUN printf '%s\n' \
      'export FLUTTER_SUPPRESS_ANALYTICS=true' \
      'export CI=true' \
      'export PUB_CACHE="${PUB_CACHE:-$HOME/.pub-cache}"' \
      'export ANDROID_HOME="${ANDROID_HOME:-/opt/android-sdk}"' \
      'export ANDROID_SDK_ROOT="${ANDROID_SDK_ROOT:-/opt/android-sdk}"' \
      'export JAVA_HOME="${JAVA_HOME:-/usr/lib/jvm/java-21}"' \
      'if [ -x "$HOME/flutter/bin/flutter" ]; then' \
      '  export PATH="$HOME/flutter/bin:$PATH"' \
      'else' \
      '  export PATH="/opt/flutter/bin:$PATH"' \
      'fi' \
      'export PATH="$PATH:$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools"' \
      > /etc/profile.d/flutter.sh \
    && chmod 644 /etc/profile.d/flutter.sh \
    && ln -sfn /opt/flutter/bin/flutter /usr/local/bin/flutter \
    && ln -sfn /opt/flutter/bin/dart /usr/local/bin/dart \
    && chown -R 1000:1000 /opt/flutter /opt/android-sdk

USER sandbox
