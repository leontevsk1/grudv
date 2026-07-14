FROM rust:1.93-alpine AS build
WORKDIR /src
RUN apk add --no-cache musl-dev
COPY Cargo.toml Cargo.lock ./
COPY .sqlx ./.sqlx
COPY src ./src
ENV SQLX_OFFLINE=true
RUN cargo build --release

FROM gcr.io/distroless/static-debian12
COPY --from=build /src/target/release/vpn-core /vpn-core
ENTRYPOINT ["/vpn-core"]
