FROM eclipse-temurin:21-jdk
ARG KOTLIN_VERSION=2.4.20
ARG KOTLIN_SHA256=59e9ca74c7904ef2c122b12114937673ccce68de820a663f0ed66ccf8799e0b7
ARG PLATFORM=36
RUN apt-get update && apt-get install -y --no-install-recommends unzip python3 ca-certificates curl && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL -o /tmp/kotlin.zip "https://github.com/JetBrains/kotlin/releases/download/v${KOTLIN_VERSION}/kotlin-compiler-${KOTLIN_VERSION}.zip" \
    && echo "${KOTLIN_SHA256}  /tmp/kotlin.zip" | sha256sum -c - \
    && unzip -q /tmp/kotlin.zip -d /opt && rm /tmp/kotlin.zip
COPY fetch.py /opt/fetch.py
RUN python3 /opt/fetch.py platform ${PLATFORM} /opt/android.jar
ENV PATH=/opt/kotlinc/bin:$PATH
