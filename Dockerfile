FROM golang:1.15
MAINTAINER TRON-US <support@tron.network>

# Install deps
RUN apt-get update && apt-get install -y \
  libssl-dev \
  ca-certificates \
  fuse

ENV SRC_DIR /go-btfs
