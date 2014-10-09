GOOS     := $(shell go env GOOS)
GOARCH   := $(shell go env GOARCH)

# Awful hack: Travis does not set $GOBIN, the $GOPATH has multiple
# elements, and the go bin directory is not in the $PATH, so we have
# to suss out the location on our own. BLECH. This will run into
# problems when $GOBIN is actually set to a different location, but
# that currently does not affect us. Someday that conditional case
# will need to be addressed.
GOBIN    := $(shell cd ../../../..; echo `pwd`/bin)

ARCHIVE := harpoon-latest.$(GOOS)-$(GOARCH).tar.gz
DISTDIR := dist/$(GOOS)-$(GOARCH)

.PHONY: default
default:

.PHONY: clean
clean:
	git clean -dfx

.PHONY: archive
archive: dep
	GOOS=$(GOOS) GOARCH=$(GOARCH) godep go build -o $(DISTDIR)/harpoon-agent ./harpoon-agent
	GOOS=$(GOOS) GOARCH=$(GOARCH) godep go build -o $(DISTDIR)/harpoon-supervisor ./harpoon-supervisor
	GOOS=$(GOOS) GOARCH=$(GOARCH) godep go build -o $(DISTDIR)/harpoon-scheduler ./harpoon-scheduler
	tar -C $(DISTDIR) -czvf dist/$(ARCHIVE) .

.PHONY: dep
dep:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go get github.com/tools/godep
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOBIN)/godep restore

