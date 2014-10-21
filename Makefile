.PHONY: default clean build test archive dep

VERSION := 0.6.0
REV := $(shell git rev-parse --short HEAD)
EXTRELVER ?= open-source

ifeq ($(origin GOROOT), undefined)
  GO=go
else
  GO=$(GOROOT)/bin/go
endif

GOOS ?= $(shell $(GO) env GOOS)
GOARCH ?= $(shell $(GO) env GOARCH)

# Awful hack: Travis does not set $GOBIN, the $GOPATH has multiple
# elements, and the go bin directory is not in the $PATH, so we have
# to suss out the location on our own. BLECH. If $GOBIN is set then
# we use it.
GOBIN ?= $(shell cd ../../../..; echo `pwd`/bin)
GODEP := $(GOBIN)/godep

ARCHIVE := harpoon-latest.$(GOOS)-$(GOARCH).tar.gz
DISTDIR ?= $(CURDIR)/dist/$(GOOS)-$(GOARCH)

LDFLAGS    := -X main.Version $(VERSION)\
              -X main.CommitID $(REV) \
              -X main.ExternalReleaseVersion $(EXTRELVER)

default:

clean:
	git clean -dfx

$(GODEP):
	$(GO) get github.com/tools/godep
	touch $@

dep: $(GODEP)
	$(GOBIN)/godep restore

build: $(DISTDIR)/harpoon-agent $(DISTDIR)/harpoon-supervisor $(DISTDIR)/harpoon-scheduler $(DISTDIR)/harpoonctl

$(DISTDIR)/%: $(GODEP)
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GODEP) go build -ldflags "$(LDFLAGS)" -o $(DISTDIR)/$* ./$*

test: $(GODEP)
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GODEP) go test ./...

archive: build test
	tar -C $(DISTDIR) -czvf dist/$(ARCHIVE) .

