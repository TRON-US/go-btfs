SHELL := /bin/bash # Use bash syntax

# golang utilities
GO_MIN_VERSION = 1.14
export GO111MODULE=on


# pre-definitions
GOCC ?= go
GOTAGS ?=
unexport GOFLAGS
GOFLAGS ?=
GOTFLAGS ?=
TEST_COVERAGE_OUTPUT ?= tests_coverage
TEST_FILE_DIR ?= /tmp

#select explicit list of packages to test
COVERPKG_LIST := $(shell cat coverpkg_list.txt)

MY_FILE := test_pkgs.txt
MY_PKGS := $(shell cat ${MY_FILE})

#select packages that dont contain a test dir to avoid build errors
NON_TEST_PKGS := $(shell go list ./... | grep -v "/test")

# Try to make building as reproducible as possible by stripping the go path.
GOFLAGS += "-asmflags=all='-trimpath=$(GOPATH)'" "-gcflags=all='-trimpath=$(GOPATH)'"

#GOTFLAGS += "-coverprofile=$(TEST_COVERAGE_OUTPUT).out" "-coverpkg=$(COVERPKG_LIST)"
GOTFLAGS += "-coverprofile=$(TEST_COVERAGE_OUTPUT).out"
#GOTFLAGS += "-coverprofile=$(TEST_COVERAGE_OUTPUT).out" "-covermode=set"

ifeq ($(tarball-is),1)
	GOFLAGS += -mod=vendor
endif

# match Go's default GOPATH behaviour
export GOPATH ?= $(shell $(GOCC) env GOPATH)

DEPS_GO :=
TEST_GO :=
TEST_GO_BUILD :=
CHECK_GO :=

go-pkg-name=$(shell $(GOCC) list $(go-tags) github.com/TRON-US/go-btfs/$(1))
go-main-name=$(notdir $(call go-pkg-name,$(1)))$(?exe)
go-curr-pkg-tgt=$(d)/$(call go-main-name,$(d))
go-pkgs=$(shell $(GOCC) list github.com/TRON-US/go-btfs/...)

go-tags=$(if $(GOTAGS), -tags="$(call join-with,$(space),$(GOTAGS))")
go-flags-with-tags=$(GOFLAGS)$(go-tags)

define go-build-relative
$(GOCC) build $(go-flags-with-tags) -o "$@" "$(call go-pkg-name,$<)"
endef

define go-build
$(GOCC) build $(go-flags-with-tags) -o "$@" "$(1)"
endef

define go-try-build
$(GOCC) build $(go-flags-with-tags) -o /dev/null "$(call go-pkg-name,$<)"
endef

test_go_test: $$(DEPS_GO)
#	$(GOCC) test $(go-flags-with-tags) $(GOTFLAGS) ./...
	$(GOCC) test $(go-flags-with-tags) $(GOTFLAGS) $(MY_PKGS)
.PHONY: test_go_test

#Used to display coverage per function an total at the end
test_coverage_output:
	$(GOCC) tool cover -func="$(TEST_COVERAGE_OUTPUT).out"

#Generates an html report of the unit tests coverage
test_coverage_html:
	$(GOCC) tool cover -html="$(TEST_COVERAGE_OUTPUT).out" -o "$(TEST_COVERAGE_OUTPUT).html"
.PHONY: test_coverage_html

test_go_build: $$(TEST_GO_BUILD)

test_go_short: GOTFLAGS += -test.short
test_go_short: test_go_test
.PHONY: test_go_short

test_go_race: GOTFLAGS += -race
test_go_race: test_go_test
.PHONY: test_go_race

test_go_expensive: test_go_test test_go_build
.PHONY: test_go_expensive
TEST_GO += test_go_expensive

test_go_fmt:
	bin/test-go-fmt
.PHONY: test_go_fmt
TEST_GO += test_go_fmt

test_go_mod: 
	bin/test-go-mod
.PHONY: test_go_mod
TEST_GO += test_go_mod

test_go_lint: test/bin/golangci-lint
	golangci-lint run ./...
.PHONY: test_go_lint

test_go: $(TEST_GO)

test_build_special_exe:
	cd ./cmd/btfs && \
	$(GOCC) test -coverpkg=./... -c *.go -tags testrunmain -o $(TEST_FILE_DIR)/test-special-exe
.PHONY: test_build_special_exe

check_go_version:
	@$(GOCC) version
	bin/check_go_version $(GO_MIN_VERSION)
	bash patching.sh
.PHONY: check_go_version
DEPS_GO += check_go_version

TEST += $(TEST_GO)
TEST_SHORT += test_go_fmt test_go_mod test_go_short
