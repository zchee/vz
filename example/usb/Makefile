.PHONY: all
all: build codesign

.PHONY: codesign
codesign:
	codesign --entitlements vz.entitlements -s - ./usb

.PHONY: build
build:
	go build -o usb .
