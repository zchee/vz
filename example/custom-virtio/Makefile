.PHONY: all
all: build codesign

.PHONY: codesign
codesign:
	codesign --entitlements vz.entitlements -s - ./custom-virtio

.PHONY: build
build:
	go build -o custom-virtio .
