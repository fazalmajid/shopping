.PHONY: all build run tidy llama-server clean download-model
.DEFAULT_GOAL := all

# Keep all Go build cache local so `make clean` removes everything.
export GOCACHE := $(CURDIR)/.cache/go-build

LLAMA_LOCAL    := $(CURDIR)/llama
LLAMA_CPP_REPO := https://github.com/ggerganov/llama.cpp
# File targets for the llama-server C build; Make rebuilds only when absent.
LLAMA_GIT      := $(LLAMA_LOCAL)/.git
LLAMA_SERVER   := $(LLAMA_LOCAL)/build/bin/llama-server

tidy:
	go mod tidy

# Clone the latest llama.cpp. Re-runs only when llama/ is absent (after make clean).
$(LLAMA_GIT):
	git clone --depth 1 $(LLAMA_CPP_REPO) $(LLAMA_LOCAL)

# Build llama-server with cmake. Re-runs only when the binary is absent.
$(LLAMA_SERVER): $(LLAMA_GIT)
	cmake -B $(LLAMA_LOCAL)/build -S $(LLAMA_LOCAL) \
		-DCMAKE_BUILD_TYPE=Release \
		-DCMAKE_RUNTIME_OUTPUT_DIRECTORY=$(LLAMA_LOCAL)/build/bin \
		-DLLAMA_BUILD_TESTS=OFF
	cmake --build $(LLAMA_LOCAL)/build --target llama-server -j

# Phony alias.
llama-server: $(LLAMA_SERVER)

# Build the Go server (pure Go, no CGo).
build:
	@mkdir -p bin
	go build -o bin/shopping-server ./cmd/server

# Default: build both the Go server and llama-server.
# `make clean all` rebuilds everything from scratch, including the C binary.
all: $(LLAMA_SERVER) build

run: all
	./bin/shopping-server

MODEL_DIR  := models
MODEL_REPO := bartowski/google_gemma-4-E4B-it-GGUF
MODEL_FILE := google_gemma-4-E4B-it-Q4_K_M.gguf
MODEL_PATH := $(MODEL_DIR)/$(MODEL_FILE)
MODEL_URL  := https://huggingface.co/$(MODEL_REPO)/resolve/main/$(MODEL_FILE)

download-model:
	@mkdir -p $(MODEL_DIR)
	@if [ -f "$(MODEL_PATH)" ] && [ $$(wc -c < "$(MODEL_PATH)") -gt 1000000 ]; then \
		echo "Model already present at $(MODEL_PATH)"; \
	elif command -v wget >/dev/null 2>&1; then \
		wget -c --show-progress -O "$(MODEL_PATH)" "$(MODEL_URL)" || { rm -f "$(MODEL_PATH)"; exit 1; }; \
	elif command -v curl >/dev/null 2>&1; then \
		curl -L -C - --progress-bar -o "$(MODEL_PATH)" "$(MODEL_URL)" || { rm -f "$(MODEL_PATH)"; exit 1; }; \
	else \
		echo "ERROR: no download tool found (need wget or curl)" >&2; exit 1; \
	fi
	@if [ -f "$(MODEL_PATH)" ] && [ $$(wc -c < "$(MODEL_PATH)") -lt 1000000 ]; then \
		echo "ERROR: download returned an error response" >&2; \
		rm -f "$(MODEL_PATH)"; exit 1; \
	fi

# Removes all local build artifacts: Go binary, llama-server build, Go cache.
clean:
	rm -rf bin/ llama/ .cache/
