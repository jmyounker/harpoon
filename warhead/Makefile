PKGDIR=.pkg
SRC=$(wildcard *.go)
TGT=warhead

OS=$(subst Darwin,darwin,$(subst Linux,linux,$(shell uname)))
ARCH=$(shell uname -m)

GOOS=$(subst Darwin,darwin,$(subst Linux,linux,$(OS)))
GOARCH=$(subst x86_64,amd64,$(ARCH))
GOPKG=$(subst darwin-amd64,darwin-amd64-osx10.8,$(GOVER).$(GOOS)-$(GOARCH).tar.gz)
GOCC=$(GOROOT)/bin/go
GO=GOROOT=$(GOROOT) GOPATH=$(GOPATH) $(GOCC)

build: $(TGT)

$(TGT): $(SRC)
	$(GO) build -v -o $(TGT)

clean:
	rm -rf $(PKGDIR) $(TGT) warhead.tgz

$(PKGDIR):
	mkdir -p $(PKGDIR)

$(PKGDIR)/warhead: $(PKGDIR)
	mkdir -p $(PKGDIR)/warhead

$(PKGDIR)/warhead/bin: $(PKGDIR)/warhead
	mkdir -p $(PKGDIR)/warhead/bin

$(PKGDIR)/warhead/bin/warhead: $(PKGDIR)/warhead/bin $(TGT)
	cp -p $(TGT) $(PKGDIR)/warhead/bin/warhead

pkg: $(PKGDIR)/warhead/bin/warhead
	echo "Collecting dependencies"
	for dep in $$(ldd $(TGT) | egrep -o '/[^ ]+'); do install -D $$dep $(PKGDIR)/warhead/$$dep; done
	echo "Bundling dependencies"
	tar cvfz warhead.tgz -C $(PKGDIR)/warhead .

.PHONY: build clean pkg

