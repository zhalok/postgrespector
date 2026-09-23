# Postgres 16 (alpine) with pg_filedump built in, for inspecting heap/index pages during development.
ARG PG_MAJOR=16

FROM postgres:${PG_MAJOR}-alpine AS builder
ARG PG_FILEDUMP_TAG=REL_16_0
RUN apk add --no-cache build-base git lz4-dev zstd-dev \
    && git clone --depth 1 --branch ${PG_FILEDUMP_TAG} https://github.com/df7cb/pg_filedump.git /src/pg_filedump \
    && make -C /src/pg_filedump with_llvm=no \
    && make -C /src/pg_filedump install with_llvm=no DESTDIR=/out

FROM postgres:${PG_MAJOR}-alpine
RUN apk add --no-cache lz4-libs zstd-libs
COPY --from=builder /out/usr/local/bin/pg_filedump /usr/local/bin/pg_filedump
