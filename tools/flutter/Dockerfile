FROM debian:trixie-slim
ARG FLUTTER_VERSION=3.50.0
ARG FLUTTER_SHA256=6ea2d806fd57269ae783e6788e2a81fc26eaad782f1e0d0ff38623bb765ff9f5
RUN apt-get update -qq && apt-get install -y -qq --no-install-recommends ca-certificates curl git unzip xz-utils >/dev/null && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL -o /tmp/flutter.tar.xz "https://storage.googleapis.com/flutter_infra_release/releases/stable/linux/flutter_linux_${FLUTTER_VERSION}-stable.tar.xz" \
 && echo "${FLUTTER_SHA256}  /tmp/flutter.tar.xz" | sha256sum -c - \
 && tar -xJf /tmp/flutter.tar.xz -C /opt && rm /tmp/flutter.tar.xz \
 && git config --system --add safe.directory /opt/flutter
ENV PATH=/opt/flutter/bin:/opt/flutter/bin/cache/dart-sdk/bin:$PATH PUB_CACHE=/cache/pub FLUTTER_SUPPRESS_ANALYTICS=true CI=true
RUN flutter config --no-analytics --no-cli-animations >/dev/null 2>&1; dart --disable-analytics >/dev/null 2>&1; flutter --version
