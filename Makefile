VERSION := 0.2.7
GO_MODULE = $(shell cat go.mod | grep module | cut -d ' ' -f2)
DIST_DIR = dist

DOCKER_IMAGE_MDNINJA = ghcr.io/skerkour/markdown-ninja
DOCKER_IMAGE_MDNINJA_EBOOK = ghcr.io/skerkour/markdown-ninja-ebook

####################################################################################################
# Dev
####################################################################################################

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: exif
exif:
	exiftool -overwrite_original -recurse -all= webapp/public/* themes/default_icons/* themes/*/public/* || exit 0


.PHONY: test
test:
	CGO_ENABLED=0 go test ./cmd/... ./mdninja-go/... ./pkg/...

.PHONY: sql
sql:
	psql $(shell cat markdown_ninja_server.yml | grep postgres | cut -d'"' -f2)

.PHONY: update_deps
update_deps:
	go get -u ./cmd/...
	go mod tidy
	go mod tidy


.PHONY: update_stdx
update_stdx:
	go get -u github.com/skerkour/stdx-go
	go mod tidy
	go mod tidy

.PHONY: clean
clean:
	rm -rf $(DIST_DIR)

.PHONY: delete_ds_store
delete_ds_store:
	find . -type f -name .DS_Store -exec rm {} \;

.PHONY: verify_deps
verify_deps:
	go mod verify

.PHONY: download_deps
download_deps:
	go mod download

.PHONY: release
release:
	date
	make delete_ds_store
	make exif
	make mdninja
	cd webapp && make check
	cd themes/blog && make check
	cd themes/docs && make check
	git checkout main
	git push
	git checkout release
	git merge main
	git push
	git checkout main



####################################################################################################
# docker
####################################################################################################

.PHONY: docker_build
docker_build:
	docker build -t $(DOCKER_IMAGE_MDNINJA):latest -f Dockerfile .


.PHONY: docker_push
docker_push:
	docker push $(DOCKER_IMAGE_MDNINJA):latest


.PHONY: docker_build_and_push_multiplatform
docker_build_and_push_multiplatform:
	docker buildx build --push --platform linux/amd64,linux/arm64 -t $(DOCKER_IMAGE_MDNINJA):latest -f Dockerfile .


####################################################################################################
# mdninja
####################################################################################################
MDNINJA_BIN = mdninja
STRIP_CMD = strip --strip-all -xX \
	--remove-section=.comment \
	--remove-section=.gnu.hash \
	--remove-section=.gnu.version \
	--remove-section=.note.ABI-tag \
	--remove-section=.note.gnu.build-id \
	--remove-section=.got \
	--remove-section=.gosymtab \
	--remove-section=.go.buildinfo \
	--remove-section=.gnu.build.attributes

# GOAMD64=v3
.PHONY: mdninja
mdninja:
	mkdir -p $(DIST_DIR)
	GOOS=linux CGO_ENABLED=0 go build -o $(DIST_DIR)/$(MDNINJA_BIN) -tags timetzdata \
		-trimpath -a -ldflags "-B none -extldflags -static -w -s -X $(GO_MODULE)/pkg/buildinfo.Version=$(VERSION)" \
		./cmd/mdninja
	$(STRIP_CMD) $(DIST_DIR)/$(MDNINJA_BIN)
#upx --best --lzma --overlay=strip --strip-relocs $(DIST_DIR)/$(MDNINJA_BIN)



####################################################################################################
# mdninja-server
####################################################################################################
MDNINJA_SERVER_BIN = mdninja-server

# We use the -tags timetzdata flag to avoid relying on the system's timezone database which is not
# present by default in scratch containers
.PHONY: mdninja-server
mdninja-server:
	mkdir -p $(DIST_DIR)
	GOAMD64=v3 GOOS=linux CGO_ENABLED=0 go build -o $(DIST_DIR)/$(MDNINJA_SERVER_BIN) -tags timetzdata \
		-trimpath -a -ldflags "-B none -extldflags -static -w -s -X $(GO_MODULE)/pkg/buildinfo.Version=$(VERSION)" \
		./cmd/mdninja-server
	$(STRIP_CMD) $(DIST_DIR)/$(MDNINJA_SERVER_BIN)


.PHONY: dev
dev:
	go tool watchgod -log-prefix=false -build="make build_server_dev" -command="./dist/$(MDNINJA_SERVER_BIN)" \
		-pattern "(.+(\\.go|\\.yml))|(^assets.+)" -graceful-kill=true

.PHONY: build_server_dev
build_server_dev:
	go build -o $(DIST_DIR)/$(MDNINJA_SERVER_BIN) -ldflags "-B none -extldflags -w -s -X $(GO_MODULE)/pkg/buildinfo.Version=$(VERSION)" \
		./cmd/mdninja-server


####################################################################################################
# mdninja-ebook
####################################################################################################
MDNINJA_EBOOK_BIN = mdninja-ebook

# GOAMD64=v3
.PHONY: mdninja-ebook
mdninja-ebook:
	mkdir -p $(DIST_DIR)
	GOOS=linux CGO_ENABLED=0 go build -o $(DIST_DIR)/$(MDNINJA_EBOOK_BIN) -trimpath -a -ldflags "-extldflags -static -w -s \
		-X $(GO_MODULE)/pkg/buildinfo.Version=$(VERSION)" \
		./cmd/mdninja-ebook
	$(STRIP_CMD) $(DIST_DIR)/$(MDNINJA_EBOOK_BIN)


.PHONY: docker_build_mdninja_ebook
docker_build_mdninja_ebook:
	docker build -t $(DOCKER_IMAGE_MDNINJA_EBOOK):latest -f Dockerfile.ebook .

.PHONY: docker_push_mdninja_ebook
docker_push_mdninja_ebook:
	docker push $(DOCKER_IMAGE_MDNINJA_EBOOK):latest
