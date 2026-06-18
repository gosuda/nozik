# Nozik — AI Music Agent for Gozik
# https://github.com/gg582/nozik

# Binary and build settings
BINARY     := nozik
MAIN_PKG   := ./cmd/nozik
BUILD_DIR  := .

# Installation paths
PREFIX     ?= /usr/local
BINDIR     ?= $(PREFIX)/bin
DATADIR    ?= $(PREFIX)/share
DESKTOPDIR ?= $(DATADIR)/applications
MIMEDIR    ?= $(DATADIR)/mime
MIMEPKGDIR ?= $(MIMEDIR)/packages
DESTDIR    ?=

# Source assets
DESKTOP_SRC := assets/nozik.desktop
MIME_SRC    := assets/nozik.xml
ICON_SRC    := assets/icons/nozik.png

# Icon sizes to generate from the master icon
ICON_SIZES := 16 22 24 32 48 64 96 128 256 512 1024
ICON_DIR   := build/icons/hicolor
ICON_FILES := $(foreach size,$(ICON_SIZES),$(ICON_DIR)/$(size)x$(size)/apps/nozik.png)

# Go settings
GO         := go
GOFLAGS    ?=
LDFLAGS    ?=

# ImageMagick tool (prefer v7 'magick', fall back to 'convert')
IMAGEMAGICK := $(shell command -v magick || command -v convert || echo convert)

# Default target: build the binary
.PHONY: all build icons install uninstall clean clean-icons

all: build

build:
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) $(MAIN_PKG)

# Generate per-size PNGs for hicolor icon theme
icons: $(ICON_FILES)

define ICON_RULE
$(ICON_DIR)/$(size)x$(size)/apps/nozik.png: $(ICON_SRC)
	install -d $$(dir $$@)
	$(IMAGEMAGICK) $$< -resize $(size)x$(size) $$@
endef
$(foreach size,$(ICON_SIZES),$(eval $(ICON_RULE)))

install: build icons
	install -d $(DESTDIR)$(BINDIR)
	install -m 0755 $(BUILD_DIR)/$(BINARY) $(DESTDIR)$(BINDIR)/$(BINARY)
	install -d $(DESTDIR)$(DESKTOPDIR)
	install -m 0644 $(DESKTOP_SRC) $(DESTDIR)$(DESKTOPDIR)/nozik.desktop
	install -d $(DESTDIR)$(MIMEPKGDIR)
	install -m 0644 $(MIME_SRC) $(DESTDIR)$(MIMEPKGDIR)/nozik.xml
	@for size in $(ICON_SIZES); do \
		install -d $(DESTDIR)$(DATADIR)/icons/hicolor/$${size}x$${size}/apps; \
		install -m 0644 $(ICON_DIR)/$${size}x$${size}/apps/nozik.png \
			$(DESTDIR)$(DATADIR)/icons/hicolor/$${size}x$${size}/apps/nozik.png; \
	done
ifeq ($(DESTDIR),)
	-update-desktop-database $(DESKTOPDIR)
	-update-mime-database $(MIMEDIR)
endif

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/$(BINARY)
	rm -f $(DESTDIR)$(DESKTOPDIR)/nozik.desktop
	rm -f $(DESTDIR)$(MIMEPKGDIR)/nozik.xml
	@for size in $(ICON_SIZES); do \
		rm -f $(DESTDIR)$(DATADIR)/icons/hicolor/$${size}x$${size}/apps/nozik.png; \
	done
ifeq ($(DESTDIR),)
	-update-desktop-database $(DESKTOPDIR)
	-update-mime-database $(MIMEDIR)
endif

clean-icons:
	rm -rf $(ICON_DIR)

clean: clean-icons
	rm -f $(BUILD_DIR)/$(BINARY)
