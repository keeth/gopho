appname := gopho

sources := $(wildcard *.go)

version := $(shell (git describe --tags --exact-match || git symbolic-ref -q --short HEAD) | sed 's/^v//')

build = GOOS=$(1) GOARCH=$(2) go build -ldflags "-X main.version=$(version)" -o build/$(appname)_$(1)_$(2)$(3)

.PHONY: all windows darwin linux clean

all: windows darwin linux

clean:
	rm -rf build/

linux: build/gopho_linux_386 build/gopho_linux_amd64 build/gopho_linux_arm build/gopho_linux_arm64

build/gopho_linux_386: $(sources)
	$(call build,linux,386,)

build/gopho_linux_amd64: $(sources)
	$(call build,linux,amd64,)

build/gopho_linux_arm: $(sources)
	$(call build,linux,arm,)

build/gopho_linux_arm64: $(sources)
	$(call build,linux,arm64,)

darwin: build/gopho_darwin_amd64

build/gopho_darwin_amd64: $(sources)
	$(call build,darwin,amd64,)

windows: build/gopho_windows_386 build/gopho_windows_amd64

build/gopho_windows_386: $(sources)
	$(call build,windows,386,.exe)

build/gopho_windows_amd64: $(sources)
	$(call build,windows,amd64,.exe)
