FROM debian:trixie-slim
RUN apt-get update -qq && apt-get install -y -qq --no-install-recommends python3 iproute2 nftables curl openssl procps >/dev/null && rm -rf /var/lib/apt/lists/*
