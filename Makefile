# VecBound Makefile

BINARY_NAME=vecbound
MODELS_DIR=.
RUNTIME_URL=https://github.com/microsoft/onnxruntime/releases/download/v1.20.1/onnxruntime-linux-x64-1.20.1.tgz
MODEL_URL=https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/onnx/model.onnx
VOCAB_URL=https://huggingface.co/sentence-transformers/all-MiniLM-L6-v2/resolve/main/vocab.txt

.PHONY: all build clean test fetch-models fetch-runtime install uninstall setup

all: build

build:
	go build -o $(BINARY_NAME) main.go

test:
	go test ./... -v

clean:
	rm -f $(BINARY_NAME)
	rm -f vec.db
	rm -f libonnxruntime.so
	rm -f model.onnx vocab.txt

install: build
	install -m 0755 $(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@echo "Installed $(BINARY_NAME) to /usr/local/bin"

uninstall:
	rm -f /usr/local/bin/$(BINARY_NAME)
	@echo "Uninstalled $(BINARY_NAME)"

fetch-models:
	@echo "Downloading ONNX models..."
	curl -L $(MODEL_URL) -o model.onnx
	curl -L $(VOCAB_URL) -o vocab.txt

fetch-runtime:
	@echo "Downloading ONNX Runtime (Linux x64)..."
	curl -L $(RUNTIME_URL) -o onnxruntime.tgz
	tar -xzf onnxruntime.tgz --strip-components=2 onnxruntime-linux-x64-1.20.1/lib/libonnxruntime.so.1.20.1
	mv libonnxruntime.so.1.20.1 libonnxruntime.so
	rm onnxruntime.tgz

setup: fetch-models fetch-runtime build
	@echo "Setup complete. Run ./vecbound --help to get started."
