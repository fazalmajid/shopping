# ── Stage 1: build llama-server ───────────────────────────────────────────────
FROM alpine:3.20 AS llama-builder
RUN apk add --no-cache git cmake make gcc g++ linux-headers
WORKDIR /build
# Clone the default branch (latest). To pin a version add: --branch bNNNN
RUN git clone --depth 1 https://github.com/ggerganov/llama.cpp
RUN cmake -B llama.cpp/build -S llama.cpp \
        -DCMAKE_BUILD_TYPE=Release \
        -DCMAKE_RUNTIME_OUTPUT_DIRECTORY=/build/llama.cpp/build/bin \
        -DBUILD_SHARED_LIBS=OFF \
        -DLLAMA_BUILD_TESTS=OFF \
        -DGGML_OPENMP=OFF
RUN cmake --build llama.cpp/build --target llama-server -j$(nproc)

# ── Stage 2: build the Go server ─────────────────────────────────────────────
FROM golang:1.22-alpine AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o shopping-server ./cmd/server

# ── Stage 3: minimal runtime image ───────────────────────────────────────────
FROM alpine:3.20
# libstdc++/libgcc are required by llama-server (built with g++)
RUN apk add --no-cache ca-certificates libstdc++ libgcc
WORKDIR /app
COPY --from=llama-builder /build/llama.cpp/build/bin/llama-server ./llama-server
COPY --from=go-builder /src/shopping-server ./shopping-server
EXPOSE 8080
ENTRYPOINT ["/app/shopping-server"]
